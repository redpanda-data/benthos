// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecimalToAnyEmitsParams(t *testing.T) {
	c := Common{
		Type:    Decimal,
		Name:    "amount",
		Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 18, Scale: 4}},
	}

	m, ok := c.ToAny().(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "DECIMAL", m[anyFieldType])
	assert.Equal(t, "amount", m[anyFieldName])
	assert.Equal(t, int64(18), m[anyFieldPrecision])
	assert.Equal(t, int64(4), m[anyFieldScale])
}

func TestDecimalNonDecimalDoesNotEmitParams(t *testing.T) {
	c := Common{Type: Int64, Name: "count"}
	m, ok := c.ToAny().(map[string]any)
	require.True(t, ok)

	_, hasPrecision := m[anyFieldPrecision]
	_, hasScale := m[anyFieldScale]
	assert.False(t, hasPrecision, "non-decimal types must not emit precision")
	assert.False(t, hasScale, "non-decimal types must not emit scale")
}

func TestDecimalRoundTrip(t *testing.T) {
	original := Common{
		Type:     Decimal,
		Name:     "balance",
		Optional: true,
		Logical:  &LogicalParams{Decimal: &DecimalParams{Precision: 38, Scale: 10}},
	}

	parsed, err := ParseFromAny(original.ToAny())
	require.NoError(t, err)

	assert.Equal(t, original.Type, parsed.Type)
	assert.Equal(t, original.Name, parsed.Name)
	assert.Equal(t, original.Optional, parsed.Optional)
	require.NotNil(t, parsed.Logical)
	require.NotNil(t, parsed.Logical.Decimal)
	assert.Equal(t, int32(38), parsed.Logical.Decimal.Precision)
	assert.Equal(t, int32(10), parsed.Logical.Decimal.Scale)
	assert.Equal(t, original.fingerprint(), parsed.fingerprint())
}

func TestDecimalRoundTripNested(t *testing.T) {
	original := Common{
		Type: Object,
		Name: "row",
		Children: []Common{
			{Type: String, Name: "id"},
			{
				Type:    Decimal,
				Name:    "price",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 12, Scale: 2}},
			},
			{
				Type:     Decimal,
				Name:     "fee",
				Optional: true,
				Logical:  &LogicalParams{Decimal: &DecimalParams{Precision: 8, Scale: 4}},
			},
		},
	}

	parsed, err := ParseFromAny(original.ToAny())
	require.NoError(t, err)
	assert.Equal(t, original.fingerprint(), parsed.fingerprint())
}

func TestDecimalParseFromAnyFloatPrecision(t *testing.T) {
	// JSON unmarshalling produces float64s for numbers; ensure we accept them
	// when they have no fractional part.
	in := map[string]any{
		anyFieldType:      "DECIMAL",
		anyFieldName:      "amount",
		anyFieldPrecision: float64(20),
		anyFieldScale:     float64(6),
	}

	c, err := ParseFromAny(in)
	require.NoError(t, err)
	require.NotNil(t, c.Logical)
	require.NotNil(t, c.Logical.Decimal)
	assert.Equal(t, int32(20), c.Logical.Decimal.Precision)
	assert.Equal(t, int32(6), c.Logical.Decimal.Scale)
}

