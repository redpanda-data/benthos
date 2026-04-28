// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
)

// NewDecimal constructs a Common schema for a fixed-precision decimal column
// and validates the precision and scale bounds. It exists so callers can avoid
// the triple-nested LogicalParams/DecimalParams struct literal at the call
// site.
func NewDecimal(name string, precision, scale int32, optional bool) (Common, error) {
	c := Common{
		Name:     name,
		Type:     Decimal,
		Optional: optional,
		Logical: &LogicalParams{
			Decimal: &DecimalParams{
				Precision: precision,
				Scale:     scale,
			},
		},
	}
	if err := c.Validate(); err != nil {
		return Common{}, err
	}
	return c, nil
}

// FormatDecimal renders an unscaled integer as the canonical decimal string
// described by the package's value contract: a leading minus sign for
// negatives, no leading plus, no leading zeros aside from a single "0" before
// the decimal point, exactly scale fractional digits, and no scientific
// notation. The scale parameter must be non-negative.
//
// Precision is not enforced here; use [DecimalParams.Format] when both
// precision and scale must be checked.
//
// Examples for scale=4:
//
//	FormatDecimal(big.NewInt(12345), 4) // "1.2345"
//	FormatDecimal(big.NewInt(0), 4)     // "0.0000"
//	FormatDecimal(big.NewInt(-1), 4)    // "-0.0001"
//	FormatDecimal(big.NewInt(12345), 0) // "12345"
func FormatDecimal(unscaled *big.Int, scale int32) (string, error) {
	if unscaled == nil {
		return "", errors.New("unscaled value must not be nil")
	}
	if scale < 0 {
		return "", fmt.Errorf("scale must be non-negative, got %d", scale)
	}

	sign := ""
	if unscaled.Sign() < 0 {
		sign = "-"
	}
	abs := new(big.Int).Abs(unscaled).String()

	if scale == 0 {
		return sign + abs, nil
	}

	// Pad so that there is at least one digit before the decimal point.
	if int32(len(abs)) <= scale {
		abs = strings.Repeat("0", int(scale)-len(abs)+1) + abs
	}

	splitAt := int32(len(abs)) - scale
	return sign + abs[:splitAt] + "." + abs[splitAt:], nil
}

// ParseDecimal interprets s as a decimal-shaped string and returns the
// unscaled integer at the given scale. Inputs with fewer fractional digits
// than scale are right-padded with zeros; inputs with more fractional digits
// than scale are rejected.
//
// The parser is lenient: leading plus signs, leading zeros, and inputs
// missing the integer part (e.g. ".5") are accepted and normalised. The
// parser is strict about ambiguous or malformed inputs — scientific
// notation, multiple decimal points, whitespace, thousands separators, and
// non-digit characters are rejected. Canonical form is enforced when values
// are re-emitted via [FormatDecimal]. The scale parameter must be
// non-negative.
//
// Precision is not enforced here; use [DecimalParams.Parse] when both
// precision and scale must be checked.
//
// The parser does not bound the input length. The underlying big.Int parse
// is super-linear, so callers exposing this directly to untrusted input
// should impose their own length cap.
func ParseDecimal(s string, scale int32) (*big.Int, error) {
	if scale < 0 {
		return nil, fmt.Errorf("scale must be non-negative, got %d", scale)
	}

	sign, intPart, fracPart, err := parseCanonicalDecimal(s)
	if err != nil {
		return nil, err
	}

	if int32(len(fracPart)) > scale {
		return nil, fmt.Errorf("decimal string has %d fractional digits, exceeds scale %d", len(fracPart), scale)
	}

	padded := fracPart + strings.Repeat("0", int(scale)-len(fracPart))
	raw := sign + intPart + padded

	n, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		return nil, fmt.Errorf("failed to parse decimal value %q", s)
	}
	return n, nil
}

