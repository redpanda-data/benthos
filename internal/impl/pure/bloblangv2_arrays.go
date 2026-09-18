// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"errors"
	"fmt"

	"github.com/Jeffail/gabs/v2"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// V2 ports of V1 array / sequence methods that don't require lambda
// arguments (the plugin spec doesn't yet support those; tracked in PARITY).

func init() {
	bloblangv2.MustRegisterMethod("enumerated",
		bloblangv2.NewPluginSpec().
			Category("Object & Array").
			Description(`Alias for V2 enumerate(): transforms an array into [{"index": i, "value": v}, ...]. Retained for V1 parity.`),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.ArrayMethod(func(arr []any) (any, error) {
				out := make([]any, len(arr))
				for i, v := range arr {
					out[i] = map[string]any{"index": int64(i), "value": v}
				}
				return out, nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("find_all",
		bloblangv2.NewPluginSpec().
			Category("Object & Array").
			Description("Returns the indexes of every element in the receiver array equal to the value argument. Empty array if none match.").
			Param(bloblangv2.NewAnyParam("value").Description("The value to search for.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			target, err := args.Get("value")
			if err != nil {
				return nil, err
			}
			return bloblangv2.ArrayMethod(func(arr []any) (any, error) {
				out := []any{}
				for i, elem := range arr {
					if valuesLooselyEqual(elem, target) {
						out = append(out, int64(i))
					}
				}
				return out, nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("index",
		bloblangv2.NewPluginSpec().
			Category("Object & Array").
			Description("Returns the element at the given index. Negative indexes count from the end (-1 is the last element). Errors if the index is out of bounds.").
			Param(bloblangv2.NewInt64Param("index").Description("The index to extract.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			idx, err := args.GetInt64("index")
			if err != nil {
				return nil, err
			}
			return func(v any) (any, error) {
				switch arr := v.(type) {
				case []any:
					i := int(idx)
					if i < 0 {
						i = len(arr) + i
					}
					if i < 0 || i >= len(arr) {
						return nil, fmt.Errorf("index '%v' was out of bounds for array size: %v", idx, len(arr))
					}
					return arr[i], nil
				case []byte:
					i := int(idx)
					if i < 0 {
						i = len(arr) + i
					}
					if i < 0 || i >= len(arr) {
						return nil, fmt.Errorf("index '%v' was out of bounds for byte array size: %v", idx, len(arr))
					}
					return int64(arr[i]), nil
				}
				return nil, fmt.Errorf("expected array or bytes receiver, got %T", v)
			}, nil
		},
	)

	bloblangv2.MustRegisterMethod("not_empty",
		bloblangv2.NewPluginSpec().
			Category("Coercion").
			Description("Returns the receiver unchanged if it is a non-empty string, array, or object, otherwise errors. Useful for asserting required values."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return func(v any) (any, error) {
				switch t := v.(type) {
				case string:
					if t == "" {
						return nil, errors.New("string value is empty")
					}
				case []any:
					if len(t) == 0 {
						return nil, errors.New("array value is empty")
					}
				case map[string]any:
					if len(t) == 0 {
						return nil, errors.New("object value is empty")
					}
				default:
					return nil, fmt.Errorf("expected string, array, or object receiver, got %T", v)
				}
				return v, nil
			}, nil
		},
	)

	bloblangv2.MustRegisterMethod("collapse",
		bloblangv2.NewPluginSpec().
			Category("Object & Array").
			Description(`Flattens a nested object / array into a single-level object whose keys are dot-paths. Empty arrays and objects are excluded by default; pass include_empty: true to keep them.`).
			Param(bloblangv2.NewBoolParam("include_empty").Description("Include empty objects and arrays in the result.").Default(false)),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			includeEmpty, err := args.GetBool("include_empty")
			if err != nil {
				return nil, err
			}
			return func(v any) (any, error) {
				g := gabs.Wrap(v)
				if includeEmpty {
					return g.FlattenIncludeEmpty()
				}
				return g.Flatten()
			}, nil
		},
	)

	bloblangv2.MustRegisterMethod("key_values",
		bloblangv2.NewPluginSpec().
			Category("Object & Array").
			Description(`Converts an object into an array of {"key": ..., "value": ...} entries. Order is unspecified — sort with sort_by(p -> p.key) if needed.`),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.ObjectMethod(func(m map[string]any) (any, error) {
				out := make([]any, 0, len(m))
				for k, v := range m {
					out = append(out, map[string]any{"key": k, "value": v})
				}
				return out, nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("find_by",
		bloblangv2.NewPluginSpec().
			Category("Object & Array").
			Description("Returns the index of the first element of the receiver array for which the predicate returns true, or -1 if none match.").
			Param(bloblangv2.NewLambdaParam("query").Description("A predicate evaluated against each element. Must return a bool.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			pred, err := args.GetLambda("query")
			if err != nil {
				return nil, err
			}
			return bloblangv2.ArrayMethod(func(arr []any) (any, error) {
				for i, elem := range arr {
					out, err := pred(elem)
					if err != nil {
						return nil, fmt.Errorf("predicate failed for index %d: %w", i, err)
					}
					b, ok := out.(bool)
					if !ok {
						return nil, fmt.Errorf("predicate returned non-bool value for index %d: %T", i, out)
					}
					if b {
						return int64(i), nil
					}
				}
				return int64(-1), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("find_all_by",
		bloblangv2.NewPluginSpec().
			Category("Object & Array").
			Description("Returns the indexes of every element of the receiver array for which the predicate returns true. Empty array if none match.").
			Param(bloblangv2.NewLambdaParam("query").Description("A predicate evaluated against each element. Must return a bool.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			pred, err := args.GetLambda("query")
			if err != nil {
				return nil, err
			}
			return bloblangv2.ArrayMethod(func(arr []any) (any, error) {
				out := []any{}
				for i, elem := range arr {
					res, err := pred(elem)
					if err != nil {
						return nil, fmt.Errorf("predicate failed for index %d: %w", i, err)
					}
					b, ok := res.(bool)
					if !ok {
						return nil, fmt.Errorf("predicate returned non-bool value for index %d: %T", i, res)
					}
					if b {
						out = append(out, int64(i))
					}
				}
				return out, nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("map_each_key",
		bloblangv2.NewPluginSpec().
			Category("Object & Array").
			Description("Returns a new object with each key transformed by the lambda. The lambda receives the original key as a string and must return a new string key.").
			Param(bloblangv2.NewLambdaParam("query").Description("A lambda that returns the new key for each entry.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			fn, err := args.GetLambda("query")
			if err != nil {
				return nil, err
			}
			return bloblangv2.ObjectMethod(func(m map[string]any) (any, error) {
				out := make(map[string]any, len(m))
				for k, v := range m {
					newKey, err := fn(k)
					if err != nil {
						return nil, fmt.Errorf("key %q: %w", k, err)
					}
					ns, ok := newKey.(string)
					if !ok {
						return nil, fmt.Errorf("key %q: lambda must return a string, got %T", k, newKey)
					}
					out[ns] = v
				}
				return out, nil
			}), nil
		},
	)
}

// valuesLooselyEqual mirrors V1's value.ICompare for find_all: treats numerics
// across families (int / float) as equal when the represented values match.
func valuesLooselyEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	if af, aOk := looseAsFloat(a); aOk {
		if bf, bOk := looseAsFloat(b); bOk {
			return af == bf
		}
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b) && fmt.Sprintf("%T", a) == fmt.Sprintf("%T", b)
}

func looseAsFloat(v any) (float64, bool) {
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
	}
	return 0, false
}
