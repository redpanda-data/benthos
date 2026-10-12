// Copyright 2025 Redpanda Data, Inc.

package cli_test

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	icli "github.com/redpanda-data/benthos/v4/internal/cli"
	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/public/service"

	_ "github.com/redpanda-data/benthos/v4/public/components/io"
	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func executeListSubcmd(args []string) (string, error) {
	var buf bytes.Buffer

	opts := common.NewCLIOpts("1.2.3", "now")
	opts.Stdout = &buf
	opts.Stderr = &buf

	err := icli.App(opts).Run(args)
	return buf.String(), err
}

func TestListBloblangFunctionsJSONSchema(t *testing.T) {
	outStr, err := executeListSubcmd([]string{"benthos", "list", "--format", "jsonschema", "bloblang-functions"})
	require.NoError(t, err)

	// Parse JSON output
	var result map[string][]map[string]any
	err = json.Unmarshal([]byte(outStr), &result)
	require.NoError(t, err, "Output should be valid JSON")

	// Verify structure
	functions, ok := result["bloblang-functions"]
	require.True(t, ok, "Output should contain 'bloblang-functions' key")
	require.NotEmpty(t, functions, "Should have at least one function")

	// Verify first function has expected fields
	firstFunc := functions[0]
	assert.Contains(t, firstFunc, "name", "Function should have 'name' field")
	assert.Contains(t, firstFunc, "description", "Function should have 'description' field")
	assert.Contains(t, firstFunc, "status", "Function should have 'status' field")
	assert.Contains(t, firstFunc, "category", "Function should have 'category' field")
	assert.Contains(t, firstFunc, "params", "Function should have 'params' field")
	assert.Contains(t, firstFunc, "examples", "Function should have 'examples' field")
	assert.Contains(t, firstFunc, "impure", "Function should have 'impure' field")

	// Find and verify specific function (uuid_v4)
	var uuidFunc map[string]any
	for _, fn := range functions {
		if fn["name"] == "uuid_v4" {
			uuidFunc = fn
			break
		}
	}
	require.NotNil(t, uuidFunc, "Should find uuid_v4 function")
	assert.Equal(t, "stable", uuidFunc["status"])
	assert.Equal(t, "General", uuidFunc["category"])
	assert.Contains(t, uuidFunc["description"], "UUID")
	assert.False(t, uuidFunc["impure"].(bool))
}

func TestListBloblangMethodsJSONSchema(t *testing.T) {
	outStr, err := executeListSubcmd([]string{"benthos", "list", "--format", "jsonschema", "bloblang-methods"})
	require.NoError(t, err)

	// Parse JSON output
	var result map[string][]map[string]any
	err = json.Unmarshal([]byte(outStr), &result)
	require.NoError(t, err, "Output should be valid JSON")

	// Verify structure
	methods, ok := result["bloblang-methods"]
	require.True(t, ok, "Output should contain 'bloblang-methods' key")
	require.NotEmpty(t, methods, "Should have at least one method")

	// Verify first method has expected fields
	firstMethod := methods[0]
	assert.Contains(t, firstMethod, "name", "Method should have 'name' field")
	assert.Contains(t, firstMethod, "status", "Method should have 'status' field")
	assert.Contains(t, firstMethod, "params", "Method should have 'params' field")
	assert.Contains(t, firstMethod, "categories", "Method should have 'categories' field")
	assert.Contains(t, firstMethod, "impure", "Method should have 'impure' field")

	// Find and verify specific method (uppercase)
	var uppercaseMethod map[string]any
	for _, method := range methods {
		if method["name"] == "uppercase" {
			uppercaseMethod = method
			break
		}
	}
	require.NotNil(t, uppercaseMethod, "Should find uppercase method")
	assert.Equal(t, "stable", uppercaseMethod["status"])
	assert.False(t, uppercaseMethod["impure"].(bool))

	// Verify categories is an array
	categories, ok := uppercaseMethod["categories"].([]any)
	require.True(t, ok, "Categories should be an array")
	require.NotEmpty(t, categories, "Should have at least one category")
}

func TestListBloblangFunctionWithParams(t *testing.T) {
	outStr, err := executeListSubcmd([]string{"benthos", "list", "--format", "jsonschema", "bloblang-functions"})
	require.NoError(t, err)

	// Parse JSON output
	var result map[string][]map[string]any
	err = json.Unmarshal([]byte(outStr), &result)
	require.NoError(t, err)

	functions := result["bloblang-functions"]

	// Find nanoid function which has optional parameters
	var nanoidFunc map[string]any
	for _, fn := range functions {
		if fn["name"] == "nanoid" {
			nanoidFunc = fn
			break
		}
	}
	require.NotNil(t, nanoidFunc, "Should find nanoid function")

	// Verify params structure
	params, ok := nanoidFunc["params"].(map[string]any)
	require.True(t, ok, "Params should be a map")

	named, ok := params["named"].([]any)
	require.True(t, ok, "Named params should be an array")
	require.NotEmpty(t, named, "Should have named parameters")

	// Verify first parameter has required fields
	firstParam := named[0].(map[string]any)
	assert.Contains(t, firstParam, "name", "Parameter should have 'name' field")
	assert.Contains(t, firstParam, "description", "Parameter should have 'description' field")
	assert.Contains(t, firstParam, "type", "Parameter should have 'type' field")
	assert.Contains(t, firstParam, "is_optional", "Parameter should have 'is_optional' field")
}

