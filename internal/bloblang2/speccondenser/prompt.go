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
Execute the mapping against the input and produce the output document.
The output document starts as an empty object {}. The mapping populates it via output assignments (e.g. "output.x = ..." creates a key "x" in the output document).
Respond with the complete output document as JSON. For example, if the mapping does "output.x = 1" and "output.y = 2", respond with {"x":1,"y":2}.
**IMPORTANT** Do not respond with anything other than the output document!
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
Prefer assigning each output field on its own line (e.g. output.x = expr) rather than building a single large object literal.
Respond with ONLY the mapping code. **IMPORTANT** Do not respond with anything other than the mapping!
Do not include any explanation, commentary, or code fences.
`)
	return b.String(), nil
}

func buildGroupWritePrompt(spec string, group writeTestGroup) (string, error) {
	// Single-case groups use the same layout as the original buildWritePrompt.
	if len(group.Cases) == 1 {
		return buildWritePrompt(spec, group.Cases[0])
	}

	var b strings.Builder
	b.WriteString("Here is a mapping language specification:\n\n<spec>\n")
	b.WriteString(spec)
	b.WriteString("\n</spec>\n\n")
	b.WriteString(fmt.Sprintf("Write a SINGLE mapping that transforms each input into its expected output.\nThe mapping must handle ALL %d cases below.\n", len(group.Cases)))

	for i, c := range group.Cases {
		inputValue, err := marshalValue(c.Input)
		if err != nil {
			return "", fmt.Errorf("case %d: marshaling input: %w", i, err)
		}
		expectedValue, err := marshalValue(c.Expected.Value)
		if err != nil {
			return "", fmt.Errorf("case %d: marshaling expected output: %w", i, err)
		}

		caseName := c.Name
		if j := strings.Index(caseName, "/"); j >= 0 {
			caseName = caseName[j+1:]
		}

		b.WriteString(fmt.Sprintf("\n### Case %d: %s\n\n<input>\n", i+1, caseName))
		b.Write(inputValue)
		b.WriteString("</input>\n")

		if len(c.InputMeta) > 0 {
			inputMeta, err := marshalValue(c.InputMeta)
			if err != nil {
				return "", fmt.Errorf("case %d: marshaling input metadata: %w", i, err)
			}
			b.WriteString("\n<input_metadata>\n")
			b.Write(inputMeta)
			b.WriteString("</input_metadata>\n")
		}

		b.WriteString("\n<expected_output>\n")
		b.Write(expectedValue)
		b.WriteString("</expected_output>\n")

		if len(c.Expected.Metadata) > 0 {
			expectedMeta, err := marshalValue(c.Expected.Metadata)
			if err != nil {
				return "", fmt.Errorf("case %d: marshaling expected metadata: %w", i, err)
			}
			b.WriteString("\n<expected_output_metadata>\n")
			b.Write(expectedMeta)
			b.WriteString("</expected_output_metadata>\n")
		}
	}

	b.WriteString(`
The mapping operates on the document directly — "output" IS the document, not a wrapper around it.
For example, if the input is null and the expected output is false, the correct mapping is simply: output = false
Prefer assigning each output field on its own line (e.g. output.x = expr) rather than building a single large object literal.
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
