// speccondenser condenses a Bloblang V2 specification and measures the
// quality of the condensed version by running prompt-based read/write exams
// against it using configurable pools of agents.
//
// Usage:
//
//	speccondenser <config.yaml>
package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: speccondenser <config.yaml>\n       speccondenser example-config")
		os.Exit(1)
	}

	if os.Args[1] == "example-config" {
		fmt.Print(exampleConfig)
		return
	}

	configPath := os.Args[1]

	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Resolve tests dir. If not configured, extract the embedded curated
	// exam suite to a temp directory and use that.
	testsDir := cfg.TestsDir
	var cleanupTestsDir func()
	if testsDir == "" {
		dir, cleanup, err := extractEmbeddedExam()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error extracting embedded exam: %v\n", err)
			os.Exit(1)
		}
		testsDir = dir
		cleanupTestsDir = cleanup
		fmt.Fprintf(os.Stderr, "using embedded exam suite\n")
	}
	if cleanupTestsDir != nil {
		defer cleanupTestsDir()
	}

	// Load spec and tests.
	specFiles, err := loadSpecDocs(cfg.SpecDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading spec: %v\n", err)
		os.Exit(1)
	}

	cats := parseCategories(strings.Join(cfg.Categories, ","))
	tests, err := loadEligibleTests(testsDir, cats)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading tests: %v\n", err)
		os.Exit(1)
	}
	if len(tests) == 0 {
		fmt.Fprintln(os.Stderr, "error: no eligible tests found")
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "loaded %d eligible tests\n", len(tests))

	output := io.Discard
	if cfg.Verbose {
		if cfg.VerboseFile != "" {
			f, err := os.Create(cfg.VerboseFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error creating verbose file: %v\n", err)
				os.Exit(1)
			}
			defer f.Close()
			output = io.MultiWriter(os.Stdout, f)
		} else {
			output = os.Stdout
		}
	}

	// Phase 1: Get the condensed spec — either from a file or by running
	// the condense agent.
	var condensedSpec string
	if cfg.Condense.SpecFile != "" {
		data, err := os.ReadFile(cfg.Condense.SpecFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading spec file: %v\n", err)
			os.Exit(1)
		}
		condensedSpec = string(data)
		fmt.Fprintf(os.Stderr, "using pre-condensed spec: %s (%d bytes)\n", cfg.Condense.SpecFile, len(condensedSpec))
	} else {
		condenseAgent, err := buildAgent(cfg.Condense.Agent)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error building condense agent: %v\n", err)
			os.Exit(1)
		}

		exam, err := buildCondenseExam(specFiles, &condensedSpec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		opts := &agentexam.Options{
			Agent:   condenseAgent,
			Timeout: cfg.Condense.Timeout,
			KeepDir: cfg.KeepDir,
			Output:  output,
		}

		results, err := agentexam.Run(context.Background(), exam, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error running condense exam: %v\n", err)
			os.Exit(1)
		}
		if len(results) == 0 || results[0].Score < 1 {
			fmt.Fprintln(os.Stderr, "error: condense exam failed — agent did not produce condensed_spec.md")
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "condensed spec: %d bytes\n", len(condensedSpec))
	}

	// Phase 2: Score the condensed spec with each pool.
	poolResults, err := scoreCondensedSpec(context.Background(), condensedSpec, tests, cfg.Scoring.Pools, output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error scoring: %v\n", err)
		os.Exit(1)
	}

	// Print results — one table per pool, then a comparison summary.
	for _, pr := range poolResults {
		fmt.Printf("\n=== %s ===\n\n", pr.Name)
		agentexam.PrintComparisonTable(os.Stdout, map[string][]agentexam.Result{
			"read":  pr.ReadResults,
			"write": pr.WriteResults,
		})
	}

	if len(poolResults) > 1 {
		fmt.Printf("\n=== comparison ===\n\n")
		printPoolSummary(os.Stdout, poolResults)
	}

	// Write artifact.
	if err := writeArtifact(cfg.ArtifactDir, condensedSpec, poolResults); err != nil {
		fmt.Fprintf(os.Stderr, "error writing artifact: %v\n", err)
		os.Exit(1)
	}
}

