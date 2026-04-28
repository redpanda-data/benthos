// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBigDecimal(t *testing.T) {
	c := NewBigDecimal("amount", true)
	assert.Equal(t, BigDecimal, c.Type)
	assert.Equal(t, "amount", c.Name)
	assert.True(t, c.Optional)
	assert.Nil(t, c.Logical)
	assert.NoError(t, c.Validate())
}

func TestBigDecimalToAnyOmitsParams(t *testing.T) {
	c := NewBigDecimal("x", false)
	m, ok := c.ToAny().(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "BIG_DECIMAL", m[anyFieldType])
	_, hasPrecision := m[anyFieldPrecision]
	_, hasScale := m[anyFieldScale]
	assert.False(t, hasPrecision)
	assert.False(t, hasScale)
}

func TestBigDecimalRoundTrip(t *testing.T) {
	original := NewBigDecimal("balance", true)
	parsed, err := ParseFromAny(original.ToAny())
	require.NoError(t, err)
	assert.Equal(t, original.Type, parsed.Type)
	assert.Equal(t, original.Name, parsed.Name)
	assert.Equal(t, original.Optional, parsed.Optional)
	assert.Nil(t, parsed.Logical)
	assert.Equal(t, original.fingerprint(), parsed.fingerprint())
}

func TestBigDecimalParseFromAnyRejectsParams(t *testing.T) {
	in := map[string]any{
		anyFieldType:      "BIG_DECIMAL",
		anyFieldName:      "x",
		anyFieldPrecision: int64(10),
		anyFieldScale:     int64(2),
	}
	_, err := ParseFromAny(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only valid for type DECIMAL")
}

func TestBigDecimalValidateRejectsLogicalDecimal(t *testing.T) {
	c := Common{
		Type:    BigDecimal,
		Name:    "x",
		Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 10, Scale: 2}},
	}
	err := c.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only valid for type DECIMAL")
}

func TestBigDecimalValidateRejectsChildren(t *testing.T) {
	c := Common{
		Type:     BigDecimal,
		Name:     "x",
		Children: []Common{{Type: String, Name: "weird"}},
	}
	err := c.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a leaf and must not have children")
}

func TestFormatBigDecimal(t *testing.T) {
	tests := []struct {
		name     string
		unscaled string
		scale    int32
		want     string
	}{
		{"zero scale zero", "0", 0, "0"},
		{"zero scale four", "0", 4, "0.0000"},
		{"twelve thousand scale four", "12345", 4, "1.2345"},
		{"negative one scale four", "-1", 4, "-0.0001"},
		{"large scale", "1", 30, "0." + strings.Repeat("0", 29) + "1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, ok := new(big.Int).SetString(tt.unscaled, 10)
			require.True(t, ok)
			got, err := FormatBigDecimal(n, tt.scale)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatBigDecimalNoNegativeZero(t *testing.T) {
	// big.Int has no concept of -0 — Sign() returns 0 for zero values, so
	// we never emit a leading minus on a zero magnitude. Verify both
	// constructions land on the same canonical zero string.
	zeroPos := big.NewInt(0)
	zeroNeg := new(big.Int).Neg(big.NewInt(0))

	got, err := FormatBigDecimal(zeroPos, 4)
	require.NoError(t, err)
	assert.Equal(t, "0.0000", got)

	got, err = FormatBigDecimal(zeroNeg, 4)
	require.NoError(t, err)
	assert.Equal(t, "0.0000", got)
}

func TestParseBigDecimal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		unscaled string
		scale    int32
	}{
		{"integer", "12345", "12345", 0},
		{"negative integer", "-12345", "-12345", 0},
		{"fractional", "1.5", "15", 1},
		{"three fractional", "1.500", "1500", 3},
		{"zero", "0", "0", 0},
		{"zero with scale", "0.0000", "0", 4},
		{"negative fractional", "-0.0001", "-1", 4},
		{"trailing dot", "1.", "1", 0},
		{"high scale", "0." + strings.Repeat("0", 29) + "1", "1", 30},
		// Lenient acceptance — non-canonical but unambiguous.
		{"leading plus", "+1.5", "15", 1},
		{"leading zero", "01.5", "15", 1},
		{"leading zeros multiple", "-001", "-1", 0},
		{"missing integer part", ".5", "5", 1},
		{"missing integer part with sign", "-.5", "-5", 1},
		{"plus and missing integer", "+.5", "5", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, scale, err := ParseBigDecimal(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.unscaled, n.String())
			assert.Equal(t, tt.scale, scale)
		})
	}
}

func TestParseBigDecimalErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"empty", "", "must not be empty"},
		{"just minus", "-", "no digits"},
		{"just plus", "+", "no digits"},
		{"just dot", ".", "no digits"},
		{"two dots", "1.2.3", "at most one decimal point"},
		{"non-digit", "1.2a", "non-digit"},
		{"scientific notation", "1e5", "non-digit"},
		{"whitespace", " 1.5", "non-digit"},
		{"thousands separator", "1,000", "non-digit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseBigDecimal(tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestBigDecimalFormatParseRoundTrip(t *testing.T) {
	// Parse a canonical string, format the recovered (unscaled, scale)
	// pair, and confirm we get the original string back.
	values := []string{
		"0",
		"0.0",
		"0.0000",
		"1",
		"-1",
		"1.5",
		"-1.5",
		"12345.6789",
		"-12345.6789",
		"0.000000000000000000000000000001",
	}

	for _, v := range values {
		t.Run(v, func(t *testing.T) {
			unscaled, scale, err := ParseBigDecimal(v)
			require.NoError(t, err)
			got, err := FormatBigDecimal(unscaled, scale)
			require.NoError(t, err)
			assert.Equal(t, v, got)
		})
	}
}

func TestParseBigDecimalNormalisesToCanonical(t *testing.T) {
	// Postel: lenient parse, strict emit. Non-canonical inputs that the
	// parser accepts must come back out in canonical form when re-emitted
	// via FormatBigDecimal.
	tests := []struct {
		input string
		want  string
	}{
		{"+1.5", "1.5"},
		{"01.5", "1.5"},
		{"-001.5", "-1.5"},
		{".5", "0.5"},
		{"+.5", "0.5"},
		{"-.5", "-0.5"},
		{"+0.0001", "0.0001"},
		{"01.500", "1.500"}, // trailing zeros preserved (scale 3)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			n, scale, err := ParseBigDecimal(tt.input)
			require.NoError(t, err)
			got, err := FormatBigDecimal(n, scale)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBigDecimalFingerprintDistinctFromDecimal(t *testing.T) {
	bd := NewBigDecimal("amount", false)
	d := Common{
		Type:    Decimal,
		Name:    "amount",
		Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 10, Scale: 2}},
	}
	assert.NotEqual(t, bd.fingerprint(), d.fingerprint())
}
