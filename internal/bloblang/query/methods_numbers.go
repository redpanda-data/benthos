// Copyright 2025 Redpanda Data, Inc.

package query

import (
	"errors"
	"fmt"
	"math"

	"github.com/redpanda-data/benthos/v4/internal/value"
)

var _ = registerSimpleMethod(
	NewMethodSpec("ceil", "Rounds a number up to the nearest integer. Returns an integer if the result fits in 64-bit, otherwise returns a float.").InCategory(
		MethodCategoryNumbers, "",
		NewExampleSpec("",
			`root.new_value = this.value.ceil()`,
			`{"value":5.3}`,
			`{"new_value":6}`,
			`{"value":-5.9}`,
			`{"new_value":-5}`,
		),
		NewExampleSpec("",
			`root.result = this.price.ceil()`,
			`{"price":19.99}`,
			`{"result":20}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return numberMethod(func(f *float64, i *int64, ui *uint64) (any, error) {
			if f != nil {
				ceiled := math.Ceil(*f)
				if i, err := value.IToInt(ceiled); err == nil {
					return i, nil
				}
				return ceiled, nil
			}
			if i != nil {
				return *i, nil
			}
			return *ui, nil
		}), nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"floor", "Rounds a number down to the nearest integer. Returns an integer if the result fits in 64-bit, otherwise returns a float.",
	).InCategory(
		MethodCategoryNumbers,
		"",
		NewExampleSpec("",
			`root.new_value = this.value.floor()`,
			`{"value":5.7}`,
			`{"new_value":5}`,
			`{"value":-3.2}`,
			`{"new_value":-4}`,
		),
		NewExampleSpec("",
			`root.whole_seconds = this.duration_seconds.floor()`,
			`{"duration_seconds":12.345}`,
			`{"whole_seconds":12}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return numberMethod(func(f *float64, i *int64, ui *uint64) (any, error) {
			if f != nil {
				floored := math.Floor(*f)
				if i, err := value.IToInt(floored); err == nil {
					return i, nil
				}
				return floored, nil
			}
			if i != nil {
				return *i, nil
			}
			return *ui, nil
		}), nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec("log", "Calculates the natural logarithm (base e) of a number.").InCategory(
		MethodCategoryNumbers, "",
		NewExampleSpec("",
			`root.new_value = this.value.log().round()`,
			`{"value":1}`,
			`{"new_value":0}`,
			`{"value":2.7183}`,
			`{"new_value":1}`,
		),
		NewExampleSpec("",
			`root.ln_result = this.number.log()`,
			`{"number":10}`,
			`{"ln_result":2.302585092994046}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return numberMethod(func(f *float64, i *int64, ui *uint64) (any, error) {
			var v float64
			if f != nil {
				v = *f
			} else if i != nil {
				v = float64(*i)
			} else {
				v = float64(*ui)
			}
			return math.Log(v), nil
		}), nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec("log10", "Calculates the base-10 logarithm of a number.").InCategory(
		MethodCategoryNumbers, "",
		NewExampleSpec("",
			`root.new_value = this.value.log10()`,
			`{"value":100}`,
			`{"new_value":2}`,
			`{"value":1000}`,
			`{"new_value":3}`,
		),
		NewExampleSpec("",
			`root.log_value = this.magnitude.log10()`,
			`{"magnitude":10000}`,
			`{"log_value":4}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return numberMethod(func(f *float64, i *int64, ui *uint64) (any, error) {
			var v float64
			if f != nil {
				v = *f
			} else if i != nil {
				v = float64(*i)
			} else {
				v = float64(*ui)
			}
			return math.Log10(v), nil
		}), nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"max",
		"Returns the largest number from an array. All elements must be numbers and the array cannot be empty.",
	).InCategory(
		MethodCategoryNumbers, "",
		NewExampleSpec("",
			`root.biggest = this.values.max()`,
			`{"values":[0,3,2.5,7,5]}`,
			`{"biggest":7}`,
		),
		NewExampleSpec("",
			`root.highest_temp = this.temperatures.max()`,
			`{"temperatures":[20.5,22.1,19.8,23.4]}`,
			`{"highest_temp":23.4}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return func(v any, ctx FunctionContext) (any, error) {
			arr, ok := v.([]any)
			if !ok {
				return nil, value.NewTypeError(v, value.TArray)
			}
			if len(arr) == 0 {
				return nil, errors.New("the array was empty")
			}
			var maxV float64
			for i, n := range arr {
				f, err := value.IGetNumber(n)
				if err != nil {
					return nil, fmt.Errorf("index %v of array: %w", i, err)
				}
				if i == 0 || f > maxV {
					maxV = f
				}
			}
			return maxV, nil
		}, nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"min",
		"Returns the smallest number from an array. All elements must be numbers and the array cannot be empty.",
	).InCategory(
		MethodCategoryNumbers, "",
		NewExampleSpec("",
			`root.smallest = this.values.min()`,
			`{"values":[0,3,-2.5,7,5]}`,
			`{"smallest":-2.5}`,
		),
		NewExampleSpec("",
			`root.lowest_temp = this.temperatures.min()`,
			`{"temperatures":[20.5,22.1,19.8,23.4]}`,
			`{"lowest_temp":19.8}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return func(v any, ctx FunctionContext) (any, error) {
			arr, ok := v.([]any)
			if !ok {
				return nil, value.NewTypeError(v, value.TArray)
			}
			if len(arr) == 0 {
				return nil, errors.New("the array was empty")
			}
			var maxV float64
			for i, n := range arr {
				f, err := value.IGetNumber(n)
				if err != nil {
					return nil, fmt.Errorf("index %v of array: %w", i, err)
				}
				if i == 0 || f < maxV {
					maxV = f
				}
			}
			return maxV, nil
		}, nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"round", "Rounds a number to the nearest integer. Values at .5 round away from zero. Returns an integer if the result fits in 64-bit, otherwise returns a float.",
	).InCategory(
		MethodCategoryNumbers,
		"",
		NewExampleSpec("",
			`root.new_value = this.value.round()`,
			`{"value":5.3}`,
			`{"new_value":5}`,
			`{"value":5.9}`,
			`{"new_value":6}`,
		),
		NewExampleSpec("",
			`root.rounded = this.score.round()`,
			`{"score":87.5}`,
			`{"rounded":88}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return numberMethod(func(f *float64, i *int64, ui *uint64) (any, error) {
			if f != nil {
				rounded := math.Round(*f)
				if i, err := value.IToInt(rounded); err == nil {
					return i, nil
				}
				return rounded, nil
			}
			if i != nil {
				return *i, nil
			}
			return *ui, nil
		}), nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"bitwise_and", "Performs a bitwise AND operation between the integer and the specified value.",
	).InCategory(
		MethodCategoryNumbers,
		"",
		NewExampleSpec("",
			`root.new_value = this.value.bitwise_and(6)`,
			`{"value":12}`,
			`{"new_value":4}`,
		),
		NewExampleSpec("",
			`root.masked = this.flags.bitwise_and(15)`,
			`{"flags":127}`,
			`{"masked":15}`,
		),
	).Param(ParamInt64("value", "The value to AND with")),
	func(args *ParsedParams) (simpleMethod, error) {
		rhs, err := args.FieldInt64("value")
		if err != nil {
			return nil, err
		}

		return func(v any, _ FunctionContext) (any, error) {
			lhs, err := value.IGetInt(v)
			if err != nil {
				return nil, value.NewTypeError(v, value.TInt)
			}

			return lhs & rhs, nil
		}, nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"bitwise_or", "Performs a bitwise OR operation between the integer and the specified value.",
	).InCategory(
		MethodCategoryNumbers,
		"",
		NewExampleSpec("",
			`root.new_value = this.value.bitwise_or(6)`,
			`{"value":12}`,
			`{"new_value":14}`,
		),
		NewExampleSpec("",
			`root.combined = this.flags.bitwise_or(8)`,
			`{"flags":4}`,
			`{"combined":12}`,
		),
	).Param(ParamInt64("value", "The value to OR with")),
	func(args *ParsedParams) (simpleMethod, error) {
		rhs, err := args.FieldInt64("value")
		if err != nil {
			return nil, err
		}

		return func(v any, _ FunctionContext) (any, error) {
			lhs, err := value.IGetInt(v)
			if err != nil {
				return nil, value.NewTypeError(v, value.TInt)
			}

			return lhs | rhs, nil
		}, nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"bitwise_xor", "Performs a bitwise XOR (exclusive OR) operation between the integer and the specified value.",
	).InCategory(
		MethodCategoryNumbers,
		"",
		NewExampleSpec("",
			`root.new_value = this.value.bitwise_xor(6)`,
			`{"value":12}`,
			`{"new_value":10}`,
		),
		NewExampleSpec("",
			`root.toggled = this.flags.bitwise_xor(5)`,
			`{"flags":3}`,
			`{"toggled":6}`,
		),
	).Param(ParamInt64("value", "The value to XOR with")),
	func(args *ParsedParams) (simpleMethod, error) {
		rhs, err := args.FieldInt64("value")
		if err != nil {
			return nil, err
		}

		return func(v any, _ FunctionContext) (any, error) {
			lhs, err := value.IGetInt(v)
			if err != nil {
				return nil, value.NewTypeError(v, value.TInt)
			}

			return lhs ^ rhs, nil
		}, nil
	},
)
