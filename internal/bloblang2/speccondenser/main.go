// speccondenser runs Bloblang V2 spec exams against a local Ollama model.
//
// Two exam modes:
//   - write (predict-mapping): given input + expected output, write a mapping
//   - read  (predict-output):  given mapping + input, predict the output
//
// Usage:
//
//	speccondenser [flags] <spec-dir>
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam/agents"
)

func main() {
	fs := flag.NewFlagSet("speccondenser", flag.ExitOnError)
	testsDir := fs.String("tests", "", "path to spec tests directory (default: <spec-dir>/tests)")
	categories := fs.String("categories", "", "comma-separated list of categories to include")
	mode := fs.String("mode", "both", "exam mode: write, read, or both")
	model := fs.String("model", "qwen3-coder:30b-16k", "Ollama model name")
	baseURL := fs.String("ollama-url", "http://localhost:11434", "Ollama API base URL")
	maxTurns := fs.Int("max-turns", 200, "max tool-calling turns for the agent")
	timeout := fs.Duration("timeout", 60*time.Minute, "timeout per exam")
	keepDir := fs.Bool("keep-dir", false, "keep the clean room directory after the run")
	verbose := fs.Bool("verbose", false, "print agent output and individual results to stdout")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: speccondenser [flags] <spec-dir>\n\nModes:\n")
		fmt.Fprintf(os.Stderr, "  write  Agent writes mappings given input + expected output\n")
		fmt.Fprintf(os.Stderr, "  read   Agent predicts output given mapping + input\n")
		fmt.Fprintf(os.Stderr, "  both   Run both exams sequentially (default)\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "error: spec directory is required")
		fs.Usage()
		os.Exit(1)
	}
	specDir := fs.Arg(0)

	if *testsDir == "" {
		candidate := filepath.Join(specDir, "tests")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			*testsDir = candidate
		} else {
			fmt.Fprintln(os.Stderr, "error: --tests is required (no tests/ subdirectory found)")
			os.Exit(1)
		}
	}

	// Validate mode before doing any work.
	runWrite, runRead := false, false
	switch *mode {
	case "write":
		runWrite = true
	case "read":
		runRead = true
	case "both":
		runWrite, runRead = true, true
	default:
		fmt.Fprintf(os.Stderr, "error: unknown mode %q (use write, read, or both)\n", *mode)
		os.Exit(1)
	}

	// Load tests once.
	tests, err := loadEligibleTests(*testsDir, parseCategories(*categories))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading tests: %v\n", err)
		os.Exit(1)
	}
	if len(tests) == 0 {
		fmt.Fprintln(os.Stderr, "error: no eligible tests found")
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "loaded %d eligible tests\n", len(tests))

	// Build exams.
	var exams []*agentexam.Exam
	if runWrite {
		exam, err := buildWriteExam(specDir, tests, *model)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		exams = append(exams, exam)
	}
	if runRead {
		exam, err := buildReadExam(specDir, tests, *model)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		exams = append(exams, exam)
	}

	agent := &agents.Ollama{
		BaseURL:  *baseURL,
		Model:    *model,
		MaxTurns: *maxTurns,
	}

	output := io.Discard
	if *verbose {
		output = os.Stdout
	}

	allResults, err := agentexam.RunAll(context.Background(), exams, &agentexam.Options{
		Agent:   agent,
		Timeout: *timeout,
		KeepDir: *keepDir,
		Output:  output,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	agentexam.PrintComparisonTable(os.Stdout, allResults)
}