func TestListBloblangMethodWithParams(t *testing.T) {
	outStr, err := executeListSubcmd([]string{"benthos", "list", "--format", "jsonschema", "bloblang-methods"})
	require.NoError(t, err)

	// Parse JSON output
	var result map[string][]map[string]any
	err = json.Unmarshal([]byte(outStr), &result)
	require.NoError(t, err)

	methods := result["bloblang-methods"]

	// Find catch method which has parameters
	var catchMethod map[string]any
	for _, method := range methods {
		if method["name"] == "catch" {
			catchMethod = method
			break
		}
	}
	require.NotNil(t, catchMethod, "Should find catch method")

	// Verify params structure
	params, ok := catchMethod["params"].(map[string]any)
	require.True(t, ok, "Params should be a map")

	named, ok := params["named"].([]any)
	require.True(t, ok, "Named params should be an array")
	require.NotEmpty(t, named, "Should have named parameters")

	// Verify parameter structure
	firstParam := named[0].(map[string]any)
	assert.Equal(t, "fallback", firstParam["name"])
	assert.Equal(t, "query expression", firstParam["type"])
	assert.Contains(t, firstParam["description"], "query")

	// Verify examples
	examples, ok := catchMethod["examples"].([]any)
	require.True(t, ok, "Examples should be an array")
	require.NotEmpty(t, examples, "Should have examples")
}

func TestListBloblangFunctionFiltering(t *testing.T) {
	// Test filtering to a single function
	outStr, err := executeListSubcmd([]string{"benthos", "list", "--format", "jsonschema", "bloblang-functions", "uuid_v4"})
	require.NoError(t, err)

	// Parse JSON output
	var result map[string][]map[string]any
	err = json.Unmarshal([]byte(outStr), &result)
	require.NoError(t, err)

	functions := result["bloblang-functions"]
	require.Len(t, functions, 1, "Should return exactly one function")
	assert.Equal(t, "uuid_v4", functions[0]["name"])

	// Test filtering to multiple functions
	outStr, err = executeListSubcmd([]string{"benthos", "list", "--format", "jsonschema", "bloblang-functions", "uuid_v4", "nanoid"})
	require.NoError(t, err)

	err = json.Unmarshal([]byte(outStr), &result)
	require.NoError(t, err)

	functions = result["bloblang-functions"]
	require.Len(t, functions, 2, "Should return exactly two functions")

	names := []string{functions[0]["name"].(string), functions[1]["name"].(string)}
	assert.Contains(t, names, "uuid_v4")
	assert.Contains(t, names, "nanoid")
}

func TestListBloblangMethodFiltering(t *testing.T) {
	// Test filtering to a single method
	outStr, err := executeListSubcmd([]string{"benthos", "list", "--format", "jsonschema", "bloblang-methods", "uppercase"})
	require.NoError(t, err)

	// Parse JSON output
	var result map[string][]map[string]any
	err = json.Unmarshal([]byte(outStr), &result)
	require.NoError(t, err)

	methods := result["bloblang-methods"]
	require.Len(t, methods, 1, "Should return exactly one method")
	assert.Equal(t, "uppercase", methods[0]["name"])

	// Test filtering to multiple methods
	outStr, err = executeListSubcmd([]string{"benthos", "list", "--format", "jsonschema", "bloblang-methods", "uppercase", "catch"})
	require.NoError(t, err)

	err = json.Unmarshal([]byte(outStr), &result)
	require.NoError(t, err)

	methods = result["bloblang-methods"]
	require.Len(t, methods, 2, "Should return exactly two methods")

	names := []string{methods[0]["name"].(string), methods[1]["name"].(string)}
	assert.Contains(t, names, "uppercase")
	assert.Contains(t, names, "catch")
}

