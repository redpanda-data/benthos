package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam"
)

// buildReadExam builds the predict-output exam: the agent receives a mapping
// and input, and must predict the output the mapping produces.
func buildReadExam(specFiles map[string][]byte, tests []eligibleTest, model string) (*agentexam.Exam, error) {
	files := map[string][]byte{}

	var entries []manifestEntry
	for _, t := range tests {
		inputData, err := marshalEnvelope(envelope{Value: t.Input, Metadata: t.InputMeta})
		if err != nil {
			continue
		}

		files[filepath.Join("tests", t.ID+".blobl2")] = []byte(t.Mapping)
		files[filepath.Join("tests", t.ID+".input.json")] = inputData
		entries = append(entries, t.manifestEntry)
	}

	for k, v := range specFiles {
		files[k] = v
	}

	return &agentexam.Exam{
		Name:   "predict-output-" + model,
		Files:  files,
		Prompt: predictOutputPrompt,
		Score: func(_ context.Context, room *agentexam.Room, output io.Writer) ([]agentexam.Result, error) {
			return scoreReadExam(room, entries, output)
		},
	}, nil
}

func scoreReadExam(room *agentexam.Room, entries []manifestEntry, output io.Writer) ([]agentexam.Result, error) {
	var results []agentexam.Result

	emit := func(r agentexam.Result) {
		results = append(results, r)
		agentexam.LogResult(output, r)
	}

	for _, entry := range entries {
		r := agentexam.Result{
			ID:    entry.ID,
			Group: entry.Category,
			Name:  entry.Name,
		}

		var outputEnv map[string]any
		if err := room.GetFileJSON(filepath.Join("tests", entry.ID+".output.json"), &outputEnv); err != nil {
			r.Error = fmt.Sprintf("reading output: %v", err)
			emit(r)
			continue
		}

		actualValue := outputEnv["value"]
		actualMeta, _ := outputEnv["metadata"].(map[string]any)
		if actualMeta == nil {
			actualMeta = map[string]any{}
		}

		if ok, diff := naturalJSONEqual(entry.Expected.Value, actualValue); !ok {
			r.Error = "output mismatch: " + diff
			emit(r)
			continue
		}

		if !entry.NoMetadataCheck {
			expectedMeta := entry.Expected.Metadata
			if expectedMeta == nil {
				expectedMeta = map[string]any{}
			}
			if ok, diff := naturalJSONEqual(any(expectedMeta), any(actualMeta)); !ok {
				r.Error = "metadata mismatch: " + diff
				emit(r)
				continue
			}
		}

		r.Score = 1
		emit(r)
	}

	return results, nil
}

const predictOutputPrompt = `# Task: Predict Bloblang V2 Mapping Output

You are being tested on your ability to understand a programming language specification.

## Instructions

1. Read the **complete** Bloblang V2 specification in the ` + "`spec/`" + ` directory.
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
