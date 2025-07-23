// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSchemaStringify(t *testing.T) {
	var zeroType CommonType

	for _, test := range []struct {
		Input  CommonType
		Output string
	}{
		{Input: Boolean, Output: "BOOLEAN"},
		{Input: Int64, Output: "INT64"},
		{Input: Int32, Output: "INT32"},
		{Input: Float32, Output: "FLOAT32"},
		{Input: Float64, Output: "FLOAT64"},
		{Input: String, Output: "STRING"},
		{Input: ByteArray, Output: "BYTE_ARRAY"},
		{Input: Object, Output: "OBJECT"},
		{Input: Map, Output: "MAP"},
		{Input: Array, Output: "ARRAY"},
		{Input: Null, Output: "NULL"},
		{Input: Union, Output: "UNION"},
		{Input: zeroType, Output: "UNKNOWN"},
		{Input: CommonType(-1), Output: "UNKNOWN"},
	} {
		assert.Equal(t, test.Input.String(), test.Output)
	}
}
