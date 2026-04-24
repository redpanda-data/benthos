// Copyright 2026 Redpanda Data, Inc.

package bloblangv2_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

func TestBasicParseAndQuery(t *testing.T) {
	env := bloblangv2.NewEnvironment()
	exec, err := env.Parse(`output = input.uppercase()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := exec.Query("hello")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if out != "HELLO" {
		t.Fatalf("expected HELLO, got %#v", out)
	}
}

func TestParseErrorSurfacesLineAndColumn(t *testing.T) {
	_, err := bloblangv2.Parse(`root = nope(`)
	if err == nil {
		t.Fatal("expected parse error")
	}
	var pErr *bloblangv2.ParseError
	if !errors.As(err, &pErr) {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pErr.Line < 1 || pErr.Column < 1 {
		t.Fatalf("expected positive line/column, got %d:%d", pErr.Line, pErr.Column)
	}
}

func TestRegisterZeroArgMethod(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	err := env.RegisterMethod("bang", bloblangv2.NewPluginSpec(),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.StringMethod(func(s string) (any, error) {
				return s + "!", nil
			}), nil
		})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exec, err := env.Parse(`output = input.bang()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := exec.Query("hi")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if out != "hi!" {
		t.Fatalf("expected hi!, got %#v", out)
	}
}

func TestRegisterMethodWithParams(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	spec := bloblangv2.NewPluginSpec().
		Description("Append n copies of a string").
		Param(bloblangv2.NewStringParam("suffix").Description("the text to append")).
		Param(bloblangv2.NewInt64Param("count").Default(int64(1)))

	err := env.RegisterMethod("append_n", spec, func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
		suffix, err := args.GetString("suffix")
		if err != nil {
			return nil, err
		}
		count, err := args.GetInt64("count")
		if err != nil {
			return nil, err
		}
		return bloblangv2.StringMethod(func(s string) (any, error) {
			return s + strings.Repeat(suffix, int(count)), nil
		}), nil
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exec, err := env.Parse(`output = input.append_n("!", 3)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := exec.Query("hi")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if out != "hi!!!" {
		t.Fatalf("expected hi!!!, got %#v", out)
	}
}

func TestRegisterMethodDefaultApplied(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	spec := bloblangv2.NewPluginSpec().
		Param(bloblangv2.NewInt64Param("times").Default(int64(2)))

	err := env.RegisterMethod("repeat_value", spec, func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
		times, err := args.GetInt64("times")
		if err != nil {
			return nil, err
		}
		return bloblangv2.StringMethod(func(s string) (any, error) {
			return strings.Repeat(s, int(times)), nil
		}), nil
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exec, err := env.Parse(`output = input.repeat_value()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := exec.Query("ab")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if out != "abab" {
		t.Fatalf("expected abab, got %#v", out)
	}
}

func TestRegisterFunction(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	spec := bloblangv2.NewPluginSpec().
		Param(bloblangv2.NewStringParam("greeting"))

	err := env.RegisterFunction("greet", spec, func(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
		g, err := args.GetString("greeting")
		if err != nil {
			return nil, err
		}
		return func() (any, error) { return g + ", world!", nil }, nil
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exec, err := env.Parse(`output = greet("hello")`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := exec.Query(nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if out != "hello, world!" {
		t.Fatalf("expected 'hello, world!', got %#v", out)
	}
}

func TestRegisterRejectsStdlibShadow(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	err := env.RegisterMethod("uppercase", bloblangv2.NewPluginSpec(),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return func(v any) (any, error) { return v, nil }, nil
		})
	if err == nil {
		t.Fatal("expected error shadowing stdlib method")
	}
}

func TestRegisterRejectsInvalidName(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	err := env.RegisterMethod("NotSnakeCase", bloblangv2.NewPluginSpec(),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return func(v any) (any, error) { return v, nil }, nil
		})
	if err == nil {
		t.Fatal("expected error for non-snake-case name")
	}
}

func TestRegisterRejectsNilSpec(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	err := env.RegisterMethod("noop", nil,
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return func(v any) (any, error) { return v, nil }, nil
		})
	if err == nil {
		t.Fatal("expected error when spec is nil")
	}
}

func TestRegisterRejectsVariadicWithParams(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	spec := bloblangv2.NewPluginSpec().
		Variadic().
		Param(bloblangv2.NewStringParam("x"))

	err := env.RegisterMethod("bad", spec,
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return func(v any) (any, error) { return v, nil }, nil
		})
	if err == nil {
		t.Fatal("expected error for Variadic + Param combination")
	}
}

func TestPluginMethodArityEnforced(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	spec := bloblangv2.NewPluginSpec().Param(bloblangv2.NewStringParam("x"))
	if err := env.RegisterMethod("needs_one", spec,
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return func(v any) (any, error) { return v, nil }, nil
		}); err != nil {
		t.Fatal(err)
	}

	_, err := env.Parse(`output = input.needs_one()`)
	if err == nil {
		t.Fatal("expected arity error when required arg missing")
	}
}

func TestWithoutMethodsStripsStdlib(t *testing.T) {
	// V2's resolver defers method name validation to runtime, so stripping a
	// stdlib method via WithoutMethods surfaces as a Query-time "unknown
	// method" error rather than a parse error.
	env := bloblangv2.NewEnvironment().WithoutMethods("uppercase")
	exec, err := env.Parse(`output = input.uppercase()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = exec.Query("hi")
	if err == nil || !strings.Contains(err.Error(), "uppercase") {
		t.Fatalf("expected unknown method error, got %v", err)
	}
}

func TestWithoutFunctionsStripsStdlib(t *testing.T) {
	env := bloblangv2.NewEnvironment().WithoutFunctions("now")
	if _, err := env.Parse(`output = now()`); err == nil {
		t.Fatal("expected parse failure after WithoutFunctions")
	}
}

func TestExecutorConcurrentUse(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	if err := env.RegisterMethod("bang", bloblangv2.NewPluginSpec(),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.StringMethod(func(s string) (any, error) {
				return s + "!", nil
			}), nil
		}); err != nil {
		t.Fatal(err)
	}
	exec, err := env.Parse(`output = input.bang()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			in := fmt.Sprintf("val%d", idx)
			out, err := exec.Query(in)
			if err != nil {
				errCh <- err
				return
			}
			if out != in+"!" {
				errCh <- fmt.Errorf("expected %s!, got %#v", in, out)
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Error(e)
	}
}

func TestRegisterVariadicFunction(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	if err := env.RegisterFunction("shout", bloblangv2.NewPluginSpec().Variadic(),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
			raw := args.AsSlice()
			if len(raw) != 1 {
				return nil, fmt.Errorf("shout() requires one argument")
			}
			s, ok := raw[0].(string)
			if !ok {
				return nil, fmt.Errorf("shout() requires a string argument")
			}
			return func() (any, error) { return strings.ToUpper(s) + "!", nil }, nil
		}); err != nil {
		t.Fatal(err)
	}

	exec, err := env.Parse(`output = shout("hey")`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := exec.Query(nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if out != "HEY!" {
		t.Fatalf("expected HEY!, got %#v", out)
	}
}

// TestCtorCachedForStaticArgs verifies the static-args optimisation: when all
// arguments at a call site are literals, the plugin constructor should be
// invoked only once regardless of how many times Query runs.
func TestCtorCachedForStaticArgs(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	var ctorCount atomic.Int64

	spec := bloblangv2.NewPluginSpec().Param(bloblangv2.NewStringParam("suffix"))
	if err := env.RegisterMethod("append_suffix", spec,
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			ctorCount.Add(1)
			suf, err := args.GetString("suffix")
			if err != nil {
				return nil, err
			}
			return bloblangv2.StringMethod(func(s string) (any, error) {
				return s + suf, nil
			}), nil
		}); err != nil {
		t.Fatal(err)
	}

	exec, err := env.Parse(`output = input.append_suffix("!")`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := exec.Query("x"); err != nil {
			t.Fatalf("query %d: %v", i, err)
		}
	}
	if got := ctorCount.Load(); got != 1 {
		t.Fatalf("expected constructor to run once; ran %d times", got)
	}
}

func TestCtorCachedForZeroArgs(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	var ctorCount atomic.Int64

	if err := env.RegisterMethod("stamp", bloblangv2.NewPluginSpec(),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			ctorCount.Add(1)
			return bloblangv2.StringMethod(func(s string) (any, error) {
				return s + "[stamp]", nil
			}), nil
		}); err != nil {
		t.Fatal(err)
	}

	exec, err := env.Parse(`output = input.stamp()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := exec.Query("x"); err != nil {
			t.Fatalf("query %d: %v", i, err)
		}
	}
	if got := ctorCount.Load(); got != 1 {
		t.Fatalf("expected constructor to run once; ran %d times", got)
	}
}

func TestCtorRunsPerCallForDynamicArgs(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	var ctorCount atomic.Int64

	spec := bloblangv2.NewPluginSpec().Param(bloblangv2.NewStringParam("s"))
	if err := env.RegisterMethod("echo_s", spec,
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			ctorCount.Add(1)
			s, err := args.GetString("s")
			if err != nil {
				return nil, err
			}
			return func(_ any) (any, error) { return s, nil }, nil
		}); err != nil {
		t.Fatal(err)
	}

	exec, err := env.Parse(`output = input.echo_s(input)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := exec.Query(fmt.Sprintf("v%d", i)); err != nil {
			t.Fatalf("query %d: %v", i, err)
		}
	}
	if got := ctorCount.Load(); got != 5 {
		t.Fatalf("expected constructor to run per call (5); ran %d times", got)
	}
}

func TestCtorCachedForStaticFunctionArgs(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	var ctorCount atomic.Int64

	spec := bloblangv2.NewPluginSpec().Param(bloblangv2.NewStringParam("greeting"))
	if err := env.RegisterFunction("greet", spec,
		func(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
			ctorCount.Add(1)
			g, err := args.GetString("greeting")
			if err != nil {
				return nil, err
			}
			return func() (any, error) { return g + ", world!", nil }, nil
		}); err != nil {
		t.Fatal(err)
	}

	exec, err := env.Parse(`output = greet("hello")`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := exec.Query(nil); err != nil {
			t.Fatalf("query %d: %v", i, err)
		}
	}
	if got := ctorCount.Load(); got != 1 {
		t.Fatalf("expected constructor to run once; ran %d times", got)
	}
}
