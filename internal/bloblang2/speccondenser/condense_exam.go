package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	bloblang2 "github.com/redpanda-data/benthos/v4/internal/bloblang2"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam"
)

// artifact is the JSON structure written to the artifact directory.
type artifact struct {
	OverallScore float64                   `json:"overall_score"`
	ReadScore    float64                   `json:"read_score"`
	WriteScore   float64                   `json:"write_score"`
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

// poolResult holds aggregated results for a single scoring pool.
type poolResult struct {
	Name         string
	ReadResults  []agentexam.Result
	WriteResults []agentexam.Result
}

// scoreCondensedSpec runs prompt-based read and write sub-exams against the
// condensed spec across all configured scoring pools.
func scoreCondensedSpec(
	ctx context.Context,
	condensedSpec string,
	tests []eligibleTest,
	pools []PoolConfig,
	output io.Writer,
) ([]poolResult, error) {
	var results []poolResult

	for _, poolCfg := range pools {
		agent, err := buildAgent(poolCfg.Agent)
		if err != nil {
			return nil, fmt.Errorf("pool %q: building agent: %w", poolCfg.Name, err)
		}

		fmt.Fprintf(output, "\n=== scoring pool: %s (%d run(s)) ===\n", poolCfg.Name, poolCfg.Runs)

		pr := poolResult{Name: poolCfg.Name}

		for run := range poolCfg.Runs {
			if poolCfg.Runs > 1 {
				fmt.Fprintf(output, "\n--- %s run %d/%d ---\n", poolCfg.Name, run+1, poolCfg.Runs)
			}

			for _, test := range tests {
				readResult := runReadTest(ctx, agent, condensedSpec, test, poolCfg.Timeout, output)
				agentexam.LogResult(output, readResult)
				pr.ReadResults = append(pr.ReadResults, readResult)

				writeResult := runWriteTest(ctx, agent, condensedSpec, test, poolCfg.Timeout, output)
				agentexam.LogResult(output, writeResult)
				pr.WriteResults = append(pr.WriteResults, writeResult)
			}
		}

		results = append(results, pr)
	}

	return results, nil
}

func runReadTest(ctx context.Context, agent agentexam.Agent, spec string, test eligibleTest, timeout time.Duration, output io.Writer) agentexam.Result {
	r := agentexam.Result{
		ID:    test.ID,
		Group: test.Category,
		Name:  test.Name + " (read)",
	}

	inputData, _ := marshalEnvelope(envelope{Value: test.Input, Metadata: test.InputMeta})
	expectedData, _ := marshalEnvelope(test.Expected)

	fmt.Fprintf(output, "\n--- [read] %s — %s ---\n", test.ID, test.Name)
	fmt.Fprintf(output, "  mapping:\n    %s\n", indentLines(test.Mapping, "    "))
	fmt.Fprintf(output, "  input:\n    %s", indentLines(string(inputData), "    "))
	fmt.Fprintf(output, "  expected:\n    %s", indentLines(string(expectedData), "    "))

	prompt, err := buildReadPrompt(spec, test)
	if err != nil {
		r.Error = fmt.Sprintf("building prompt: %v", err)
		return r
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
		return r
	}

	response := result.Response
	fmt.Fprintf(output, "  response: %s\n", strings.TrimSpace(response))

	outputEnv, err := extractJSON(response)
	if err != nil {
		r.Error = fmt.Sprintf("extracting output: %v", err)
		return r
	}

	actualValue := outputEnv["value"]
	actualMeta, _ := outputEnv["metadata"].(map[string]any)
	if actualMeta == nil {
		actualMeta = map[string]any{}
	}

	if ok, diff := naturalJSONEqual(test.Expected.Value, actualValue); !ok {
		r.Error = "output mismatch: " + diff
		return r
	}

	if !test.NoMetadataCheck {
		expectedMeta := test.Expected.Metadata
		if expectedMeta == nil {
			expectedMeta = map[string]any{}
		}
		if ok, diff := naturalJSONEqual(any(expectedMeta), any(actualMeta)); !ok {
			r.Error = "metadata mismatch: " + diff
			return r
		}
	}

	r.Score = 1
	return r
}

func runWriteTest(ctx context.Context, agent agentexam.Agent, spec string, test eligibleTest, timeout time.Duration, output io.Writer) agentexam.Result {
	r := agentexam.Result{
		ID:    test.ID,
		Group: test.Category,
		Name:  test.Name + " (write)",
	}

	inputData, _ := marshalEnvelope(envelope{Value: test.Input, Metadata: test.InputMeta})
	expectedData, _ := marshalEnvelope(test.Expected)

	fmt.Fprintf(output, "\n--- [write] %s — %s ---\n", test.ID, test.Name)
	fmt.Fprintf(output, "  input:\n    %s", indentLines(string(inputData), "    "))
	fmt.Fprintf(output, "  expected:\n    %s", indentLines(string(expectedData), "    "))

	prompt, err := buildWritePrompt(spec, test)
	if err != nil {
		r.Error = fmt.Sprintf("building prompt: %v", err)
		return r
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
		return r
	}

	response := result.Response
	fmt.Fprintf(output, "  response: %s\n", strings.TrimSpace(response))

	mappingSrc := extractMapping(response)
	if mappingSrc == "" {
		r.Error = "agent produced empty mapping"
		return r
	}

	interp := &bloblang2.Interp{}
	mapping, compileErr := interp.Compile(mappingSrc, nil)
	if compileErr != nil {
		r.Error = fmt.Sprintf("compile error: %v", compileErr)
		return r
	}

	outVal, outMeta, deleted, execErr := mapping.Exec(test.Input, test.InputMeta)
	if execErr != nil {
		r.Error = fmt.Sprintf("runtime error: %v", execErr)
		return r
	}
	if deleted {
		r.Error = "mapping deleted the message"
		return r
	}
	if outMeta == nil {
		outMeta = map[string]any{}
	}

	coercedOutput := coerceToNaturalJSON(outVal)
	coercedMeta := coerceToNaturalJSON(outMeta)

	if ok, diff := naturalJSONEqual(test.Expected.Value, coercedOutput); !ok {
		r.Error = "output mismatch: " + diff
		return r
	}

	if !test.NoMetadataCheck {
		expectedMeta := test.Expected.Metadata
		if expectedMeta == nil {
			expectedMeta = map[string]any{}
		}
		if ok, diff := naturalJSONEqual(any(expectedMeta), coercedMeta); !ok {
			r.Error = "metadata mismatch: " + diff
			return r
		}
	}

	r.Score = 1
	return r
}

// aggregatePoolResults builds the comparison table data and per-pool artifacts.
func aggregatePoolResults(poolResults []poolResult) map[string][]agentexam.Result {
	out := make(map[string][]agentexam.Result, len(poolResults)*2)
	for _, pr := range poolResults {
		out[pr.Name+"/read"] = pr.ReadResults
		out[pr.Name+"/write"] = pr.WriteResults
	}
	return out
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

	combined := make(map[string]artifact, len(pools))
	for _, pr := range pools {
		combined[pr.Name] = buildPoolArtifact(pr)
	}

	data, err := json.MarshalIndent(combined, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "results.json"), append(data, '\n'), 0o644); err != nil {
		return err
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
