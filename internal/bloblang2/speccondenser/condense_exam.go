package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	bloblang2 "github.com/redpanda-data/benthos/v4/internal/bloblang2"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam"
)

// artifact is the JSON structure written to the artifact directory.
type artifact struct {
	OverallScore float64                   `json:"overall_score"`
	ReadScore    float64                   `json:"read_score"`
	WriteScore   float64                   `json:"write_score"`
	DurationSecs float64                   `json:"duration_s"`
	Categories   map[string]categoryScores `json:"categories"`
}

type categoryScores struct {
	ReadScore  float64 `json:"read_score"`
	WriteScore float64 `json:"write_score"`
}

// buildCondenseExam builds the top-level exam: the agent reads the full spec
// and produces a condensed version. The condensed spec text is captured into
// the provided pointer during scoring.
func buildCondenseExam(specFiles map[string][]byte, condensedOut *string) (*agentexam.Exam, error) {
	files := make(map[string][]byte, len(specFiles))
	for k, v := range specFiles {
		files[k] = v
	}

	return &agentexam.Exam{
		Name:     "condense",
		UseFiles: true,
		Files:    files,
		Prompt:   condensePrompt,
		Score: func(_ context.Context, room *agentexam.Room, _ io.Writer) ([]agentexam.Result, error) {
			spec, ok := room.GetFile("condensed_spec.md")
			if !ok {
				return nil, errors.New("agent did not produce condensed_spec.md")
			}
			*condensedOut = spec
			return []agentexam.Result{{
				ID:    "condense",
				Name:  "condensed spec produced",
				Score: 1,
			}}, nil
		},
	}, nil
}

// failedTranscript captures the exact prompt and response for a failed test,
// written to the artifact directory for debugging.
type failedTranscript struct {
	Pool     string
	Run      int // 1-based
	TestID   string
	TestName string
	Kind     string // "read" or "write"
	Prompt   string
	Response string
	Error    string
}

// poolResult holds aggregated results for a single scoring pool.
type poolResult struct {
	Name              string
	ReadResults       []agentexam.Result
	WriteResults      []agentexam.Result
	FailedTranscripts []failedTranscript
	Duration          time.Duration
}

// testOutcome captures the result, any failed transcripts, and the buffered
// verbose output for a single test executed concurrently.
type testOutcome struct {
	result      agentexam.Result
	transcripts []failedTranscript
	output      bytes.Buffer
}

