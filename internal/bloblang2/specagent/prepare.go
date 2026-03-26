package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/spectest"
)

// envelope wraps a value and its metadata for JSON serialization.
type envelope struct {
	Value    any            `json:"value"`
	Metadata map[string]any `json:"metadata"`
}

// manifestEntry records one eligible test for scoring.
type manifestEntry struct {
	ID              string   `json:"id"`
	Category        string   `json:"category"`
	SourceFile      string   `json:"source_file"`
	SourceIndex     int      `json:"source_index"`
	Name            string   `json:"name"`
	NoMetadataCheck bool     `json:"no_metadata_check,omitempty"`
	Expected        envelope `json:"expected"`

	// Fields used during prepare only (not serialized to manifest).
	mapping   string
	input     any
	inputMeta map[string]any
}

// manifest is written alongside the clean rooms as the answer key.
type manifest struct {
	Tests []manifestEntry `json:"tests"`
}

func cmdPrepare(args []string) {
	fs := flag.NewFlagSet("prepare", flag.ExitOnError)
	specDir := fs.String("spec", "spec", "path to spec directory (containing numbered .md files)")
	testsDir := fs.String("tests", "spec/tests", "path to spec tests directory")
	outputDir := fs.String("output", "", "output directory for clean rooms (required)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: specagent prepare [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if *outputDir == "" {
		fmt.Fprintln(os.Stderr, "error: --output is required")
		fs.Usage()
		os.Exit(1)
	}

	entries, err := loadEligibleTests(*testsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading tests: %v\n", err)
		os.Exit(1)
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "error: no eligible tests found")
		os.Exit(1)
	}

	fmt.Printf("Found %d eligible tests across %d categories\n", len(entries), countCategories(entries))

	poDir := filepath.Join(*outputDir, "predict_output")
	pmDir := filepath.Join(*outputDir, "predict_mapping")

	if err := generateCleanRoom(poDir, *specDir, entries, true); err != nil {
		fmt.Fprintf(os.Stderr, "error generating predict_output: %v\n", err)
		os.Exit(1)
	}
	if err := generateCleanRoom(pmDir, *specDir, entries, false); err != nil {
		fmt.Fprintf(os.Stderr, "error generating predict_mapping: %v\n", err)
		os.Exit(1)
	}

	mf := manifest{Tests: entries}
	if err := writeManifest(filepath.Join(*outputDir, "manifest.json"), &mf); err != nil {
		fmt.Fprintf(os.Stderr, "error writing manifest: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Clean rooms generated in %s\n", *outputDir)
	fmt.Printf("  predict_output: %s\n", poDir)
	fmt.Printf("  predict_mapping: %s\n", pmDir)
}

func loadEligibleTests(testsDir string) ([]manifestEntry, error) {
	files, err := spectest.DiscoverFiles(testsDir)
	if err != nil {
		return nil, err
	}

	var entries []manifestEntry
	for _, path := range files {
		rel, err := filepath.Rel(testsDir, path)
		if err != nil {
			rel = path
		}
		category := filepath.Dir(rel)
		baseName := strings.TrimSuffix(filepath.Base(rel), ".yaml")

		tf, err := spectest.LoadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", rel, err)
			continue
		}

		for i := range tf.Tests {
			tc := &tf.Tests[i]

			// Skip tests without concrete output.
			if !tc.HasOutput || tc.NoOutputCheck {
				continue
			}
			// Skip error/compile_error/deleted tests.
			if tc.CompileError != "" || tc.Error != "" || tc.HasError || tc.Deleted {
				continue
			}
			// Skip tests that require import files.
			if len(tf.Files) > 0 || len(tc.Files) > 0 {
				continue
			}

			// Decode values from YAML.
			input, err := spectest.DecodeValue(tc.Input)
			if err != nil {
				continue
			}
			inputMeta, err := decodeMetaValue(tc.InputMetadata)
			if err != nil {
				continue
			}
			output, err := spectest.DecodeValue(tc.Output)
			if err != nil {
				continue
			}
			outputMeta, err := decodeMetaValue(tc.OutputMetadata)
			if err != nil {
				continue
			}

			// Encode to natural JSON. Skip tests with types that JSON
			// cannot represent (bytes, timestamps, NaN, Inf).
			encodedInput, ok := encodeNaturalJSON(input)
			if !ok {
				continue
			}
			encodedInputMeta, ok := encodeNaturalMeta(inputMeta)
			if !ok {
				continue
			}
			encodedOutput, ok := encodeNaturalJSON(output)
			if !ok {
				continue
			}
			encodedOutputMeta, ok := encodeNaturalMeta(outputMeta)
			if !ok {
				continue
			}

			id := fmt.Sprintf("%s/%s_%03d", category, baseName, i)

			entries = append(entries, manifestEntry{
				ID:              id,
				Category:        category,
				SourceFile:      rel,
				SourceIndex:     i,
				Name:            tc.Name,
				NoMetadataCheck: tc.NoMetadataCheck,
				Expected: envelope{
					Value:    encodedOutput,
					Metadata: encodedOutputMeta,
				},
				mapping:   tc.Mapping,
				input:     encodedInput,
				inputMeta: encodedInputMeta,
			})
		}
	}
	return entries, nil
}

