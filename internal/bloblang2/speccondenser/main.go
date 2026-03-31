// speccondenser condenses a Bloblang V2 specification and measures the
// quality of the condensed version by running read/write exams against it.
//
// Modes:
//   - condense (default): agent condenses the spec, sub-exams score it
//   - write:  directly test predict-mapping against the original spec
//   - read:   directly test predict-output against the original spec
//   - both:   run write + read directly against the original spec
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
	mode := fs.String("mode", "condense", "exam mode: condense, write, read, or both")
	agentType := fs.String("agent", "ollama", "agent type: ollama or claude")
	model := fs.String("model", "", "model name (ollama: default qwen3.5:latest, claude: default from CLI)")
	baseURL := fs.String("ollama-url", "http://localhost:11434", "Ollama API base URL")
	maxTurns := fs.Int("max-turns", 200, "max tool-calling turns (ollama) or max turns (claude)")
	timeout := fs.Duration("timeout", 60*time.Minute, "timeout per exam")
	keepDir := fs.Bool("keep-dir", false, "keep the clean room directory after the run")
	verbose := fs.Bool("verbose", false, "print agent output and individual results to stdout")
	artifactDir := fs.String("artifact-dir", ".", "directory where artifact folders are written")
	subRuns := fs.Int("sub-runs", 1, "number of times to run each sub-exam in condense mode (results averaged)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: speccondenser [flags] <spec-dir>\n\nModes:\n")
		fmt.Fprintf(os.Stderr, "  condense  Agent condenses the spec, sub-exams score it (default)\n")
		fmt.Fprintf(os.Stderr, "  write     Agent writes mappings given input + expected output\n")
		fmt.Fprintf(os.Stderr, "  read      Agent predicts output given mapping + input\n")
		fmt.Fprintf(os.Stderr, "  both      Run write + read directly against original spec\n\n")
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

	// Validate mode.
	switch *mode {
	case "condense", "write", "read", "both":
	default:
		fmt.Fprintf(os.Stderr, "error: unknown mode %q (use condense, write, read, or both)\n", *mode)
		os.Exit(1)
	}

	// Load spec docs.
	specFiles, err := loadSpecDocs(specDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading spec: %v\n", err)
		os.Exit(1)
	}

	// Load tests (needed for all modes except... well, all modes need them).
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

	var agent agentexam.Agent
	agentLabel := *model
	switch *agentType {
	case "ollama":
		ollamaModel := *model
		if ollamaModel == "" {
			ollamaModel = "qwen3.5:latest"
		}
		agentLabel = ollamaModel
		agent = &agents.Ollama{
			BaseURL:  *baseURL,
			Model:    ollamaModel,
			MaxTurns: *maxTurns,
		}
	case "claude":
		cc := &agents.ClaudeCode{
			Model:    *model,
			MaxTurns: *maxTurns,
		}
		if agentLabel == "" {
			agentLabel = "claude"
		}
		agent = cc
	default:
		fmt.Fprintf(os.Stderr, "error: unknown agent type %q (use ollama or claude)\n", *agentType)
		os.Exit(1)
	}

	output := io.Discard
	if *verbose {
		output = os.Stdout
	}

	opts := &agentexam.Options{
		Agent:   agent,
		Timeout: *timeout,
		KeepDir: *keepDir,
		Output:  output,
	}

	// Build exams based on mode.
	var exams []*agentexam.Exam

	switch *mode {
	case "condense":
		exam, buildErr := buildCondenseExam(specFiles, condenseConfig{
			tests:       tests,
			model:       agentLabel,
			agent:       agent,
			timeout:     *opts,
			artifactDir: *artifactDir,
			subRuns:     *subRuns,
		})
		if buildErr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", buildErr)
			os.Exit(1)
		}
		exams = append(exams, exam)

	case "write":
		exam, buildErr := buildWriteExam(specFiles, tests, agentLabel)
		if buildErr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", buildErr)
			os.Exit(1)
		}
		exams = append(exams, exam)

	case "read":
		exam, buildErr := buildReadExam(specFiles, tests, agentLabel)
		if buildErr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", buildErr)
			os.Exit(1)
		}
		exams = append(exams, exam)

	case "both":
		writeExam, buildErr := buildWriteExam(specFiles, tests, agentLabel)
		if buildErr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", buildErr)
			os.Exit(1)
		}
		readExam, buildErr := buildReadExam(specFiles, tests, agentLabel)
		if buildErr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", buildErr)
			os.Exit(1)
		}
		exams = append(exams, writeExam, readExam)
	}

	allResults, err := agentexam.RunAll(context.Background(), exams, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	agentexam.PrintComparisonTable(os.Stdout, allResults)
}