// printPoolSummary writes a compact comparison table with one row per pool.
func printPoolSummary(w io.Writer, pools []poolResult) {
	nameWidth := 4 // "Pool"
	for _, pr := range pools {
		if len(pr.Name) > nameWidth {
			nameWidth = len(pr.Name)
		}
	}

	colWidth := 20
	sep := "+" + strings.Repeat("-", nameWidth+2) +
		"+" + strings.Repeat("-", colWidth+2) +
		"+" + strings.Repeat("-", colWidth+2) + "+"

	fmt.Fprintln(w, sep)
	fmt.Fprintf(w, "| %-*s | %-*s | %-*s |\n", nameWidth, "Pool", colWidth, "read", colWidth, "write")
	fmt.Fprintln(w, sep)

	for _, pr := range pools {
		readSum := agentexam.Summarize(pr.ReadResults)
		writeSum := agentexam.Summarize(pr.WriteResults)

		readCell := "N/A"
		if readSum.Total > 0 {
			pct := readSum.TotalScore / float64(readSum.Total) * 100
			readCell = fmt.Sprintf("%5.1f%% (%.1f/%d)", pct, readSum.TotalScore, readSum.Total)
		}
		writeCell := "N/A"
		if writeSum.Total > 0 {
			pct := writeSum.TotalScore / float64(writeSum.Total) * 100
			writeCell = fmt.Sprintf("%5.1f%% (%.1f/%d)", pct, writeSum.TotalScore, writeSum.Total)
		}

		fmt.Fprintf(w, "| %-*s | %-*s | %-*s |\n", nameWidth, pr.Name, colWidth, readCell, colWidth, writeCell)
	}
	fmt.Fprintln(w, sep)
}

// extractEmbeddedExam writes the embedded exam YAML files to a temp directory
// and returns the path along with a cleanup function.
func extractEmbeddedExam() (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "blobl-exam-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp dir: %w", err)
	}
	cleanup = func() { os.RemoveAll(dir) }

	err = fs.WalkDir(examFS, "exam", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		data, readErr := examFS.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(filepath.Join(dir, d.Name()), data, 0o644)
	})
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("extracting exam files: %w", err)
	}
	return dir, cleanup, nil
}

const exampleConfig = `# speccondenser configuration
#
# Usage: speccondenser config.yaml

# Path to the directory containing spec .md files.
spec_dir: ./spec

# Path to the directory containing test .yaml files.
# Defaults to the embedded curated exam suite if omitted.
# tests_dir: ./spec/tests

# Optional list of test categories to include. Omit to include all.
# categories:
#   - expressions
#   - stdlib

# Where to write result artifacts.
artifact_dir: .

# Print agent output and individual results to stdout.
verbose: false

# When verbose is enabled, also write agent output to this file.
# verbose_file: output.log

# Keep the condense phase clean room directory after the run.
keep_dir: false

# Outer phase: a single agent condenses the full spec into a compact form.
# To skip condensing and use a pre-made spec file, set spec_file instead.
condense:
  # spec_file: ./condensed_spec.md
  agent:
    type: ollama          # ollama | claude | opencode
    model: qwen3.5:latest
    # base_url: http://localhost:11434  # ollama only
    # max_turns: 200                    # ollama / claude
    # command: claude                   # claude / opencode executable override
    # no_think: false                   # ollama only — disable reasoning
  timeout: 60m

# Inner phase: pools of agents score the condensed spec via read/write exams.
# Each test is a separate prompt-based agent call (no file tools).
scoring:
  pools:
    - name: qwen3.5
      agent:
        type: ollama
        model: qwen3.5:latest
        base_url: http://localhost:11434
        max_turns: 200
      runs: 1       # number of times to run the full test suite
      timeout: 10m  # per-test timeout

    # - name: claude-opus
    #   agent:
    #     type: claude
    #     model: claude-opus-4-20250514
    #   runs: 3
    #   timeout: 10m
`