// syncWriter wraps an io.Writer with a mutex so concurrent goroutines can
// write complete lines without interleaving.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// scoreCondensedSpec runs prompt-based read and write sub-exams against the
// condensed spec across all configured scoring pools. Pools run concurrently.
func scoreCondensedSpec(
	ctx context.Context,
	condensedSpec string,
	tests []eligibleTest,
	pools []PoolConfig,
	output io.Writer,
) ([]poolResult, error) {
	writeGroups := groupWriteTests(tests)
	sw := &syncWriter{w: output}

	results := make([]poolResult, len(pools))
	eg, egCtx := errgroup.WithContext(ctx)

	for i, poolCfg := range pools {
		eg.Go(func() error {
			pr, err := scorePool(egCtx, condensedSpec, tests, writeGroups, poolCfg, sw)
			if err != nil {
				return fmt.Errorf("pool %q: %w", poolCfg.Name, err)
			}
			results[i] = pr
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// scorePool runs all runs for a single scoring pool. Runs within a pool are
// sequential; tests within each run are parallel (bounded by Concurrency).
func scorePool(
	ctx context.Context,
	condensedSpec string,
	tests []eligibleTest,
	writeGroups []writeTestGroup,
	poolCfg PoolConfig,
	sw *syncWriter,
) (poolResult, error) {
	// Validate agent config once up front.
	if _, err := buildAgent(poolCfg.Agent); err != nil {
		return poolResult{}, fmt.Errorf("building agent: %w", err)
	}

	fmt.Fprintf(sw, "\n=== scoring pool: %s (%d run(s), concurrency=%d) ===\n",
		poolCfg.Name, poolCfg.Runs, poolCfg.Concurrency)

	poolStart := time.Now()
	pr := poolResult{Name: poolCfg.Name}

	for run := range poolCfg.Runs {
		if poolCfg.Runs > 1 {
			fmt.Fprintf(sw, "\n--- %s run %d/%d ---\n", poolCfg.Name, run+1, poolCfg.Runs)
		}

		readOutcomes, writeOutcomes, err := scoreRun(
			ctx, condensedSpec, tests, writeGroups, poolCfg, sw,
		)
		if err != nil {
			return poolResult{}, err
		}

		// Assemble results in deterministic order and flush buffered output.
		for i := range readOutcomes {
			o := &readOutcomes[i]
			pr.ReadResults = append(pr.ReadResults, o.result)
			for j := range o.transcripts {
				o.transcripts[j].Pool = poolCfg.Name
				o.transcripts[j].Run = run + 1
			}
			pr.FailedTranscripts = append(pr.FailedTranscripts, o.transcripts...)
			_, _ = sw.Write(o.output.Bytes())
		}
		for i := range writeOutcomes {
			o := &writeOutcomes[i]
			pr.WriteResults = append(pr.WriteResults, o.result)
			for j := range o.transcripts {
				o.transcripts[j].Pool = poolCfg.Name
				o.transcripts[j].Run = run + 1
			}
			pr.FailedTranscripts = append(pr.FailedTranscripts, o.transcripts...)
			_, _ = sw.Write(o.output.Bytes())
		}
	}

	pr.Duration = time.Since(poolStart)
	return pr, nil
}

// scoreRun fans out read and write tests concurrently within a single run,
// bounded by the pool's Concurrency setting.
func scoreRun(
	ctx context.Context,
	condensedSpec string,
	tests []eligibleTest,
	writeGroups []writeTestGroup,
	poolCfg PoolConfig,
	sw *syncWriter,
) ([]testOutcome, []testOutcome, error) {
	totalTests := len(tests) + len(writeGroups)
	readOutcomes := make([]testOutcome, len(tests))
	writeOutcomes := make([]testOutcome, len(writeGroups))

	var completed int32

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(poolCfg.Concurrency)

	// Fan out read tests.
	for i, test := range tests {
		eg.Go(func() error {
			agent, err := buildAgent(poolCfg.Agent)
			if err != nil {
				return fmt.Errorf("building agent: %w", err)
			}

			buf := &readOutcomes[i].output
			result, ft := runReadTest(egCtx, agent, condensedSpec, test, poolCfg.Timeout, buf)
			agentexam.LogResult(buf, result)

			readOutcomes[i].result = result
			if ft != nil {
				readOutcomes[i].transcripts = []failedTranscript{*ft}
			}

			n := atomic.AddInt32(&completed, 1)
			status := "PASS"
			if result.Score < 1 {
				status = "FAIL"
			}
			fmt.Fprintf(sw, "  [%d/%d] %s  %s  %s\n", n, totalTests, status, result.ID, result.Name)
			return nil
		})
	}

	// Fan out write groups.
	for i, group := range writeGroups {
		eg.Go(func() error {
			agent, err := buildAgent(poolCfg.Agent)
			if err != nil {
				return fmt.Errorf("building agent: %w", err)
			}

			buf := &writeOutcomes[i].output
			result, fts := runGroupWriteTest(egCtx, agent, condensedSpec, group, poolCfg.Timeout, buf)
			agentexam.LogResult(buf, result)

			writeOutcomes[i].result = result
			writeOutcomes[i].transcripts = fts

			n := atomic.AddInt32(&completed, 1)
			status := "PASS"
			if result.Score < 1 {
				status = "FAIL"
			}
			fmt.Fprintf(sw, "  [%d/%d] %s  %s  %s\n", n, totalTests, status, result.ID, result.Name)
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, nil, err
	}

	return readOutcomes, writeOutcomes, nil
}

func runReadTest(ctx context.Context, agent agentexam.Agent, spec string, test eligibleTest, timeout time.Duration, output io.Writer) (agentexam.Result, *failedTranscript) {
	r := agentexam.Result{
		ID:    test.ID,
		Group: test.Category,
		Name:  test.Name + " (read)",
	}

	inputJSON, _ := json.Marshal(test.Input)
	expectedJSON, _ := json.Marshal(test.Expected.Value)

	fmt.Fprintf(output, "\n--- [read] %s — %s ---\n", test.ID, test.Name)
	fmt.Fprintf(output, "  mapping:  %s\n", indentLines(test.Mapping, "            "))
	fmt.Fprintf(output, "  input:    %s\n", inputJSON)
	fmt.Fprintf(output, "  expected: %s\n", expectedJSON)

	var prompt, response string

	fail := func() (agentexam.Result, *failedTranscript) {
		return r, &failedTranscript{
			TestID: test.ID, TestName: test.Name, Kind: "read",
			Prompt: prompt, Response: response, Error: r.Error,
		}
	}

	prompt, err := buildReadPrompt(spec, test)
	if err != nil {
		r.Error = fmt.Sprintf("building prompt: %v", err)
		return fail()
	}

	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	result, err := agent.Run(runCtx, "", prompt, output)
	if err != nil {
		r.Error = fmt.Sprintf("agent error: %v", err)
		return fail()
	}

	response = result.Response
	fmt.Fprintf(output, "  response: %s\n", strings.TrimSpace(response))

	actualValue, err := extractValue(response)
	if err != nil {
		r.Error = fmt.Sprintf("extracting output: %v", err)
		return fail()
	}

	if ok, diff := naturalJSONEqual(test.Expected.Value, actualValue); !ok {
		r.Error = "output mismatch: " + diff
		return fail()
	}

	if !test.NoMetadataCheck {
		expectedMeta := test.Expected.Metadata
		if expectedMeta == nil {
			expectedMeta = map[string]any{}
		}
		actualMeta := extractMetadata(response)
		if actualMeta == nil {
			actualMeta = map[string]any{}
		}
		if ok, diff := naturalJSONEqual(any(expectedMeta), any(actualMeta)); !ok {
			r.Error = "metadata mismatch: " + diff
			return fail()
		}
	}

	r.Score = 1
	return r, nil
}

func runGroupWriteTest(ctx context.Context, agent agentexam.Agent, spec string, group writeTestGroup, timeout time.Duration, output io.Writer) (agentexam.Result, []failedTranscript) {
	r := agentexam.Result{
		ID:    group.Key,
		Group: group.Category,
		Name:  group.Name + " (write)",
	}

	fmt.Fprintf(output, "\n--- [write] %s — %s (%d case(s)) ---\n",
		group.Key, group.Name, len(group.Cases))

	var prompt, response string

	failAll := func(errMsg string) (agentexam.Result, []failedTranscript) {
		r.Error = errMsg
		return r, []failedTranscript{{
			TestID: group.Key, TestName: group.Name, Kind: "write",
			Prompt: prompt, Response: response, Error: errMsg,
		}}
	}

	prompt, err := buildGroupWritePrompt(spec, group)
	if err != nil {
		return failAll(fmt.Sprintf("building prompt: %v", err))
	}

	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	result, err := agent.Run(runCtx, "", prompt, output)
	if err != nil {
		return failAll(fmt.Sprintf("agent error: %v", err))
	}

	response = result.Response
	fmt.Fprintf(output, "  response: %s\n", strings.TrimSpace(response))

	mappingSrc := extractMapping(response)
	if mappingSrc == "" {
		return failAll("agent produced empty mapping")
	}

	interp := &bloblang2.Interp{}
	mapping, compileErr := interp.Compile(mappingSrc, nil)
	if compileErr != nil {
		return failAll(fmt.Sprintf("compile error: %v", compileErr))
	}

	// Execute against each case, accumulate partial score.
	passed := 0
	var caseErrors []string
	var failedTxns []failedTranscript

	for i, c := range group.Cases {
		outVal, outMeta, deleted, execErr := mapping.Exec(c.Input, c.InputMeta)

		caseFail := func(reason string) {
			msg := fmt.Sprintf("case %d (%s): %s", i+1, c.Name, reason)
			caseErrors = append(caseErrors, msg)
			failedTxns = append(failedTxns, failedTranscript{
				TestID:   c.ID,
				TestName: c.Name,
				Kind:     "write",
				Prompt:   prompt,
				Response: response,
				Error:    reason,
			})
		}

		if execErr != nil {
			caseFail(fmt.Sprintf("runtime error: %v", execErr))
			continue
		}
		if deleted {
			caseFail("mapping deleted the message")
			continue
		}
		if outMeta == nil {
			outMeta = map[string]any{}
		}

		coercedOutput := coerceToNaturalJSON(outVal)
		coercedMeta := coerceToNaturalJSON(outMeta)

		if ok, diff := naturalJSONEqual(c.Expected.Value, coercedOutput); !ok {
			caseFail("output mismatch: " + diff)
			continue
		}

		if !c.NoMetadataCheck {
			expectedMeta := c.Expected.Metadata
			if expectedMeta == nil {
				expectedMeta = map[string]any{}
			}
			if ok, diff := naturalJSONEqual(any(expectedMeta), coercedMeta); !ok {
				caseFail("metadata mismatch: " + diff)
				continue
			}
		}

		passed++
	}

	r.Score = float64(passed) / float64(len(group.Cases))

	if len(caseErrors) > 0 {
		r.Error = fmt.Sprintf("%d/%d cases failed:\n  %s",
			len(group.Cases)-passed, len(group.Cases),
			strings.Join(caseErrors, "\n  "))
	}

	fmt.Fprintf(output, "  score: %d/%d cases passed\n", passed, len(group.Cases))

	if len(failedTxns) > 0 {
		return r, failedTxns
	}
	return r, nil
}

func buildPoolArtifact(pr poolResult) artifact {
	readSummary := agentexam.Summarize(pr.ReadResults)
	writeSummary := agentexam.Summarize(pr.WriteResults)

	var readPct, writePct float64
	if readSummary.Total > 0 {
		readPct = readSummary.TotalScore / float64(readSummary.Total)
	}
	if writeSummary.Total > 0 {
		writePct = writeSummary.TotalScore / float64(writeSummary.Total)
	}

	return artifact{
		OverallScore: (readPct + writePct) / 2,
		ReadScore:    readPct,
		WriteScore:   writePct,
		DurationSecs: pr.Duration.Seconds(),
		Categories:   buildCategoryScores(pr.ReadResults, pr.WriteResults),
	}
}

func buildCategoryScores(readResults, writeResults []agentexam.Result) map[string]categoryScores {
	type accum struct {
		score float64
		total int
	}

	readCats := map[string]accum{}
	for _, r := range readResults {
		if r.Group == "" {
			continue
		}
		a := readCats[r.Group]
		a.score += r.Score
		a.total++
		readCats[r.Group] = a
	}

	writeCats := map[string]accum{}
	for _, r := range writeResults {
		if r.Group == "" {
			continue
		}
		a := writeCats[r.Group]
		a.score += r.Score
		a.total++
		writeCats[r.Group] = a
	}

	allCats := map[string]struct{}{}
	for k := range readCats {
		allCats[k] = struct{}{}
	}
	for k := range writeCats {
		allCats[k] = struct{}{}
	}

	out := make(map[string]categoryScores, len(allCats))
	for cat := range allCats {
		var cs categoryScores
		if a, ok := readCats[cat]; ok && a.total > 0 {
			cs.ReadScore = a.score / float64(a.total)
		}
		if a, ok := writeCats[cat]; ok && a.total > 0 {
			cs.WriteScore = a.score / float64(a.total)
		}
		out[cat] = cs
	}
	return out
}

func writeArtifact(baseDir, condensedSpec string, pools []poolResult) error {
	id, err := generateUUID()
	if err != nil {
		return fmt.Errorf("generating UUID: %w", err)
	}

	dir := filepath.Join(baseDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, "condensed_spec.md"), []byte(condensedSpec), 0o644); err != nil {
		return err
	}

	type resultsFile struct {
		Pools     map[string]artifact `json:"pools"`
		Aggregate artifact            `json:"aggregate"`
	}

	poolArtifacts := make(map[string]artifact, len(pools))
	for _, pr := range pools {
		poolArtifacts[pr.Name] = buildPoolArtifact(pr)
	}

	// Build aggregate across all pools.
	var allRead, allWrite []agentexam.Result
	var totalDuration time.Duration
	for _, pr := range pools {
		allRead = append(allRead, pr.ReadResults...)
		allWrite = append(allWrite, pr.WriteResults...)
		totalDuration += pr.Duration
	}
	aggregate := buildPoolArtifact(poolResult{
		ReadResults:  allRead,
		WriteResults: allWrite,
		Duration:     totalDuration,
	})

	data, err := json.MarshalIndent(resultsFile{
		Pools:     poolArtifacts,
		Aggregate: aggregate,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "results.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}

	// Write transcripts for failed tests, organized as:
	//   fail_transcripts/<pool>/run_<N>/<testID>_<kind>.txt
	var totalTranscripts int
	for _, pr := range pools {
		totalTranscripts += len(pr.FailedTranscripts)
	}
	if totalTranscripts > 0 {
		sanitize := strings.NewReplacer("/", "__", " ", "_")
		for _, pr := range pools {
			for _, ft := range pr.FailedTranscripts {
				runDir := filepath.Join(dir, "fail_transcripts", ft.Pool, fmt.Sprintf("run_%d", ft.Run))
				if err := os.MkdirAll(runDir, 0o755); err != nil {
					return err
				}
				filename := sanitize.Replace(ft.TestID) + "_" + ft.Kind + ".txt"
				content := fmt.Sprintf("Test: %s (%s)\nID:   %s\nError: %s\n\n=== PROMPT ===\n%s\n\n=== RESPONSE ===\n%s\n",
					ft.TestName, ft.Kind, ft.TestID, ft.Error, ft.Prompt, ft.Response)
				if err := os.WriteFile(filepath.Join(runDir, filename), []byte(content), 0o644); err != nil {
					return err
				}
			}
		}
		fmt.Fprintf(os.Stderr, "wrote %d failed transcript(s) to %s\n", totalTranscripts, filepath.Join(dir, "fail_transcripts"))
	}

	fmt.Fprintf(os.Stderr, "artifact written to %s\n", dir)
	return nil
}

const condensePrompt = `# Task: Condense a Programming Language Specification

You have access to the complete Bloblang V2 specification in the spec/ directory.

## Instructions

1. Use your available tools to list and read all files in the spec/ directory.
2. Read each spec file thoroughly.
3. After reading all files, compose a single condensed specification that preserves ALL semantic detail.
4. Write the result to a file called condensed_spec.md in the working directory root (not inside spec/).

## Condensation Rules

- Preserve every rule, behavior, edge case, operator, function, and type from the original.
- You may restructure, reword, and compress freely. Remove redundancy, merge sections, use tables or compact notation.
- The goal is minimum size with zero information loss. Another agent will be tested on its ability to understand the language using only your condensed spec.
- Do NOT omit any language features, functions, operators, or edge cases.
- Do NOT add invented features or behaviors not in the original spec.
- Do NOT include test cases or examples unless they clarify an otherwise ambiguous rule.

## IMPORTANT

You MUST write the file condensed_spec.md before you finish. Do not stop until you have written the file.
`
