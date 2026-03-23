package spectest

import (
	"math"
	"testing"
	"time"
)

func TestNormalizeYAMLValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{"int to int64", int(42), int64(42)},
		{"int64 unchanged", int64(42), int64(42)},
		{"float64 unchanged", 3.14, 3.14},
		{"string unchanged", "hello", "hello"},
		{"bool unchanged", true, true},
		{"nil unchanged", nil, nil},
		{
			"map with int values",
			map[string]any{"a": int(1), "b": int(2)},
			map[string]any{"a": int64(1), "b": int64(2)},
		},
		{
			"slice with int values",
			[]any{int(1), int(2), int(3)},
			[]any{int64(1), int64(2), int64(3)},
		},
		{
			"nested map",
			map[string]any{"outer": map[string]any{"inner": int(5)}},
			map[string]any{"outer": map[string]any{"inner": int64(5)}},
		},
		{
			"map[any]any to map[string]any",
			map[any]any{"key": int(10)},
			map[string]any{"key": int64(10)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeYAMLValue(tt.input)
			ok, diff := DeepEqual(tt.expected, result)
			if !ok {
				t.Fatalf("NormalizeYAMLValue mismatch:\n%s", diff)
			}
		})
	}
}

func TestDecodeTypedValues(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
		wantErr  bool
	}{
		{
			"int32",
			map[string]any{"_type": "int32", "value": "42"},
			int32(42),
			false,
		},
		{
			"int64",
			map[string]any{"_type": "int64", "value": "-100"},
			int64(-100),
			false,
		},
		{
			"uint32",
			map[string]any{"_type": "uint32", "value": "255"},
			uint32(255),
			false,
		},
		{
			"uint64",
			map[string]any{"_type": "uint64", "value": "18446744073709551615"},
			uint64(18446744073709551615),
			false,
		},
		{
			"float32",
			map[string]any{"_type": "float32", "value": "3.14"},
			float32(3.14),
			false,
		},
		{
			"float64",
			map[string]any{"_type": "float64", "value": "3.14"},
			float64(3.14),
			false,
		},
		{
			"float64 NaN",
			map[string]any{"_type": "float64", "value": "NaN"},
			math.NaN(),
			false,
		},
		{
			"float64 Infinity",
			map[string]any{"_type": "float64", "value": "Infinity"},
			math.Inf(1),
			false,
		},
		{
			"float64 -Infinity",
			map[string]any{"_type": "float64", "value": "-Infinity"},
			math.Inf(-1),
			false,
		},
		{
			"float64 -0.0",
			map[string]any{"_type": "float64", "value": "-0.0"},
			math.Float64frombits(1 << 63),
			false,
		},
		{
			"bytes",
			map[string]any{"_type": "bytes", "value": "aGVsbG8="},
			[]byte("hello"),
			false,
		},
		{
			"timestamp",
			map[string]any{"_type": "timestamp", "value": "2024-03-01T12:00:00Z"},
			time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
			false,
		},
		{
			"regular map not decoded",
			map[string]any{"_type": "int32", "value": "42", "extra": "field"},
			map[string]any{"_type": "int32", "value": "42", "extra": "field"},
			false,
		},
		{
			"nested typed values in map",
			map[string]any{"a": map[string]any{"_type": "int32", "value": "5"}},
			map[string]any{"a": int32(5)},
			false,
		},
		{
			"nested typed values in slice",
			[]any{map[string]any{"_type": "uint64", "value": "100"}, "plain"},
			[]any{uint64(100), "plain"},
			false,
		},
		{
			"scalar passthrough",
			"hello",
			"hello",
			false,
		},
		{
			"nil passthrough",
			nil,
			nil,
			false,
		},
		{
			"unknown type errors",
			map[string]any{"_type": "unknown", "value": "x"},
			nil,
			true,
		},
		{
			"value not a string — treated as regular map",
			map[string]any{"_type": "int32", "value": 42},
			map[string]any{"_type": "int32", "value": 42},
			false,
		},
		{
			"type not a string — treated as regular map",
			map[string]any{"_type": 99, "value": "hello"},
			map[string]any{"_type": 99, "value": "hello"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeTypedValues(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Special case for NaN comparison since DeepEqual handles it.
			ok, diff := DeepEqual(tt.expected, result)
			if !ok {
				t.Fatalf("DecodeTypedValues mismatch:\n%s", diff)
			}
		})
	}
}

func TestDecodeValue(t *testing.T) {
	// Tests the combined NormalizeYAMLValue + DecodeTypedValues pipeline.
	input := map[string]any{
		"count":  int(42),
		"typed":  map[string]any{"_type": "float32", "value": "1.5"},
		"nested": []any{int(1), map[string]any{"_type": "int32", "value": "2"}},
	}

	result, err := DecodeValue(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]any{
		"count":  int64(42),
		"typed":  float32(1.5),
		"nested": []any{int64(1), int32(2)},
	}

	ok, diff := DeepEqual(expected, result)
	if !ok {
		t.Fatalf("DecodeValue mismatch:\n%s", diff)
	}
}
