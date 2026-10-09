// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"fmt"
	"math"
	"strconv"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// V2 ports of V1 numeric methods. All deterministic; live with the existing
// pure plugins.

func init() {
	bloblangv2.MustRegisterMethod("log",
		bloblangv2.NewPluginSpec().
			Category("Numbers").
			Description("Calculates the natural logarithm (base e) of the receiver number."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.Float64Method(func(f float64) (any, error) {
				return math.Log(f), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("log10",
		bloblangv2.NewPluginSpec().
			Category("Numbers").
			Description("Calculates the base-10 logarithm of the receiver number."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.Float64Method(func(f float64) (any, error) {
				return math.Log10(f), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("bitwise_and",
		bloblangv2.NewPluginSpec().
			Category("Numbers").
			Description("Performs a bitwise AND between the receiver integer and the value argument.").
			Param(bloblangv2.NewInt64Param("value").Description("The value to AND with.")),
		bitwiseV2Ctor(func(a, b int64) int64 { return a & b }),
	)

	bloblangv2.MustRegisterMethod("bitwise_or",
		bloblangv2.NewPluginSpec().
			Category("Numbers").
			Description("Performs a bitwise OR between the receiver integer and the value argument.").
			Param(bloblangv2.NewInt64Param("value").Description("The value to OR with.")),
		bitwiseV2Ctor(func(a, b int64) int64 { return a | b }),
	)

	bloblangv2.MustRegisterMethod("bitwise_xor",
		bloblangv2.NewPluginSpec().
			Category("Numbers").
			Description("Performs a bitwise XOR between the receiver integer and the value argument.").
			Param(bloblangv2.NewInt64Param("value").Description("The value to XOR with.")),
		bitwiseV2Ctor(func(a, b int64) int64 { return a ^ b }),
	)

	bloblangv2.MustRegisterMethod("number",
		bloblangv2.NewPluginSpec().
			Category("Coercion").
			Description("Attempts to coerce the receiver into a float64. Accepts numbers, numeric strings, and bools. If coercion fails and a default is supplied the default is returned instead.").
			Param(bloblangv2.NewFloat64Param("default").Description("Value returned when coercion fails.").Optional()),
		numberCoerceV2Ctor,
	)

	bloblangv2.MustRegisterMethod("pow",
		bloblangv2.NewPluginSpec().
			Category("Numbers").
			Description("Returns the receiver raised to the specified exponent.").
			Param(bloblangv2.NewFloat64Param("exponent").Description("The exponent to raise the receiver to.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			exp, err := args.GetFloat64("exponent")
			if err != nil {
				return nil, err
			}
			return bloblangv2.Float64Method(func(base float64) (any, error) {
				return math.Pow(base, exp), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("sin",
		bloblangv2.NewPluginSpec().
			Category("Numbers").
			Description("Calculates the sine of the receiver, interpreted as an angle in radians."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.Float64Method(func(f float64) (any, error) {
				return math.Sin(f), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("cos",
		bloblangv2.NewPluginSpec().
			Category("Numbers").
			Description("Calculates the cosine of the receiver, interpreted as an angle in radians."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.Float64Method(func(f float64) (any, error) {
				return math.Cos(f), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("tan",
		bloblangv2.NewPluginSpec().
			Category("Numbers").
			Description("Calculates the tangent of the receiver, interpreted as an angle in radians."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.Float64Method(func(f float64) (any, error) {
				return math.Tan(f), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterFunction("pi",
		bloblangv2.NewPluginSpec().
			Category("Numbers").
			Description("Returns the value of the mathematical constant Pi."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
			return func() (any, error) {
				return math.Pi, nil
			}, nil
		},
	)
}

func bitwiseV2Ctor(op func(a, b int64) int64) bloblangv2.MethodConstructor {
	return func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
		rhs, err := args.GetInt64("value")
		if err != nil {
			return nil, err
		}
		return bloblangv2.Int64Method(func(lhs int64) (any, error) {
			return op(lhs, rhs), nil
		}), nil
	}
}

func numberCoerceV2Ctor(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
	defaultPtr, err := args.GetOptionalFloat64("default")
	if err != nil {
		return nil, err
	}
	return func(v any) (any, error) {
		f, ok := coerceToFloat(v)
		if ok {
			return f, nil
		}
		if defaultPtr != nil {
			return *defaultPtr, nil
		}
		return nil, fmt.Errorf("could not coerce %T to a number", v)
	}, nil
}

func coerceToFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case uint32:
		return float64(n), true
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err == nil {
			return f, true
		}
		return 0, false
	case []byte:
		f, err := strconv.ParseFloat(string(n), 64)
		if err == nil {
			return f, true
		}
		return 0, false
	}
	return 0, false
}
