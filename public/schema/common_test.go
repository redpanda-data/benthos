// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{Input: Timestamp, Output: "TIMESTAMP"},
		{Input: Any, Output: "ANY"},
		{Input: Decimal, Output: "DECIMAL"},
		{Input: BigDecimal, Output: "BIG_DECIMAL"},
		{Input: zeroType, Output: "UNKNOWN"},
		{Input: CommonType(-1), Output: "UNKNOWN"},
	} {
		assert.Equal(t, test.Input.String(), test.Output)
	}
}

func TestValidateRejectsChildrenOnLeafTypes(t *testing.T) {
	leafTypes := []CommonType{
		Boolean, Int32, Int64, Float32, Float64, String, ByteArray,
		Null, Timestamp, Any, BigDecimal,
	}

	for _, typ := range leafTypes {
		t.Run(typ.String(), func(t *testing.T) {
			c := Common{
				Type:     typ,
				Name:     "x",
				Children: []Common{{Type: String, Name: "weird"}},
			}
			err := c.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "is a leaf and must not have children")
		})
	}
}

func TestValidateAllowsChildrenOnContainerTypes(t *testing.T) {
	containers := []CommonType{Object, Map, Array, Union}

	for _, typ := range containers {
		t.Run(typ.String(), func(t *testing.T) {
			c := Common{
				Type:     typ,
				Name:     "x",
				Children: []Common{{Type: String, Name: "field"}},
			}
			assert.NoError(t, c.Validate())
		})
	}
}

func TestValidateRejectsChildrenOnDecimal(t *testing.T) {
	c := Common{
		Type:     Decimal,
		Name:     "amount",
		Logical:  &LogicalParams{Decimal: &DecimalParams{Precision: 10, Scale: 2}},
		Children: []Common{{Type: String, Name: "weird"}},
	}
	err := c.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a leaf and must not have children")
}
