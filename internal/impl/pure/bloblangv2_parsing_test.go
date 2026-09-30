// Copyright 2026 Redpanda Data, Inc.

package pure_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func TestBloblangV2ParseYAML(t *testing.T) {
	got := runBloblangV2(t, `output = input.parse_yaml()`, "foo: bar\nbaz: 42\n")
	assert.Equal(t, map[string]any{"foo": "bar", "baz": int64(42)}, got)
}

func TestBloblangV2FormatYAML(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.format_yaml().string()`,
		map[string]any{"foo": "bar"},
	)
	assert.Equal(t, "foo: bar\n", got)
}

func TestBloblangV2ParseCSVHeaderRow(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.parse_csv()`,
		"name,age\nalice,30\nbob,40",
	)
	expected := []any{
		map[string]any{"name": "alice", "age": "30"},
		map[string]any{"name": "bob", "age": "40"},
	}
	assert.Equal(t, expected, got)
}

func TestBloblangV2ParseCSVNoHeader(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.parse_csv(parse_header_row: false)`,
		"a,b\nc,d",
	)
	expected := []any{
		[]any{"a", "b"},
		[]any{"c", "d"},
	}
	assert.Equal(t, expected, got)
}

func TestBloblangV2ParseCSVCustomDelimiter(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.parse_csv(delimiter: "|")`,
		"foo|bar\n1|2",
	)
	expected := []any{
		map[string]any{"foo": "1", "bar": "2"},
	}
	assert.Equal(t, expected, got)
}

func TestBloblangV2ParseCSVRejectsMultiCharDelimiter(t *testing.T) {
	// Named args bypass V2's static-arg folding so the constructor runs per
	// query rather than at parse time. The error therefore surfaces from
	// Query, not Parse.
	exec, err := bloblangv2.GlobalEnvironment().Parse(`output = input.parse_csv(delimiter: "::")`)
	require.NoError(t, err)
	_, err = exec.Query("a,b\n1,2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one character")
}

func TestBloblangV2JSONSchemaPasses(t *testing.T) {
	schema := `{\"type\":\"object\",\"properties\":{\"name\":{\"type\":\"string\"}}}`
	mapping := `output = input.json_schema("` + schema + `")`
	got := runBloblangV2(t, mapping, map[string]any{"name": "alice"})
	assert.Equal(t, map[string]any{"name": "alice"}, got)
}

func TestBloblangV2JSONSchemaFails(t *testing.T) {
	schema := `{\"type\":\"object\",\"properties\":{\"name\":{\"type\":\"string\"}},\"required\":[\"name\"]}`
	mapping := `output = input.json_schema("` + schema + `")`
	exec, err := bloblangv2.GlobalEnvironment().Parse(mapping)
	require.NoError(t, err)
	_, err = exec.Query(map[string]any{"age": int64(30)})
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "name") ||
			strings.Contains(err.Error(), "required"),
		"expected error mentioning name or required, got: %v", err,
	)
}
