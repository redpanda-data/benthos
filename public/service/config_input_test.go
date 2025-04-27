// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigInput(t *testing.T) {
	spec := NewConfigSpec().
		Field(NewInputField("a"))

	parsedConfig, err := spec.ParseYAML(`
a:
  generate:
    count: 1
    interval: ""
    mapping: 'root = "hello world"'
`, nil)
	require.NoError(t, err)

	input, err := parsedConfig.FieldInput("a")
	require.NoError(t, err)

	res, aFn, err := input.ReadBatch(t.Context())
	require.NoError(t, err)
	require.Len(t, res, 1)

	require.NoError(t, aFn(t.Context(), nil))

	resBytes, err := res[0].AsBytes()
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(resBytes))

	_, _, err = input.ReadBatch(t.Context())
	require.Equal(t, ErrEndOfInput, err)

	require.NoError(t, input.Close(t.Context()))
}

func TestConfigInputList(t *testing.T) {
	spec := NewConfigSpec().
		Field(NewInputListField("a"))

	parsedConfig, err := spec.ParseYAML(`
a:
  - generate:
      count: 1
      interval: ""
      mapping: 'root = "hello world"'
  - generate:
      count: 1
      interval: ""
      mapping: 'root = "hello world two"'
`, nil)
	require.NoError(t, err)

	inputs, err := parsedConfig.FieldInputList("a")
	require.NoError(t, err)
	require.Len(t, inputs, 2)

	res, aFn, err := inputs[0].ReadBatch(t.Context())
	require.NoError(t, err)
	require.Len(t, res, 1)

	require.NoError(t, aFn(t.Context(), nil))

	resBytes, err := res[0].AsBytes()
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(resBytes))

	_, _, err = inputs[0].ReadBatch(t.Context())
	require.Equal(t, ErrEndOfInput, err)

	res, aFn, err = inputs[1].ReadBatch(t.Context())
	require.NoError(t, err)
	require.Len(t, res, 1)

	require.NoError(t, aFn(t.Context(), nil))

	resBytes, err = res[0].AsBytes()
	require.NoError(t, err)
	assert.Equal(t, "hello world two", string(resBytes))

	_, _, err = inputs[1].ReadBatch(t.Context())
	require.Equal(t, ErrEndOfInput, err)

	require.NoError(t, inputs[0].Close(t.Context()))
	require.NoError(t, inputs[1].Close(t.Context()))
}
