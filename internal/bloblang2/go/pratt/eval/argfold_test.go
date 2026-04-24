package eval_test

import (
	"regexp"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/eval"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

// TestArgFolderSubstitutesValue is the end-to-end regression for the
// parse-time fold mechanism. Compiles a mapping with a literal regex
// pattern, inspects the resolved AST to confirm the translator stashed
// a *regexp.Regexp on the argument, then executes the mapping and
// checks the result — the runtime must have used the precompiled
// value rather than compiling again.
func TestArgFolderSubstitutesValue(t *testing.T) {
	methods, functions := eval.StdlibNames()
	methodOpcodes, functionOpcodes := eval.StdlibOpcodes()

	prog, errs := syntax.Parse(`output = input.re_match("[0-9]+")`, "", nil)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	syntax.Optimize(prog)
	if rerrs := syntax.Resolve(prog, syntax.ResolveOptions{
		Methods:         methods,
		Functions:       functions,
		MethodOpcodes:   methodOpcodes,
		FunctionOpcodes: functionOpcodes,
	}); len(rerrs) > 0 {
		t.Fatalf("resolve: %v", rerrs)
	}

	// Walk the AST: the output = <path>.re_match(...) assignment should
	// carry a *regexp.Regexp on the method seg's first argument.
	found := false
	var walk func(any)
	walk = func(n any) {
		if found {
			return
		}
		switch v := n.(type) {
		case *syntax.PathExpr:
			for _, seg := range v.Segments {
				if seg.Kind == syntax.PathSegMethod && seg.Name == "re_match" {
					if len(seg.Args) == 0 {
						continue
					}
					if _, ok := seg.Args[0].Folded.(*regexp.Regexp); ok {
						found = true
						return
					}
				}
			}
		case *syntax.Assignment:
			walk(v.Value)
		case *syntax.MethodCallExpr:
			walk(v.Receiver)
		}
	}
	for _, s := range prog.Stmts {
		walk(s)
	}
	if !found {
		t.Fatalf("expected re_match's literal pattern to be folded into *regexp.Regexp, but CallArg.Folded was nil")
	}

	interp := eval.NewWithStdlib(prog)
	out, _, _, err := interp.Run("abc123", nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if b, ok := out.(bool); !ok || !b {
		t.Fatalf("expected true, got %v (%T)", out, out)
	}
}

// TestArgFolderRejectsInvalidLiteralAtParseTime confirms that a folder
// returning an error surfaces as a resolver diagnostic anchored at the
// call site, not a runtime error on first call.
func TestArgFolderRejectsInvalidLiteralAtParseTime(t *testing.T) {
	methods, functions := eval.StdlibNames()
	methodOpcodes, functionOpcodes := eval.StdlibOpcodes()

	prog, errs := syntax.Parse(`output = input.re_match("[unclosed")`, "", nil)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	syntax.Optimize(prog)
	rerrs := syntax.Resolve(prog, syntax.ResolveOptions{
		Methods:         methods,
		Functions:       functions,
		MethodOpcodes:   methodOpcodes,
		FunctionOpcodes: functionOpcodes,
	})
	if len(rerrs) == 0 {
		t.Fatal("expected a resolver diagnostic for the invalid regex, got none")
	}
	// Should mention the method name so users can find the offending call.
	found := false
	for _, e := range rerrs {
		if containsAny(e.Msg, "re_match", "invalid regex") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("resolver diagnostic did not name the method/error: %v", rerrs)
	}
}

// TestArgFolderLeavesDynamicArgsAlone confirms a non-literal pattern
// (e.g. from a $variable) skips folding and the runtime compiles the
// pattern normally.
func TestArgFolderLeavesDynamicArgsAlone(t *testing.T) {
	methods, functions := eval.StdlibNames()
	methodOpcodes, functionOpcodes := eval.StdlibOpcodes()

	src := `$pat = "[0-9]+"
output = input.re_match($pat)`
	prog, errs := syntax.Parse(src, "", nil)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	syntax.Optimize(prog)
	if rerrs := syntax.Resolve(prog, syntax.ResolveOptions{
		Methods:         methods,
		Functions:       functions,
		MethodOpcodes:   methodOpcodes,
		FunctionOpcodes: functionOpcodes,
	}); len(rerrs) > 0 {
		t.Fatalf("resolve: %v", rerrs)
	}
	interp := eval.NewWithStdlib(prog)
	out, _, _, err := interp.Run("abc123", nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if b, ok := out.(bool); !ok || !b {
		t.Fatalf("expected true, got %v", out)
	}
}

// containsAny is a tiny helper; strings.Contains loop inlined to avoid
// importing the strings package just for the test.
func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		for i := 0; i+len(n) <= len(haystack); i++ {
			if haystack[i:i+len(n)] == n {
				return true
			}
		}
	}
	return false
}
