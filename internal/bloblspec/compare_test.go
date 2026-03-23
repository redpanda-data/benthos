package bloblspec

import (
	"math"
	"testing"
	"time"
)

func TestDeepEqual(t *testing.T) {
	tests := []struct {
		name     string
		expected any
		actual   any
		want     bool
	}{
		// Nil.
		{"both nil", nil, nil, true},
		{"expected nil actual not", nil, "hello", false},
		{"actual nil expected not", "hello", nil, false},

		// Strings.
		{"equal strings", "hello", "hello", true},
		{"different strings", "hello", "world", false},

		// Booleans.
		{"equal bools", true, true, true},
		{"different bools", true, false, false},

		// int64.
		{"equal int64", int64(42), int64(42), true},
		{"different int64", int64(42), int64(43), false},

		// int32.
		{"equal int32", int32(10), int32(10), true},
		{"different int32", int32(10), int32(11), false},

		// Type mismatch between int types.
		{"int32 vs int64", int32(5), int64(5), false},
		{"int64 vs uint64", int64(5), uint64(5), false},

		// uint32.
		{"equal uint32", uint32(100), uint32(100), true},

		// uint64.
		{"equal uint64", uint64(999), uint64(999), true},

		// float64.
		{"equal float64", 3.14, 3.14, true},
		{"different float64", 3.14, 3.15, false},
		{"float64 NaN both", math.NaN(), math.NaN(), true},
		{"float64 +Inf", math.Inf(1), math.Inf(1), true},
		{"float64 -Inf", math.Inf(-1), math.Inf(-1), true},
		{"float64 +Inf vs -Inf", math.Inf(1), math.Inf(-1), false},
		{"float64 NaN vs number", math.NaN(), 1.0, false},
		{"float64 -0 vs +0", math.Float64frombits(1 << 63), 0.0, false},

		// float32.
		{"equal float32", float32(1.5), float32(1.5), true},
		{"different float32", float32(1.5), float32(2.5), false},
		{"float32 NaN both", float32(math.NaN()), float32(math.NaN()), true},

		// float32 vs float64 type mismatch.
		{"float32 vs float64", float32(1.0), float64(1.0), false},

		// Bytes.
		{"equal bytes", []byte("hello"), []byte("hello"), true},
		{"different bytes", []byte("hello"), []byte("world"), false},

		// Time.
		{
			"equal timestamps",
			time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
			time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
			true,
		},
		{
			"equal timestamps different timezone",
			time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
			time.Date(2024, 3, 1, 7, 0, 0, 0, time.FixedZone("EST", -5*3600)),
			true,
		},
		{
			"different timestamps",
			time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
			time.Date(2024, 3, 1, 13, 0, 0, 0, time.UTC),
			false,
		},

		// Maps.
		{
			"equal maps",
			map[string]any{"a": int64(1), "b": int64(2)},
			map[string]any{"b": int64(2), "a": int64(1)},
			true,
		},
		{
			"maps different values",
			map[string]any{"a": int64(1)},
			map[string]any{"a": int64(2)},
			false,
		},
		{
			"maps missing key",
			map[string]any{"a": int64(1), "b": int64(2)},
			map[string]any{"a": int64(1)},
			false,
		},
		{
			"maps extra key",
			map[string]any{"a": int64(1)},
			map[string]any{"a": int64(1), "b": int64(2)},
			false,
		},
		{
			"empty maps",
			map[string]any{},
			map[string]any{},
			true,
		},

		// Slices.
		{
			"equal slices",
			[]any{int64(1), "two", true},
			[]any{int64(1), "two", true},
			true,
		},
		{
			"slices different length",
			[]any{int64(1), int64(2)},
			[]any{int64(1)},
			false,
		},
		{
			"slices different element",
			[]any{int64(1), int64(2)},
			[]any{int64(1), int64(3)},
			false,
		},
		{
			"empty slices",
			[]any{},
			[]any{},
			true,
		},

		// Nested structures.
		{
			"nested equal",
			map[string]any{"items": []any{map[string]any{"id": int64(1)}}},
			map[string]any{"items": []any{map[string]any{"id": int64(1)}}},
			true,
		},
		{
			"nested different",
			map[string]any{"items": []any{map[string]any{"id": int64(1)}}},
			map[string]any{"items": []any{map[string]any{"id": int64(2)}}},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, diff := DeepEqual(tt.expected, tt.actual)
			if got != tt.want {
				if tt.want {
					t.Fatalf("expected equal, got diff: %s", diff)
				} else {
					t.Fatalf("expected not equal, but got equal")
				}
			}
		})
	}
}

func TestCheckOutputType(t *testing.T) {
	tests := []struct {
		name         string
		expectedType string
		actual       any
		want         bool
	}{
		{"string", "string", "hello", true},
		{"int64", "int64", int64(42), true},
		{"int32", "int32", int32(42), true},
		{"float64", "float64", 3.14, true},
		{"bool", "bool", true, true},
		{"null", "null", nil, true},
		{"bytes", "bytes", []byte{1, 2}, true},
		{"timestamp", "timestamp", time.Now(), true},
		{"array", "array", []any{1}, true},
		{"object", "object", map[string]any{}, true},
		{"wrong type", "string", int64(42), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := CheckOutputType(tt.expectedType, tt.actual)
			if got != tt.want {
				t.Fatalf("CheckOutputType(%q, %T): expected %v, got %v", tt.expectedType, tt.actual, tt.want, got)
			}
		})
	}
}
