// Copyright 2026 Redpanda Data, Inc.

package schema

import (
	"fmt"
	"math/big"
)

// NewBigDecimal constructs a Common schema for a [BigDecimal] column —
// an arbitrary-precision decimal with no schema-level precision or scale
// commitment. Use this for sources where the column type does not carry
// fixed precision and scale (e.g. unparameterised Postgres NUMERIC, Oracle
// NUMBER without DATA_PRECISION, MongoDB Decimal128).
func NewBigDecimal(name string, optional bool) Common {
	return Common{
		Name:     name,
		Type:     BigDecimal,
		Optional: optional,
	}
}

// FormatBigDecimal renders an unscaled integer at the given scale as a
// canonical [BigDecimal] string. Output rules match [FormatDecimal]: leading
// minus for negatives only, no leading plus, no leading zeros aside from a
// single "0" before the decimal point, decimal point present iff scale > 0,
// exactly scale fractional digits emitted.
//
// Unlike [DecimalParams.Format], the [BigDecimal] schema imposes no fixed
// scale; callers pick the scale that matches the source value's natural
// precision. The scale parameter must be non-negative.
func FormatBigDecimal(unscaled *big.Int, scale int32) (string, error) {
	return FormatDecimal(unscaled, scale)
}

// ParseBigDecimal interprets s as a decimal-shaped string and returns the
// unscaled integer alongside the scale recovered from the number of
// fractional digits in the input.
//
// The accepted form matches [ParseDecimal]: lenient on non-canonical-but-
// unambiguous inputs (leading plus, leading zeros, missing integer part as
// in ".5"), strict on ambiguous or malformed inputs (scientific notation,
// multiple decimal points, whitespace, thousands separators, non-digit
// characters). Canonical form is enforced when values are re-emitted via
// [FormatBigDecimal]. Unlike [ParseDecimal], the scale is recovered from
// the input rather than supplied by the caller.
//
// The parser does not bound the input length. The underlying big.Int parse
// is super-linear, so callers exposing this directly to untrusted input
// should impose their own length cap.
func ParseBigDecimal(s string) (*big.Int, int32, error) {
	sign, intPart, fracPart, err := parseCanonicalDecimal(s)
	if err != nil {
		return nil, 0, err
	}

	raw := sign + intPart + fracPart
	n, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		return nil, 0, fmt.Errorf("failed to parse decimal value %q", s)
	}

	return n, int32(len(fracPart)), nil
}