func TestListJSONSchemaComponentsNotAffected(t *testing.T) {
	// Verify that regular component jsonschema output still works
	outStr, err := executeListSubcmd([]string{"benthos", "list", "--format", "jsonschema"})
	require.NoError(t, err)

	// Should be a JSON schema document (not our bloblang metadata format)
	var result map[string]any
	err = json.Unmarshal([]byte(outStr), &result)
	require.NoError(t, err, "Output should be valid JSON")

	// Should NOT have bloblang-functions or bloblang-methods keys
	_, hasFunctions := result["bloblang-functions"]
	_, hasMethods := result["bloblang-methods"]
	assert.False(t, hasFunctions, "Component schema should not have bloblang-functions")
	assert.False(t, hasMethods, "Component schema should not have bloblang-methods")

	// Should have typical JSON schema fields (definitions and properties)
	assert.Contains(t, result, "definitions", "Should be a JSON schema document with definitions")
	assert.Contains(t, result, "properties", "Should be a JSON schema document with properties")
}

// Every doc string renders without AsciiDoc left over, so consumers of
// --docs-format markdown never see xrefs, anchors, conditionals, or block
// delimiters. Config snippets, mappings, and example results are code and
// left alone.
func TestListJSONFullMarkdownHasNoAsciidoc(t *testing.T) {
	outStr, err := executeListSubcmd([]string{"benthos", "list", "--format", "json-full", "--docs-format", "markdown"})
	require.NoError(t, err)

	var doc any
	require.NoError(t, json.Unmarshal([]byte(outStr), &doc))

	leftovers := map[string]*regexp.Regexp{
		"xref":        regexp.MustCompile(`xref:`),
		"anchor":      regexp.MustCompile(`<<[^\s<]|[^\s>]>>`),
		"conditional": regexp.MustCompile(`(?m)^(ifn?def|endif)::`),
		"delimiter":   regexp.MustCompile(`(?m)^(====|----|\|===|\[tabs\])$`),
		"link macro":  regexp.MustCompile(`https?://[^\s)]+\[`),
		"raw target":  regexp.MustCompile(`\.adoc\]\(`),
		"bold":        regexp.MustCompile(`(^|\s)\*[^*\s][^*\n]*[^*\s]\*(\s|$)`),
	}
	code := map[string]bool{"config": true, "mapping": true, "results": true, "examples": true, "default": true}

	tt := t
	var walk func(v any, path, key string)
	walk = func(v any, path, key string) {
		if code[key] {
			return
		}
		switch t := v.(type) {
		case string:
			s := t
			for name, rx := range leftovers {
				assert.False(tt, rx.MatchString(s), "%v: leftover %v in %q", path, name, s)
			}
		case []any:
			for i, e := range t {
				walk(e, path+"/"+strconv.Itoa(i), key)
			}
		case map[string]any:
			for k, e := range t {
				walk(e, path+"/"+k, k)
			}
		}
	}
	walk(doc, "", "")
}

// Every raw doc string follows the one supported conditional form, so a
// malformed conditional fails here rather than dropping text for readers.
func TestListJSONFullConditionalsAreWellFormed(t *testing.T) {
	outStr, err := executeListSubcmd([]string{"benthos", "list", "--format", "json-full"})
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(outStr), &doc))
	assert.Equal(t, "asciidoc", doc["docs_format"])

	code := map[string]bool{"config": true, "mapping": true, "results": true, "default": true}
	var walk func(v any, path, key string)
	walk = func(v any, path, key string) {
		if code[key] {
			return
		}
		switch x := v.(type) {
		case string:
			for _, p := range service.LintDocsConditionals(x) {
				assert.Fail(t, "malformed conditional", "%v: %v", path, p)
			}
			if key == "summary" && (strings.Contains(x, "ifdef::") || strings.Contains(x, "ifndef::")) {
				assert.Fail(t, "conditional in a summary", "%v: summaries render as one line", path)
			}
		case []any:
			for i, e := range x {
				walk(e, path+"/"+strconv.Itoa(i), key)
			}
		case map[string]any:
			for k, e := range x {
				walk(e, path+"/"+k, k)
			}
		}
	}
	walk(doc, "", "")
}

func TestListJSONFullMarkdownRejectsMalformedConditionals(t *testing.T) {
	assert.Contains(t, service.LintDocsConditionals("ifndef::env-cloud[]\ntext")[0], "never closed")
}

func TestListJSONFullMarkdownResolvesLinksAndConditionals(t *testing.T) {
	outStr, err := executeListSubcmd([]string{"benthos", "list", "--format", "json-full", "--docs-format", "markdown", "inputs"})
	require.NoError(t, err)
	assert.Contains(t, outStr, "](https://benthos.dev/docs/components/rate_limits/about/)")

	assert.Contains(t, outStr, `"docs_format":"markdown"`)

	raw, err := executeListSubcmd([]string{"benthos", "list", "--format", "json-full", "inputs"})
	require.NoError(t, err)
	assert.Contains(t, raw, "xref:components:rate_limits/about.adoc", "asciidoc stays the default")

	_, err = executeListSubcmd([]string{"benthos", "list", "--format", "json-full", "--docs-format", "html"})
	require.Error(t, err)
}