func TestDecimalValidate(t *testing.T) {
	tests := []struct {
		name    string
		schema  Common
		wantErr string
	}{
		{
			name: "valid",
			schema: Common{
				Type:    Decimal,
				Name:    "x",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 10, Scale: 2}},
			},
		},
		{
			name: "valid scale equals precision",
			schema: Common{
				Type:    Decimal,
				Name:    "x",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 5, Scale: 5}},
			},
		},
		{
			name: "valid scale zero",
			schema: Common{
				Type:    Decimal,
				Name:    "x",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 38, Scale: 0}},
			},
		},
		{
			name:    "missing logical",
			schema:  Common{Type: Decimal, Name: "x"},
			wantErr: "requires Logical.Decimal parameters",
		},
		{
			name: "missing decimal params",
			schema: Common{
				Type:    Decimal,
				Name:    "x",
				Logical: &LogicalParams{},
			},
			wantErr: "requires Logical.Decimal parameters",
		},
		{
			name: "precision below minimum",
			schema: Common{
				Type:    Decimal,
				Name:    "x",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 0, Scale: 0}},
			},
			wantErr: "precision 0 out of range",
		},
		{
			name: "precision above maximum",
			schema: Common{
				Type:    Decimal,
				Name:    "x",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 39, Scale: 0}},
			},
			wantErr: "precision 39 out of range",
		},
		{
			name: "negative scale",
			schema: Common{
				Type:    Decimal,
				Name:    "x",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 10, Scale: -1}},
			},
			wantErr: "scale -1 out of range",
		},
		{
			name: "scale exceeds precision",
			schema: Common{
				Type:    Decimal,
				Name:    "x",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 5, Scale: 6}},
			},
			wantErr: "scale 6 out of range",
		},
		{
			name: "decimal params on non-decimal type",
			schema: Common{
				Type:    Int64,
				Name:    "x",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 5, Scale: 2}},
			},
			wantErr: "only valid for type DECIMAL",
		},
		{
			name: "child validation propagates",
			schema: Common{
				Type: Object,
				Name: "row",
				Children: []Common{
					{
						Type:    Decimal,
						Name:    "bad",
						Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 0, Scale: 0}},
					},
				},
			},
			wantErr: `child 0 ("bad")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.schema.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestDecimalParseFromAnyErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		wantErr string
	}{
		{
			name: "decimal missing precision and scale",
			input: map[string]any{
				anyFieldType: "DECIMAL",
				anyFieldName: "x",
			},
			wantErr: "requires fields `precision` and `scale`",
		},
		{
			name: "decimal missing scale",
			input: map[string]any{
				anyFieldType:      "DECIMAL",
				anyFieldName:      "x",
				anyFieldPrecision: int64(10),
			},
			wantErr: "requires field `scale`",
		},
		{
			name: "decimal missing precision",
			input: map[string]any{
				anyFieldType:  "DECIMAL",
				anyFieldName:  "x",
				anyFieldScale: int64(2),
			},
			wantErr: "requires field `precision`",
		},
		{
			name: "precision/scale on non-decimal",
			input: map[string]any{
				anyFieldType:      "INT64",
				anyFieldName:      "x",
				anyFieldPrecision: int64(10),
				anyFieldScale:     int64(2),
			},
			wantErr: "only valid for type DECIMAL",
		},
		{
			name: "non-integer precision",
			input: map[string]any{
				anyFieldType:      "DECIMAL",
				anyFieldName:      "x",
				anyFieldPrecision: 10.5,
				anyFieldScale:     int64(2),
			},
			wantErr: "must be an integer",
		},
		{
			name: "wrong type for precision",
			input: map[string]any{
				anyFieldType:      "DECIMAL",
				anyFieldName:      "x",
				anyFieldPrecision: "10",
				anyFieldScale:     int64(2),
			},
			wantErr: "expected field `precision` of integer type",
		},
		{
			name: "validation runs on parse",
			input: map[string]any{
				anyFieldType:      "DECIMAL",
				anyFieldName:      "x",
				anyFieldPrecision: int64(50),
				anyFieldScale:     int64(2),
			},
			wantErr: "out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFromAny(tt.input)
			require.Error(t, err)
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestNewDecimal(t *testing.T) {
	c, err := NewDecimal("amount", 18, 4, true)
	require.NoError(t, err)
	assert.Equal(t, Decimal, c.Type)
	assert.Equal(t, "amount", c.Name)
	assert.True(t, c.Optional)
	require.NotNil(t, c.Logical)
	require.NotNil(t, c.Logical.Decimal)
	assert.Equal(t, int32(18), c.Logical.Decimal.Precision)
	assert.Equal(t, int32(4), c.Logical.Decimal.Scale)
}

func TestNewDecimalRejectsInvalid(t *testing.T) {
	_, err := NewDecimal("x", 0, 0, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "precision 0 out of range")

	_, err = NewDecimal("x", 5, 6, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scale 6 out of range")

	_, err = NewDecimal("x", 5, -1, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scale -1 out of range")
}

func TestFormatDecimal(t *testing.T) {
	tests := []struct {
		name     string
		unscaled string
		scale    int32
		want     string
	}{
		{"zero scale zero", "0", 0, "0"},
		{"zero scale four", "0", 4, "0.0000"},
		{"one scale zero", "1", 0, "1"},
		{"one scale four", "1", 4, "0.0001"},
		{"negative one scale four", "-1", 4, "-0.0001"},
		{"twelve thousand scale zero", "12345", 0, "12345"},
		{"twelve thousand scale two", "12345", 2, "123.45"},
		{"twelve thousand scale four", "12345", 4, "1.2345"},
		{"twelve thousand scale five", "12345", 5, "0.12345"},
		{"twelve thousand scale six", "12345", 6, "0.012345"},
		{"negative scale two", "-12345", 2, "-123.45"},
		{"max precision", "12345678901234567890123456789012345678", 0, "12345678901234567890123456789012345678"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, ok := new(big.Int).SetString(tt.unscaled, 10)
			require.True(t, ok)
			got, err := FormatDecimal(n, tt.scale)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatDecimalErrors(t *testing.T) {
	_, err := FormatDecimal(nil, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be nil")

	_, err = FormatDecimal(big.NewInt(1), -1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scale must be non-negative")
}

func TestParseDecimal(t *testing.T) {
	tests := []struct {
		name  string
		input string
		scale int32
		want  string
	}{
		{"zero scale zero", "0", 0, "0"},
		{"zero scale four", "0.0000", 4, "0"},
		{"one scale zero", "1", 0, "1"},
		{"one scale four", "0.0001", 4, "1"},
		{"negative scale four", "-0.0001", 4, "-1"},
		{"twelve thousand scale four", "1.2345", 4, "12345"},
		{"twelve thousand scale two", "123.45", 2, "12345"},
		{"twelve thousand scale zero", "12345", 0, "12345"},
		{"pad fewer fractional digits", "1.5", 4, "15000"},
		{"integer to scale two", "12345", 2, "1234500"},
		{"trailing dot allowed", "1.", 0, "1"},
		{"trailing dot with scale", "1.", 3, "1000"},
		{"negative integer", "-123", 0, "-123"},
		{"max precision", "12345678901234567890123456789012345678", 0, "12345678901234567890123456789012345678"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := ParseDecimal(tt.input, tt.scale)
			require.NoError(t, err)
			assert.Equal(t, tt.want, n.String())
		})
	}
}

func TestParseDecimalErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		scale   int32
		wantErr string
	}{
		{"empty", "", 0, "must not be empty"},
		{"just minus", "-", 0, "no digits"},
		{"leading plus", "+1", 0, "must not have a leading plus"},
		{"missing integer part", ".5", 1, "missing the integer part"},
		{"two dots", "1.2.3", 1, "at most one decimal point"},
		{"non-digit", "1.2a", 1, "non-digit"},
		{"scientific notation", "1e5", 0, "non-digit"},
		{"whitespace", " 1.5", 4, "non-digit"},
		{"trailing whitespace", "1.5 ", 4, "non-digit"},
		{"thousands separator", "1,000", 0, "non-digit"},
		{"too many fractional digits", "1.23456", 4, "exceeds scale 4"},
		{"negative scale", "1.5", -1, "scale must be non-negative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDecimal(tt.input, tt.scale)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestDecimalRoundTripFormatParse(t *testing.T) {
	values := []struct {
		unscaled string
		scale    int32
	}{
		{"0", 0},
		{"0", 4},
		{"12345", 4},
		{"-12345", 4},
		{"1", 38},
		{"-1", 38},
		{"12345678901234567890123456789012345678", 0},
	}

	for _, v := range values {
		t.Run(v.unscaled+"@"+itoa(v.scale), func(t *testing.T) {
			n, ok := new(big.Int).SetString(v.unscaled, 10)
			require.True(t, ok)
			s, err := FormatDecimal(n, v.scale)
			require.NoError(t, err)
			parsed, err := ParseDecimal(s, v.scale)
			require.NoError(t, err)
			assert.Equal(t, 0, n.Cmp(parsed), "round trip mismatch: %s vs %s", n, parsed)
		})
	}
}

func itoa(v int32) string {
	return new(big.Int).SetInt64(int64(v)).String()
}

func TestDecimalParamsFormatRejectsOverflow(t *testing.T) {
	p := DecimalParams{Precision: 5, Scale: 2}

	// Within precision.
	s, err := p.Format(big.NewInt(99999))
	require.NoError(t, err)
	assert.Equal(t, "999.99", s)

	// Exceeds precision.
	_, err = p.Format(big.NewInt(123456))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds precision 5")
}

func TestDecimalParamsParseRejectsOverflow(t *testing.T) {
	p := DecimalParams{Precision: 5, Scale: 2}

	n, err := p.Parse("999.99")
	require.NoError(t, err)
	assert.Equal(t, "99999", n.String())

	_, err = p.Parse("9999.99")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds precision 5")
}

func TestDecimalParamsValidateValue(t *testing.T) {
	p := DecimalParams{Precision: 5, Scale: 2}

	assert.NoError(t, p.ValidateValue(big.NewInt(0)))
	assert.NoError(t, p.ValidateValue(big.NewInt(99999)))
	assert.NoError(t, p.ValidateValue(big.NewInt(-99999)))

	err := p.ValidateValue(big.NewInt(100000))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "6 significant digits")

	err = p.ValidateValue(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be nil")
}
