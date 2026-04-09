package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func marshalValue(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func buildReadPrompt(spec string, test eligibleTest) (string, error) {
	inputValue, err := marshalValue(test.Input)
	if err != nil {
		return "", fmt.Errorf("marshaling input: %w", err)
	}

	hasMeta := len(test.InputMeta) > 0

	var b strings.Builder
	b.WriteString(`Here is a mapping language specification:

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

Here is the input document (as JSON):

<input>
`)
	b.Write(inputValue)
	b.WriteString("</input>\n")

	if hasMeta {
		inputMeta, err := marshalValue(test.InputMeta)
		if err != nil {
			return "", fmt.Errorf("marshaling input metadata: %w", err)
		}
		b.WriteString("\nThe input document has this metadata:\n\n<input_metadata>\n")
		b.Write(inputMeta)
		b.WriteString("</input_metadata>\n")
	}

	b.WriteString(`
Execute the mapping against the input and produce the output.
Respond with ONLY the output value as JSON. Do not wrap it in any envelope or object structure. **IMPORTANT** Do not respond with anything other than the output!
Do not include any explanation, commentary, or code fences.
`)

	if !test.NoMetadataCheck {
		b.WriteString("If the mapping assigns metadata, write a second line starting with \"Metadata: \" followed by the metadata as a JSON object.\n")
	}

	b.WriteString("Do not include any explanation, commentary, or code fences.\n")
	return b.String(), nil
}

func buildWritePrompt(spec string, test eligibleTest) (string, error) {
	inputValue, err := marshalValue(test.Input)
	if err != nil {
		return "", fmt.Errorf("marshaling input: %w", err)
	}
	expectedValue, err := marshalValue(test.Expected.Value)
	if err != nil {
		return "", fmt.Errorf("marshaling expected output: %w", err)
	}

	hasMeta := len(test.InputMeta) > 0
	hasExpectedMeta := len(test.Expected.Metadata) > 0

	var b strings.Builder
	b.WriteString(`Here is a mapping language specification:

<spec>
`)
	b.WriteString(spec)
	b.WriteString(`
</spec>

Here is the input document (as JSON):

<input>
`)
	b.Write(inputValue)
	b.WriteString("</input>\n")

	if hasMeta {
		inputMeta, err := marshalValue(test.InputMeta)
		if err != nil {
			return "", fmt.Errorf("marshaling input metadata: %w", err)
		}
		b.WriteString("\nThe input document has this metadata:\n\n<input_metadata>\n")
		b.Write(inputMeta)
		b.WriteString("</input_metadata>\n")
	}

	b.WriteString("\nHere is the expected output document (as JSON):\n\n<output>\n")
	b.Write(expectedValue)
	b.WriteString("</output>\n")

	if hasExpectedMeta {
		expectedMeta, err := marshalValue(test.Expected.Metadata)
		if err != nil {
			return "", fmt.Errorf("marshaling expected metadata: %w", err)
		}
		b.WriteString("\nThe expected output has this metadata:\n\n<output_metadata>\n")
		b.Write(expectedMeta)
		b.WriteString("</output_metadata>\n")
	}

	b.WriteString(`
Write a mapping in this language that transforms the input into the expected output.
The mapping operates on the document directly — "output" IS the document, not a wrapper around it.
For example, if the input is null and the expected output is false, the correct mapping is simply: output = false
Respond with ONLY the mapping code. **IMPORTANT** Do not respond with anything other than the mapping!
Do not include any explanation, commentary, or code fences.
`)
	return b.String(), nil
}

// extractValue parses an arbitrary JSON value from the agent's response,
// stripping markdown code fences and surrounding text.
func extractValue(raw string) (any, error) {
	s := strings.TrimSpace(stripCodeFences(strings.TrimSpace(raw)))

	// Split off a potential "Metadata: {...}" trailing line.
	s, _ = splitMetadataLine(s)
	s = strings.TrimSpace(s)

	if s == "" {
		return nil, errors.New("empty response")
	}

	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("parsing JSON value from response: %w", err)
	}
	return v, nil
}

// extractMetadata looks for a "Metadata: {...}" line in the agent's response.
// Returns nil if not found.
func extractMetadata(raw string) map[string]any {
	s := strings.TrimSpace(stripCodeFences(strings.TrimSpace(raw)))
	_, metaLine := splitMetadataLine(s)
	if metaLine == "" {
		return nil
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(metaLine), &m); err != nil {
		return nil
	}
	return m
}

// splitMetadataLine splits the response into the main value and an optional
// metadata JSON string. Looks for the last line starting with "Metadata:" (case-insensitive).
func splitMetadataLine(s string) (value, meta string) {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "metadata:") {
			metaJSON := strings.TrimSpace(trimmed[len("metadata:"):])
			valuePart := strings.Join(lines[:i], "\n")
			return valuePart, metaJSON
		}
	}
	return s, ""
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
