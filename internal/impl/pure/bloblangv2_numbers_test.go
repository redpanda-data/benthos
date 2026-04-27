// Copyright 2026 Redpanda Data, Inc.

package pure_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func TestBloblangV2Bitwise(t *testing.T) {
	cases := []struct {
		name    string
		mapping string
		input   any
		want    any
	}{
		{name: "and", mapping: `output = input.bitwise_and(6)`, input: int64(12), want: int64(4)},
		{name: "or", mapping: `output = input.bitwise_or(6)`, input: int64(12), want: int64(14)},
		{name: "xor", mapping: `output = input.bitwise_xor(6)`, input: int64(12), want: int64(10)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runBloblangV2(t, tc.mapping, tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBloblangV2Logarithms(t *testing.T) {
	got := runBloblangV2(t, `output = input.log()`, math.E)
	if got.(float64) < 0.99 || got.(float64) > 1.01 {
		t.Fatalf("log(e) = %v, want ~1.0", got)
	}

	got = runBloblangV2(t, `output = input.log10()`, int64(1000))
	if got.(float64) < 2.99 || got.(float64) > 3.01 {
		t.Fatalf("log10(1000) = %v, want ~3.0", got)
	}
}

func TestBloblangV2NumberCoercion(t *testing.T) {
	cases := []struct {
		name    string
		mapping string
		input   any
		want    float64
	}{
		{name: "string", mapping: `output = input.number()`, input: "3.14", want: 3.14},
		{name: "int", mapping: `output = input.number()`, input: int64(7), want: 7},
		{name: "bool true", mapping: `output = input.number()`, input: true, want: 1},
		{name: "bool false", mapping: `output = input.number()`, input: false, want: 0},
		{name: "default used", mapping: `output = input.number(42.0)`, input: "not a number", want: 42},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runBloblangV2(t, tc.mapping, tc.input)
			assert.InDelta(t, tc.want, got, 1e-9)
		})
	}
}

func TestBloblangV2NumberFailsWithoutDefault(t *testing.T) {
	exec, err := bloblangv2.GlobalEnvironment().Parse(`output = input.number()`)
	require.NoError(t, err)
	_, err = exec.Query("not a number")
	require.Error(t, err)
}
