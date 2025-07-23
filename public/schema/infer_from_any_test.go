// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromAnySchema(t *testing.T) {
	for _, test := range []struct {
		Name        string
		Input       any
		Output      Common
		ErrContains string
	}{
		{
			Name:  "Valid scalar schema",
			Input: 10,
			Output: Common{
				Type: Int64,
			},
		},
		{
			Name: "Valid flat object schema",
			Input: map[string]any{
				"foo":   "hello world",
				"bar":   int32(11),
				"baz":   float32(1.1),
				"buz":   float64(1.2),
				"moo":   true,
				"meow":  time.Now().Add(time.Second),
				"quack": nil,
			},
			Output: Common{
				Type: Object,
				Children: []Common{
					{Name: "bar", Type: Int32},
					{Name: "baz", Type: Float32},
					{Name: "buz", Type: Float64},
					{Name: "foo", Type: String},
					{Name: "meow", Type: Timestamp},
					{Name: "moo", Type: Boolean},
					{Name: "quack", Type: Null},
				},
			},
		},
		{
			Name: "Valid nested object schema",
			Input: map[string]any{
				"foo": map[string]any{
					"bar": []any{
						[]any{
							map[string]any{
								"baz": []any{10},
							},
						},
					},
				},
			},
			Output: Common{
				Type: Object,
				Children: []Common{
					{
						Name: "foo",
						Type: Object,
						Children: []Common{
							{
								Name: "bar",
								Type: Array,
								Children: []Common{
									{
										Type: Array,
										Children: []Common{
											{
												Type: Object,
												Children: []Common{
													{
														Name: "baz",
														Type: Array,
														Children: []Common{
															{
																Type: Int64,
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "Invalid deeply nested unsupported type",
			Input: map[string]any{
				"foo": map[string]any{
					"bar": []any{
						[]any{
							map[string]any{
								"baz": []any{uint32(10)},
							},
						},
					},
				},
			},
			ErrContains: "unsupported data type",
		},
		{
			Name: "Invalid array mismatched types",
			Input: []any{
				"hello world", "this", 10, "is wrong",
			},
			ErrContains: "mismatched array types",
		},
	} {
		t.Run(test.Name, func(t *testing.T) {
			res, err := InferFromAny(test.Input)
			if test.ErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.ErrContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, res, test.Output)

				// Also test serialization and deserialization
				rtSchema, err := ParseFromAny(res.ToAny())
				require.NoError(t, err, "Ability to serialize the schema")

				assert.Equal(t, rtSchema, test.Output)
			}
		})
	}
}