// parseCanonicalDecimal parses a decimal-shaped string into its components
// without applying any scale-specific transformation. The returned sign is ""
// or "-", and intPart and fracPart are digit-only strings (fracPart may be
// empty).
//
// Parsing follows Postel's principle: the function is lenient about
// non-canonical-but-unambiguous inputs (leading "+" sign, leading zeros,
// missing integer part as in ".5") and strict about ambiguous or malformed
// inputs (scientific notation, multiple decimal points, whitespace,
// thousands separators, non-digit characters). The canonical-form guarantee
// is upheld at the emit boundary by [FormatDecimal] and [FormatBigDecimal];
// strict parsing would duplicate that responsibility at a less useful layer.
func parseCanonicalDecimal(s string) (sign, intPart, fracPart string, err error) {
	if s == "" {
		return "", "", "", errors.New("decimal string must not be empty")
	}

	rest := s
	switch rest[0] {
	case '-':
		sign = "-"
		rest = rest[1:]
	case '+':
		// Leading plus is unambiguous and accepted; the canonical form just
		// omits it.
		rest = rest[1:]
	}
	if rest == "" {
		return "", "", "", errors.New("decimal string has no digits")
	}

	var hasDot bool
	intPart, fracPart, hasDot = strings.Cut(rest, ".")
	if hasDot && strings.Contains(fracPart, ".") {
		return "", "", "", errors.New("decimal string must contain at most one decimal point")
	}
	if intPart == "" {
		if !hasDot || fracPart == "" {
			return "", "", "", errors.New("decimal string has no digits")
		}
		// Inputs like ".5" — accept and treat as "0.5". The canonical emit
		// path will produce the leading zero on the way out.
		intPart = "0"
	}

	if err := requireDigits(intPart); err != nil {
		return "", "", "", err
	}
	if err := requireDigits(fracPart); err != nil {
		return "", "", "", err
	}

	// Cap the fractional length so callers downstream can safely cast it to
	// the int32 scale type without silent wrap-around. The integer part is
	// not bounded here — its length only feeds big.Int.SetString, which
	// handles arbitrary lengths correctly (if slowly).
	if len(fracPart) > math.MaxInt32 {
		return "", "", "", fmt.Errorf("decimal string has %d fractional digits, exceeds maximum %d", len(fracPart), math.MaxInt32)
	}

	return sign, intPart, fracPart, nil
}

func requireDigits(s string) error {
	for _, r := range s {
		if r < '0' || r > '9' {
			return fmt.Errorf("decimal string contains non-digit %q", r)
		}
	}
	return nil
}

// Format renders the unscaled integer as a canonical decimal string at the
// configured scale, and rejects values whose magnitude exceeds the configured
// precision.
func (p DecimalParams) Format(unscaled *big.Int) (string, error) {
	if err := p.ValidateValue(unscaled); err != nil {
		return "", err
	}
	return FormatDecimal(unscaled, p.Scale)
}

// Parse interprets s as a canonical decimal string at the configured scale,
// and rejects values whose magnitude exceeds the configured precision.
func (p DecimalParams) Parse(s string) (*big.Int, error) {
	n, err := ParseDecimal(s, p.Scale)
	if err != nil {
		return nil, err
	}
	if err := p.ValidateValue(n); err != nil {
		return nil, err
	}
	return n, nil
}

// ValidateValue confirms that the magnitude of v has no more significant
// digits than the configured precision. The configured precision and scale
// are not themselves validated by this method; use [Common.Validate] for
// that.
func (p DecimalParams) ValidateValue(v *big.Int) error {
	if v == nil {
		return errors.New("decimal value must not be nil")
	}
	digits := decimalDigits(v)
	if int32(digits) > p.Precision {
		return fmt.Errorf("decimal value has %d significant digits, exceeds precision %d", digits, p.Precision)
	}
	return nil
}

// decimalDigits returns the number of digits in the decimal representation of
// the absolute value of v. The digit count of zero is one.
func decimalDigits(v *big.Int) int {
	return len(new(big.Int).Abs(v).String())
}
