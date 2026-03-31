package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	bloblang2 "github.com/redpanda-data/benthos/v4/internal/bloblang2"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam"
)

// buildWriteExam builds the predict-mapping exam: the agent receives input and
// expected output, and must write a Bloblang V2 mapping that produces it.
func buildWriteExam(specFiles map[string][]byte, tests []eligibleTest, model string) (*agentexam.Exam, error) {
	files := map[string][]byte{}

	var entries []manifestEntry
	for _, t := range tests {
		inputData, err := marshalEnvelope(envelope{Value: t.Input, Metadata: t.InputMeta})
		if err != nil {
			continue
		}
		outputData, err := marshalEnvelope(t.Expected)
		if err != nil {
			continue
		}

		files[filepath.Join("tests", t.ID+".input.json")] = inputData
		files[filepath.Join("tests", t.ID+".output.json")] = outputData
		entries = append(entries, t.manifestEntry)
	}

	for k, v := range specFiles {
		files[k] = v
	}

	return &agentexam.Exam{
		Name:   "predict-mapping-" + model,
		Files:  files,
		Prompt: predictMappingPrompt,
		Score: func(_ context.Context, room *agentexam.Room, output io.Writer) ([]agentexam.Result, error) {
			return scoreWriteExam(room, entries, output)
		},
	}, nil
}

func scoreWriteExam(room *agentexam.Room, entries []manifestEntry, output io.Writer) ([]agentexam.Result, error) {
	interp := &bloblang2.Interp{}
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

		mappingSrc, ok := room.GetFile(filepath.Join("tests", entry.ID+".blobl2"))
		if !ok {
			r.Error = "mapping file not found"
			emit(r)
			continue
		}

		var inputEnv map[string]any
		if err := room.GetFileJSON(filepath.Join("tests", entry.ID+".input.json"), &inputEnv); err != nil {
			r.Error = fmt.Sprintf("reading input: %v", err)
			emit(r)
			continue
		}
		inputValue := inputEnv["value"]
		inputMeta, _ := inputEnv["metadata"].(map[string]any)
		if inputMeta == nil {
			inputMeta = map[string]any{}
		}

		mapping, compileErr := interp.Compile(mappingSrc, nil)
		if compileErr != nil {
			r.Error = fmt.Sprintf("compile error: %v", compileErr)
			emit(r)
			continue
		}

		outVal, outMeta, deleted, execErr := mapping.Exec(inputValue, inputMeta)
		if execErr != nil {
			r.Error = fmt.Sprintf("runtime error: %v", execErr)
			emit(r)
			continue
		}
		if deleted {
			r.Error = "mapping deleted the message"
			emit(r)
			continue
		}
		if outMeta == nil {
			outMeta = map[string]any{}
		}

		coercedOutput := coerceToNaturalJSON(outVal)
		coercedMeta := coerceToNaturalJSON(outMeta)

		if ok, diff := naturalJSONEqual(entry.Expected.Value, coercedOutput); !ok {
			r.Error = "output mismatch: " + diff
			emit(r)
			continue
		}

		if !entry.NoMetadataCheck {
			expectedMeta := entry.Expected.Metadata
			if expectedMeta == nil {
				expectedMeta = map[string]any{}
			}
			if ok, diff := naturalJSONEqual(any(expectedMeta), coercedMeta); !ok {
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

const predictMappingPrompt = `# Task: Write Bloblang V2 Mappings

You are being tested on your ability to understand a programming language specification.

## Instructions

1. Read the **complete** Bloblang V2 specification in the ` + "`spec/`" + ` directory.
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
