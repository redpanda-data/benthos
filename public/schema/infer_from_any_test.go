// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"encoding/json"
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
		{
			Name:  "json.Number integer",
			Input: json.Number("12345"),
			Output: Common{
				Type: Int64,
			},
		},
		{
			Name:  "json.Number float",
			Input: json.Number("1.5"),
			Output: Common{
				Type: Float64,
			},
		},
		{
			Name:        "json.Number invalid",
			Input:       json.Number("not-a-number"),
			ErrContains: "not parseable as int64 or float64",
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

// TestParameterisedRoundTrip exercises ToAny → ParseFromAny round-trips for
// the new parameterised logical types. These types are not reachable through
// [InferFromAny] (no Go runtime type maps to them), so they're exercised
// here directly.
func TestParameterisedRoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		schema Common
	}{
		{
			name:   "Date bare",
			schema: Common{Type: Date, Name: "d"},
		},
		{
			name:   "UUID bare",
			schema: Common{Type: UUID, Name: "u"},
		},
		{
			name: "Timestamp millis UTC",
			schema: Common{
				Type:    Timestamp,
				Name:    "ts",
				Logical: &LogicalParams{Timestamp: &TimestampParams{Unit: TimeUnitMillis, AdjustToUTC: true}},
			},
		},
		{
			name: "Timestamp micros local",
			schema: Common{
				Type:    Timestamp,
				Name:    "ts",
				Logical: &LogicalParams{Timestamp: &TimestampParams{Unit: TimeUnitMicros, AdjustToUTC: false}},
			},
		},
		{
			name: "Timestamp nanos UTC",
			schema: Common{
				Type:    Timestamp,
				Name:    "ts",
				Logical: &LogicalParams{Timestamp: &TimestampParams{Unit: TimeUnitNanos, AdjustToUTC: true}},
			},
		},
		{
			name:   "Timestamp legacy nil Logical",
			schema: Common{Type: Timestamp, Name: "ts"},
		},
		{
			name: "TimeOfDay millis",
			schema: Common{
				Type:    TimeOfDay,
				Name:    "tod",
				Logical: &LogicalParams{TimeOfDay: &TimeOfDayParams{Unit: TimeUnitMillis}},
			},
		},
		{
			name: "TimeOfDay micros UTC",
			schema: Common{
				Type:    TimeOfDay,
				Name:    "tod",
				Logical: &LogicalParams{TimeOfDay: &TimeOfDayParams{Unit: TimeUnitMicros, AdjustToUTC: true}},
			},
		},
		{
			name: "Object containing parameterised children",
			schema: Common{
				Type: Object,
				Name: "row",
				Children: []Common{
					{Type: Date, Name: "created_at"},
					{
						Type:    Timestamp,
						Name:    "updated_at",
						Logical: &LogicalParams{Timestamp: &TimestampParams{Unit: TimeUnitMicros, AdjustToUTC: true}},
					},
					{
						Type:    TimeOfDay,
						Name:    "open_at",
						Logical: &LogicalParams{TimeOfDay: &TimeOfDayParams{Unit: TimeUnitMillis}},
					},
					{Type: UUID, Name: "id"},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := ParseFromAny(tc.schema.ToAny())
			require.NoError(t, err)
			assert.Equal(t, tc.schema, rt)

			// Validate round-trips too
			assert.NoError(t, rt.Validate())
		})
	}
}

// TestParseFromAnyRejectsMisplacedParams asserts that timestamp/time-of-day
// params attached to the wrong top-level type are rejected on ParseFromAny.
func TestParseFromAnyRejectsMisplacedParams(t *testing.T) {
	cases := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{
			name: "unit on Int64",
			input: map[string]any{
				"type":          "INT64",
				"unit":          "MILLIS",
				"adjust_to_utc": true,
			},
			want: "only valid for types TIMESTAMP or TIME_OF_DAY",
		},
		{
			name: "TimeOfDay missing unit",
			input: map[string]any{
				"type": "TIME_OF_DAY",
			},
			want: "type TIME_OF_DAY requires fields",
		},
		{
			name: "Timestamp with adjust but no unit",
			input: map[string]any{
				"type":          "TIMESTAMP",
				"adjust_to_utc": true,
			},
			want: "requires field `unit`",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseFromAny(tc.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