func decodeMetaValue(raw any) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}
	decoded, err := spectest.DecodeValue(raw)
	if err != nil {
		return nil, err
	}
	m, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("metadata must be an object, got %T", decoded)
	}
	return m, nil
}

func encodeNaturalMeta(m map[string]any) (map[string]any, bool) {
	if len(m) == 0 {
		return map[string]any{}, true
	}
	encoded, ok := encodeNaturalJSON(m)
	if !ok {
		return nil, false
	}
	em, _ := encoded.(map[string]any)
	return em, em != nil
}

func generateCleanRoom(dir, specDir string, entries []manifestEntry, isPredictOutput bool) error {
	// Copy spec docs.
	specDst := filepath.Join(dir, "spec")
	if err := copySpecDocs(specDir, specDst); err != nil {
		return fmt.Errorf("copying spec: %w", err)
	}

	// Write test files.
	testsDir := filepath.Join(dir, "tests")
	for _, e := range entries {
		testDir := filepath.Join(testsDir, filepath.Dir(e.ID))
		if err := os.MkdirAll(testDir, 0o755); err != nil {
			return err
		}

		base := filepath.Join(testsDir, e.ID)

		// Both modes get the input file.
		inputEnv := envelope{Value: e.input, Metadata: e.inputMeta}
		if err := writeJSONFile(base+".input.json", inputEnv); err != nil {
			return fmt.Errorf("writing input %s: %w", e.ID, err)
		}

		if isPredictOutput {
			// Agent gets the mapping, must produce the output.
			if err := os.WriteFile(base+".blobl2", []byte(e.mapping), 0o644); err != nil {
				return fmt.Errorf("writing mapping %s: %w", e.ID, err)
			}
		} else {
			// Agent gets the expected output, must produce the mapping.
			if err := writeJSONFile(base+".output.json", e.Expected); err != nil {
				return fmt.Errorf("writing output %s: %w", e.ID, err)
			}
		}
	}

	// Write the agent prompt.
	prompt := predictOutputPrompt
	if !isPredictOutput {
		prompt = predictMappingPrompt
	}
	if err := os.WriteFile(filepath.Join(dir, "PROMPT.md"), []byte(prompt), 0o644); err != nil {
		return fmt.Errorf("writing prompt: %w", err)
	}

	return nil
}

func copySpecDocs(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	// Only copy top-level .md files — skip subdirectories entirely.
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, d := range entries {
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, d.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, d.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writeManifest(path string, m *manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func countCategories(entries []manifestEntry) int {
	cats := map[string]struct{}{}
	for _, e := range entries {
		cats[e.Category] = struct{}{}
	}
	return len(cats)
}

const predictOutputPrompt = `# Task: Predict Bloblang V2 Mapping Output

You are being tested on your ability to understand a programming language specification.

## Instructions

1. Read the **complete** Bloblang V2 specification in the ` + "`spec/`" + ` directory. Start with ` + "`spec/README.md`" + ` for an overview, then read each numbered file in order.
2. In the ` + "`tests/`" + ` directory (organized by category subdirectories) you will find pairs of files:
   - ` + "`<name>.blobl2`" + ` — a Bloblang V2 mapping
   - ` + "`<name>.input.json`" + ` — the input document for that mapping
3. For **each** pair, determine what the mapping produces when executed against the input, and write the result to ` + "`<name>.output.json`" + ` in the same directory.

## JSON File Format

Input and output files use this envelope structure:

` + "```json" + `
{
  "value": <the document value>,
  "metadata": { <key-value metadata, defaults to empty object> }
}
` + "```" + `

All values use standard JSON types: strings, numbers, booleans, null, arrays, and objects. There are no special type annotations — numbers are just numbers.

## Rules

- Process **every** test file. Do not skip any.
- Do **not** reference any documents, code, or examples outside of this directory.
- Base your answers solely on the language specification in ` + "`spec/`" + `.
- If a mapping does not modify metadata, the output metadata should be ` + "`{}`" + `.
`

const predictMappingPrompt = `# Task: Write Bloblang V2 Mappings

You are being tested on your ability to understand a programming language specification.

## Instructions

1. Read the **complete** Bloblang V2 specification in the ` + "`spec/`" + ` directory. Start with ` + "`spec/README.md`" + ` for an overview, then read each numbered file in order.
2. In the ` + "`tests/`" + ` directory (organized by category subdirectories) you will find pairs of files:
   - ` + "`<name>.input.json`" + ` — an input document
   - ` + "`<name>.output.json`" + ` — the expected output document
3. For **each** pair, write a Bloblang V2 mapping that transforms the input into the expected output, and save it as ` + "`<name>.blobl2`" + ` in the same directory.

## JSON File Format

Input and output files use this envelope structure:

` + "```json" + `
{
  "value": <the document value>,
  "metadata": { <key-value metadata, defaults to empty object> }
}
` + "```" + `

All values use standard JSON types: strings, numbers, booleans, null, arrays, and objects. There are no special type annotations — numbers are just numbers.

## Rules

- Your mapping must be valid Bloblang V2 according to the specification.
- When executed against the input, it must produce the expected output (value and metadata).
- If the expected output has non-empty metadata, include metadata assignments.
- Process **every** test file. Do not skip any.
- Do **not** reference any documents, code, or examples outside of this directory.
- Base your mappings solely on the language specification in ` + "`spec/`" + `.
`
