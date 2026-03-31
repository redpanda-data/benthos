package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func buildReadPrompt(spec string, test eligibleTest) (string, error) {
	inputData, err := marshalEnvelope(envelope{Value: test.Input, Metadata: test.InputMeta})
	if err != nil {
		return "", fmt.Errorf("marshaling input: %w", err)
	}

	var b strings.Builder
	b.WriteString(`Here is a programming language specification:

<spec>
`)
	b.WriteString(spec)
	b.WriteString(`
</spec>

Here is a mapping written in that language:

<mapping>
`)
	b.WriteString(test.Mapping)
	b.WriteString(`
</mapping>

Here is the input document:

<input>
`)
	b.Write(inputData)
	b.WriteString(`</input>

The input and output use a JSON envelope format: {"value": <document>, "metadata": {...}}.
The mapping's "root" corresponds to the envelope's "value" field — the document itself.
Metadata assignments in the mapping correspond to the envelope's "metadata" field.

Execute the mapping against the input document and produce the output.
Respond with ONLY the JSON output envelope: {"value": ..., "metadata": {...}}
Do not include any explanation, commentary, or code fences.
`)
	return b.String(), nil
}

func buildWritePrompt(spec string, test eligibleTest) (string, error) {
	inputData, err := marshalEnvelope(envelope{Value: test.Input, Metadata: test.InputMeta})
	if err != nil {
		return "", fmt.Errorf("marshaling input: %w", err)
	}
	outputData, err := marshalEnvelope(test.Expected)
	if err != nil {
		return "", fmt.Errorf("marshaling output: %w", err)
	}

	var b strings.Builder
	b.WriteString(`Here is a programming language specification:

<spec>
`)
	b.WriteString(spec)
	b.WriteString(`
</spec>

Here is the input document:

<input>
`)
	b.Write(inputData)
	b.WriteString(`</input>

Here is the expected output document:

<output>
`)
	b.Write(outputData)
	b.WriteString(`</output>

The input and output use a JSON envelope format: {"value": <document>, "metadata": {...}}.
The mapping's "root" corresponds to the envelope's "value" field — the document itself.
Metadata assignments in the mapping correspond to the envelope's "metadata" field.
For example, if the input value is null and the expected output value is false, the mapping is simply: root = false

Write a mapping in this language that transforms the input into the expected output.
Respond with ONLY the mapping code.
Do not include any explanation, commentary, or code fences.
`)
	return b.String(), nil
}

// extractJSON finds the outermost JSON object in the agent's response,
// stripping markdown code fences and surrounding text.
func extractJSON(raw string) (map[string]any, error) {
	s := stripCodeFences(strings.TrimSpace(raw))

	// Find the outermost { ... }.
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return nil, errors.New("no JSON object found in response")
	}
	end := strings.LastIndexByte(s, '}')
	if end < 0 || end <= start {
		return nil, errors.New("no closing brace found in response")
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(s[start:end+1]), &obj); err != nil {
		return nil, fmt.Errorf("parsing JSON from response: %w", err)
	}
	return obj, nil
}

// extractMapping extracts mapping source code from the agent's response,
// stripping markdown code fences if present.
func extractMapping(raw string) string {
	return strings.TrimSpace(stripCodeFences(strings.TrimSpace(raw)))
}

// stripCodeFences removes the outermost markdown code fence if the text is
// wrapped in one. Handles ```lang\n...\n``` and bare ```\n...\n```.
func stripCodeFences(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}

	// Find end of opening fence line.
	firstNL := strings.IndexByte(s, '\n')
	if firstNL < 0 {
		return s
	}

	// Find closing fence.
	if !strings.HasSuffix(s, "```") {
		return s
	}

	inner := s[firstNL+1 : len(s)-3]
	return strings.TrimSpace(inner)
}
