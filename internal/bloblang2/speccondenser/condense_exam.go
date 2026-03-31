package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

// condenseConfig holds everything the condense exam's scorer needs.
type condenseConfig struct {
	tests       []eligibleTest
	model       string
	agent       agentexam.Agent
	timeout     agentexam.Options
	artifactDir string
	subRuns     int // number of times to run each sub-exam (results averaged)
}

// buildCondenseExam builds the top-level exam: the agent reads the full spec
// and produces a condensed version. The scorer then runs read/write sub-exams
// against the condensed spec to measure how much information was preserved.
func buildCondenseExam(specFiles map[string][]byte, cfg condenseConfig) (*agentexam.Exam, error) {
	// The clean room contains only the spec files. The agent must produce
	// a single condensed file.
	files := make(map[string][]byte, len(specFiles))
	for k, v := range specFiles {
		files[k] = v
	}

	return &agentexam.Exam{
		Name:   "condense-" + cfg.model,
		Files:  files,
		Prompt: condensePrompt,
		Score: func(ctx context.Context, room *agentexam.Room, output io.Writer) ([]agentexam.Result, error) {
			return scoreCondense(ctx, room, output, cfg)
		},
	}, nil
}

func scoreCondense(ctx context.Context, room *agentexam.Room, output io.Writer, cfg condenseConfig) ([]agentexam.Result, error) {
	// Extract the condensed spec.
	condensed, ok := room.GetFile("condensed_spec.md")
	if !ok {
		return nil, errors.New("agent did not produce condensed_spec.md")
	}

	condensedSpec := map[string][]byte{
		"spec/condensed_spec.md": []byte(condensed),
	}

	subRuns := cfg.subRuns
	if subRuns < 1 {
		subRuns = 1
	}

	fmt.Fprintf(output, "=== condensed spec produced, running %d sub-exam iteration(s) ===\n", subRuns)

	subOpts := &agentexam.Options{
		Agent:   cfg.agent,
		Timeout: cfg.timeout.Timeout,
		Output:  output,
	}

	var allReadResults, allWriteResults []agentexam.Result

	for i := range subRuns {
		if subRuns > 1 {
			fmt.Fprintf(output, "\n--- sub-exam iteration %d/%d ---\n", i+1, subRuns)
		}

		readExam, err := buildReadExam(condensedSpec, cfg.tests, cfg.model)
		if err != nil {
			return nil, fmt.Errorf("building read sub-exam: %w", err)
		}
		readResults, err := agentexam.Run(ctx, readExam, subOpts)
		if err != nil {
			return nil, fmt.Errorf("running read sub-exam (iteration %d): %w", i+1, err)
		}
		allReadResults = append(allReadResults, readResults...)

		writeExam, err := buildWriteExam(condensedSpec, cfg.tests, cfg.model)
		if err != nil {
			return nil, fmt.Errorf("building write sub-exam: %w", err)
		}
		writeResults, err := agentexam.Run(ctx, writeExam, subOpts)
		if err != nil {
			return nil, fmt.Errorf("running write sub-exam (iteration %d): %w", i+1, err)
		}
		allWriteResults = append(allWriteResults, writeResults...)
	}

	// Aggregate across all iterations.
	readSummary := agentexam.Summarize(allReadResults)
	writeSummary := agentexam.Summarize(allWriteResults)

	var readPct, writePct float64
	if readSummary.Total > 0 {
		readPct = readSummary.TotalScore / float64(readSummary.Total)
	}
	if writeSummary.Total > 0 {
		writePct = writeSummary.TotalScore / float64(writeSummary.Total)
	}
	overallPct := (readPct + writePct) / 2

	fmt.Fprintf(output, "\n=== condense results (%d iteration(s)) ===\n", subRuns)
	fmt.Fprintf(output, "  read:    %.1f%% (%d tests)\n", readPct*100, readSummary.Total)
	fmt.Fprintf(output, "  write:   %.1f%% (%d tests)\n", writePct*100, writeSummary.Total)
	fmt.Fprintf(output, "  overall: %.1f%%\n", overallPct*100)

	// Build per-category breakdown.
	catScores := buildCategoryScores(allReadResults, allWriteResults)

	// Write artifact.
	if err := writeArtifact(cfg.artifactDir, condensed, artifact{
		OverallScore: overallPct,
		ReadScore:    readPct,
		WriteScore:   writePct,
		Categories:   catScores,
	}); err != nil {
		return nil, fmt.Errorf("writing artifact: %w", err)
	}

	// Return a single result representing the condensed spec quality.
	return []agentexam.Result{{
		ID:    "condense",
		Name:  "condensed spec quality",
		Score: overallPct,
	}}, nil
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

	// Merge all category names.
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

func writeArtifact(baseDir, condensedSpec string, art artifact) error {
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

	data, err := json.MarshalIndent(art, "", "  ")
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

## Available Tools

You have four tools:
- list_files: list files in the working directory (use pattern "" for all files)
- read_file: read a file by relative path
- write_file: write content to a file by relative path
- grep: search for text across files

## Instructions

1. Use list_files to find all files in the spec/ directory.
2. Use read_file to read each spec file thoroughly.
3. After reading all files, compose a single condensed specification that preserves ALL semantic detail.
4. Use write_file to save the result as condensed_spec.md.

## Condensation Rules

- Preserve every rule, behavior, edge case, operator, function, and type from the original.
- You may restructure, reword, and compress freely. Remove redundancy, merge sections, use tables or compact notation.
- The goal is minimum size with zero information loss. Another agent will be tested on its ability to understand the language using only your condensed spec.
- Do NOT omit any language features, functions, operators, or edge cases.
- Do NOT add invented features or behaviors not in the original spec.
- Do NOT include test cases or examples unless they clarify an otherwise ambiguous rule.

## IMPORTANT

You MUST use the write_file tool to create condensed_spec.md before you finish. Do not stop until you have written the file.
`
