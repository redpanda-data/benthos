// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigQuerier(t *testing.T) {
	env := NewEnvironment()
	schema := env.CoreConfigSchema("", "")

	querier := schema.NewConfigQuerier()
	require.NotNil(t, querier)

	assert.NotNil(t, querier.env)
	assert.NotNil(t, querier.res)
	assert.NotNil(t, querier.spec)
	assert.NotNil(t, querier.envVarLookupFn)
}

func TestConfigQuerierSetResources(t *testing.T) {
	env := NewEnvironment()
	schema := env.CoreConfigSchema("", "")

	querier := schema.NewConfigQuerier()

	// Create new resources
	newRes := MockResources()

	// Set new resources
	querier.SetResources(newRes)

	// Verify resources were set
	assert.Equal(t, newRes, querier.res)
}

func TestConfigQuerierSetEnvVarLookupFunc(t *testing.T) {
	env := NewEnvironment()
	schema := env.CoreConfigSchema("", "")

	querier := schema.NewConfigQuerier()

	// Track whether custom lookup was called
	lookupCalled := false

	// Create custom lookup function
	customLookup := func(_ context.Context, k string) (string, bool) {
		lookupCalled = true
		if k == "TEST_VAR" {
			return "lines", true
		}
		return "", false
	}

	// Set custom lookup function
	querier.SetEnvVarLookupFunc(customLookup)

	// Test that env var is resolved using custom function
	confStr := `
input:
  stdin: {}
output:
  stdout:
    codec: ${TEST_VAR}
`
	queryFile, err := querier.ParseYAML(confStr)
	require.NoError(t, err)
	require.NotNil(t, queryFile)

	// Verify the custom lookup was called
	assert.True(t, lookupCalled)
}

func TestConfigQuerierParseYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "simple valid config",
			yaml: `
input:
  stdin: {}
output:
  stdout: {}
`,
			wantErr: false,
		},
		{
			name: "config with pipeline",
			yaml: `
input:
  stdin: {}
pipeline:
  processors:
    - bloblang: 'root = content().uppercase()'
output:
  stdout: {}
`,
			wantErr: false,
		},
		{
			name: "invalid yaml",
			yaml: `
input: [unclosed
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := NewEnvironment()
			schema := env.CoreConfigSchema("", "")
			querier := schema.NewConfigQuerier()

			queryFile, err := querier.ParseYAML(tt.yaml)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, queryFile)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, queryFile)
			}
		})
	}
}

func TestConfigQuerierParseYAMLWithEnvVars(t *testing.T) {
	env := NewEnvironment()
	schema := env.CoreConfigSchema("", "")

	querier := schema.NewConfigQuerier()

	// Set custom env var lookup
	querier.SetEnvVarLookupFunc(func(_ context.Context, k string) (string, bool) {
		envVars := map[string]string{
			"INPUT_TYPE":  "stdin",
			"OUTPUT_TYPE": "stdout",
		}
		val, ok := envVars[k]
		return val, ok
	})

	confStr := `
input:
  ${INPUT_TYPE}: {}
output:
  ${OUTPUT_TYPE}: {}
`

	queryFile, err := querier.ParseYAML(confStr)
	require.NoError(t, err)
	require.NotNil(t, queryFile)
}

func TestConfigQuerierParseYAMLWithMissingEnvVars(t *testing.T) {
	env := NewEnvironment()
	schema := env.CoreConfigSchema("", "")

	querier := schema.NewConfigQuerier()

	// Set env var lookup that always returns false
	querier.SetEnvVarLookupFunc(func(_ context.Context, k string) (string, bool) {
		return "", false
	})

	confStr := `
input:
  stdin: {}
output:
  stdout:
    codec: ${MISSING_VAR}
`

	// Should not error, but use best attempt (the literal string with env var)
	queryFile, err := querier.ParseYAML(confStr)
	require.NoError(t, err)
	require.NotNil(t, queryFile)
}

func TestConfigQueryFileFieldAtPath(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		path    []string
		wantErr bool
		check   func(t *testing.T, field *ParsedConfig)
	}{
		{
			name: "input field",
			yaml: `
input:
  stdin: {}
output:
  stdout: {}
`,
			path:    []string{"input"},
			wantErr: false,
			check: func(t *testing.T, field *ParsedConfig) {
				assert.NotNil(t, field)
			},
		},
		{
			name: "output field",
			yaml: `
input:
  stdin: {}
output:
  stdout: {}
`,
			path:    []string{"output"},
			wantErr: false,
			check: func(t *testing.T, field *ParsedConfig) {
				assert.NotNil(t, field)
			},
		},
		{
			name: "non-existent top level field",
			yaml: `
input:
  stdin: {}
output:
  stdout: {}
`,
			path:    []string{"nonexistent"},
			wantErr: true,
		},
	}

	env := NewEnvironment()
	schema := env.CoreConfigSchema("", "")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			querier := schema.NewConfigQuerier()

			queryFile, err := querier.ParseYAML(tt.yaml)
			require.NoError(t, err)

			field, err := queryFile.FieldAtPath(tt.path...)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, field)
			} else {
				require.NoError(t, err)
				require.NotNil(t, field)
				if tt.check != nil {
					tt.check(t, field)
				}
			}
		})
	}
}

func TestConfigQueryFileFieldAtPathWithDifferentTypes(t *testing.T) {
	yaml := `
input:
  stdin: {}
pipeline:
  threads: 4
  processors:
    - bloblang: 'root = content().uppercase()'
output:
  stdout: {}
`

	env := NewEnvironment()
	schema := env.CoreConfigSchema("", "")

	querier := schema.NewConfigQuerier()

	queryFile, err := querier.ParseYAML(yaml)
	require.NoError(t, err)

	// Test object field
	inputField, err := queryFile.FieldAtPath("input")
	require.NoError(t, err)
	assert.NotNil(t, inputField)

	// Test output field
	outputField, err := queryFile.FieldAtPath("output")
	require.NoError(t, err)
	assert.NotNil(t, outputField)

	// Test pipeline field
	pipelineField, err := queryFile.FieldAtPath("pipeline")
	require.NoError(t, err)
	assert.NotNil(t, pipelineField)
}

func TestConfigQueryFileWithResources(t *testing.T) {
	env := NewEnvironment()
	schema := env.CoreConfigSchema("", "")

	querier := schema.NewConfigQuerier()

	// Set custom resources
	customRes := MockResources()
	querier.SetResources(customRes)

	yaml := `
input:
  stdin: {}
output:
  stdout: {}
`

	queryFile, err := querier.ParseYAML(yaml)
	require.NoError(t, err)
	require.NotNil(t, queryFile)

	// Verify that the query file uses the custom resources
	assert.Equal(t, customRes, queryFile.res)
}
