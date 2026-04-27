// Copyright 2026 Redpanda Data, Inc.

package bloblangv2_test

import (
	"fmt"
	"testing"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// TestPluginFunctionLambdaParam exercises the function-side of the plugin
// lambda support, which has no V1 stdlib equivalent and is otherwise
// untested. The function takes a count and a producer lambda; the lambda
// is invoked count times with its index and the results are collected.
func TestPluginFunctionLambdaParam(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	if err := env.RegisterFunction("repeat_call",
		bloblangv2.NewPluginSpec().
			Description("Calls fn(i) for i in 0..count-1 and returns the array of results.").
			Param(bloblangv2.NewInt64Param("count").Description("Number of invocations.")).
			Param(bloblangv2.NewLambdaParam("fn").Description("Producer lambda receiving the index.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
			n, err := args.GetInt64("count")
			if err != nil {
				return nil, err
			}
			fn, err := args.GetLambda("fn")
			if err != nil {
				return nil, err
			}
			return func() (any, error) {
				out := make([]any, 0, n)
				for i := int64(0); i < n; i++ {
					v, err := fn(i)
					if err != nil {
						return nil, fmt.Errorf("index %d: %w", i, err)
					}
					out = append(out, v)
				}
				return out, nil
			}, nil
		},
	); err != nil {
		t.Fatal(err)
	}

	exec, err := env.Parse(`output = repeat_call(3, i -> "v" + i.string())`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := exec.Query(nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	want := []any{"v0", "v1", "v2"}
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPluginMethodLambdaWithMapRef(t *testing.T) {
	// Bare map references are valid in lambda positions per spec §5.5.
	// A mapping that references a user-defined map by name should
	// synthesise a single-param lambda automatically when invoking a
	// plugin method that takes a Lambda.
	env := bloblangv2.NewEnvironment()
	if err := env.RegisterMethod("count_matches",
		bloblangv2.NewPluginSpec().
			Description("Counts elements for which the predicate is true.").
			Param(bloblangv2.NewLambdaParam("pred")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			pred, err := args.GetLambda("pred")
			if err != nil {
				return nil, err
			}
			return bloblangv2.ArrayMethod(func(arr []any) (any, error) {
				n := int64(0)
				for _, e := range arr {
					out, err := pred(e)
					if err != nil {
						return nil, err
					}
					if b, ok := out.(bool); ok && b {
						n++
					}
				}
				return n, nil
			}), nil
		},
	); err != nil {
		t.Fatal(err)
	}

	src := `
map is_big(data) {
  data > 5
}
output = input.count_matches(is_big)
`
	exec, err := env.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := exec.Query([]any{int64(1), int64(7), int64(3), int64(8)})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if got != int64(2) {
		t.Fatalf("got %v, want 2", got)
	}
}
