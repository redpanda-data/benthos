// Tests in this file use run() which calls Parse → New → Run, bypassing
// Optimize and Resolve. This means AST nodes have no opcode IDs or stack
// slot indices — all execution goes through the scope-chain and map-lookup
// fallback paths.
//
// The optimized paths (opcode dispatch, variable stack) are exercised by
// the spec conformance suite in bloblang2_test.go, which uses the full
// compilation pipeline: Parse → Optimize → Resolve → NewWithStdlib → Run.
package eval

import (
	"reflect"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

func run(t *testing.T, src string, input any) (any, map[string]any, bool) {
	t.Helper()
	prog, errs := syntax.Parse(src, "", nil)
	if len(errs) > 0 {
		t.Fatalf("parse errors:\n%s", syntax.FormatErrors(errs))
	}
	interp := New(prog)
	meta := map[string]any{}
	output, outputMeta, deleted, err := interp.Run(input, meta)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	return output, outputMeta, deleted
}

func runExpectError(t *testing.T, src string, input any, substr string) {
	t.Helper()
	prog, errs := syntax.Parse(src, "", nil)
	if len(errs) > 0 {
		t.Fatalf("parse errors:\n%s", syntax.FormatErrors(errs))
	}
	interp := New(prog)
	meta := map[string]any{}
	_, _, _, err := interp.Run(input, meta)
	if err == nil {
		t.Fatalf("expected runtime error containing %q, but execution succeeded", substr)
	}
	if !containsStr(err.Error(), substr) {
		t.Fatalf("error %q does not contain %q", err.Error(), substr)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------
// Basic assignments
// -----------------------------------------------------------------------

func TestInterp_SimpleAssignment(t *testing.T) {
	output, _, _ := run(t, `output.x = 42`, nil)
	m := output.(map[string]any)
	if m["x"] != int64(42) {
		t.Fatalf("expected 42, got %v (%T)", m["x"], m["x"])
	}
}

func TestInterp_MultipleAssignments(t *testing.T) {
	output, _, _ := run(t, "output.a = 1\noutput.b = 2", nil)
	m := output.(map[string]any)
	if m["a"] != int64(1) || m["b"] != int64(2) {
		t.Fatalf("expected {a:1, b:2}, got %v", m)
	}
}

func TestInterp_VarDeclarationAndUse(t *testing.T) {
	output, _, _ := run(t, "$x = 42\noutput.v = $x", nil)
	m := output.(map[string]any)
	if m["v"] != int64(42) {
		t.Fatalf("expected 42, got %v", m["v"])
	}
}

func TestInterp_NestedFieldAssignment(t *testing.T) {
	output, _, _ := run(t, `output.user.name = "Alice"`, nil)
	m := output.(map[string]any)
	user := m["user"].(map[string]any)
	if user["name"] != "Alice" {
		t.Fatalf("expected Alice, got %v", user["name"])
	}
}

// -----------------------------------------------------------------------
// Arithmetic
// -----------------------------------------------------------------------

func TestInterp_Addition(t *testing.T) {
	output, _, _ := run(t, `output = 5 + 3`, nil)
	if output != int64(8) {
		t.Fatalf("expected 8, got %v (%T)", output, output)
	}
}

func TestInterp_Division(t *testing.T) {
	output, _, _ := run(t, `output = 7 / 2`, nil)
	if output != 3.5 {
		t.Fatalf("expected 3.5, got %v (%T)", output, output)
	}
}

func TestInterp_DivisionByZero(t *testing.T) {
	runExpectError(t, `output = 7 / 0`, nil, "division by zero")
}

func TestInterp_IntegerOverflow(t *testing.T) {
	runExpectError(t, `output = 9223372036854775807 + 1`, nil, "overflow")
}

func TestInterp_Modulo(t *testing.T) {
	output, _, _ := run(t, `output = 7 % 2`, nil)
	if output != int64(1) {
		t.Fatalf("expected 1, got %v (%T)", output, output)
	}
}

func TestInterp_StringConcat(t *testing.T) {
	output, _, _ := run(t, `output = "hello" + " " + "world"`, nil)
	if output != "hello world" {
		t.Fatalf("expected 'hello world', got %v", output)
	}
}

func TestInterp_UnaryMinus(t *testing.T) {
	output, _, _ := run(t, `output = -5`, nil)
	if output != int64(-5) {
		t.Fatalf("expected -5, got %v", output)
	}
}

func TestInterp_UnaryNot(t *testing.T) {
	output, _, _ := run(t, `output = !true`, nil)
	if output != false {
		t.Fatalf("expected false, got %v", output)
	}
}

// -----------------------------------------------------------------------
// Equality and comparison
// -----------------------------------------------------------------------

func TestInterp_Equality(t *testing.T) {
	output, _, _ := run(t, `output = 5 == 5`, nil)
	if output != true {
		t.Fatalf("expected true, got %v", output)
	}
}

func TestInterp_CrossFamilyEquality(t *testing.T) {
	output, _, _ := run(t, `output = 5 == "5"`, nil)
	if output != false {
		t.Fatalf("expected false, got %v", output)
	}
}

func TestInterp_Comparison(t *testing.T) {
	output, _, _ := run(t, `output = 5 > 3`, nil)
	if output != true {
		t.Fatalf("expected true, got %v", output)
	}
}

// -----------------------------------------------------------------------
// Logical operators
// -----------------------------------------------------------------------

func TestInterp_LogicalAnd(t *testing.T) {
	output, _, _ := run(t, `output = true && false`, nil)
	if output != false {
		t.Fatalf("expected false, got %v", output)
	}
}

func TestInterp_LogicalOr(t *testing.T) {
	output, _, _ := run(t, `output = false || true`, nil)
	if output != true {
		t.Fatalf("expected true, got %v", output)
	}
}

func TestInterp_LogicalRequiresBool(t *testing.T) {
	runExpectError(t, `output = 5 && true`, nil, "boolean")
}

// -----------------------------------------------------------------------
// Input access
// -----------------------------------------------------------------------

func TestInterp_InputAccess(t *testing.T) {
	input := map[string]any{"name": "Alice"}
	output, _, _ := run(t, `output.v = input.name`, input)
	m := output.(map[string]any)
	if m["v"] != "Alice" {
		t.Fatalf("expected Alice, got %v", m["v"])
	}
}

func TestInterp_InputRoot(t *testing.T) {
	output, _, _ := run(t, `output = input`, map[string]any{"x": int64(1)})
	m := output.(map[string]any)
	if m["x"] != int64(1) {
		t.Fatalf("expected {x:1}, got %v", m)
	}
}

// -----------------------------------------------------------------------
// If expression
// -----------------------------------------------------------------------

func TestInterp_IfExprTrue(t *testing.T) {
	output, _, _ := run(t, `output = if true { 1 } else { 2 }`, nil)
	if output != int64(1) {
		t.Fatalf("expected 1, got %v", output)
	}
}

func TestInterp_IfExprFalse(t *testing.T) {
	output, _, _ := run(t, `output = if false { 1 } else { 2 }`, nil)
	if output != int64(2) {
		t.Fatalf("expected 2, got %v", output)
	}
}

func TestInterp_IfExprVoid(t *testing.T) {
	// if without else when false → void → assignment skipped.
	output, _, _ := run(t, "output.x = \"prior\"\noutput.x = if false { 1 }", nil)
	m := output.(map[string]any)
	if m["x"] != "prior" {
		t.Fatalf("expected 'prior', got %v", m["x"])
	}
}

// -----------------------------------------------------------------------
// Match expression
// -----------------------------------------------------------------------

func TestInterp_MatchEquality(t *testing.T) {
	output, _, _ := run(t, `output = match "cat" { "cat" => "meow", "dog" => "woof", _ => "?" }`, nil)
	if output != "meow" {
		t.Fatalf("expected meow, got %v", output)
	}
}

func TestInterp_MatchWildcard(t *testing.T) {
	output, _, _ := run(t, `output = match "bird" { "cat" => "meow", _ => "unknown" }`, nil)
	if output != "unknown" {
		t.Fatalf("expected unknown, got %v", output)
	}
}

func TestInterp_MatchVoid(t *testing.T) {
	// No matching case, no wildcard → void.
	output, _, _ := run(t, "output.x = \"prior\"\noutput.x = match \"bird\" { \"cat\" => \"meow\" }", nil)
	m := output.(map[string]any)
	if m["x"] != "prior" {
		t.Fatalf("expected 'prior', got %v", m["x"])
	}
}

// -----------------------------------------------------------------------
// Map declarations
// -----------------------------------------------------------------------

func TestInterp_MapCall(t *testing.T) {
	output, _, _ := run(t, "map double(x) {\n  x * 2\n}\noutput = double(21)", nil)
	if output != int64(42) {
		t.Fatalf("expected 42, got %v", output)
	}
}

func TestInterp_MapCallMultiParam(t *testing.T) {
	output, _, _ := run(t, "map add(a, b) {\n  a + b\n}\noutput = add(3, 7)", nil)
	if output != int64(10) {
		t.Fatalf("expected 10, got %v", output)
	}
}

func TestInterp_MapIsolation(t *testing.T) {
	// Map cannot access top-level variables.
	runExpectError(t, "$x = 10\nmap bad(a) {\n  a + $x\n}\noutput = bad(1)", nil, "undefined")
}

// -----------------------------------------------------------------------
// Deletion
// -----------------------------------------------------------------------

func TestInterp_OutputDeleted(t *testing.T) {
	prog, _ := syntax.Parse("output = deleted()", "", nil)
	interp := New(prog)
	interp.RegisterFunction("deleted", FunctionSpec{Fn: func(_ []any) any { return Deleted }})
	_, _, deleted, err := interp.Run(nil, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected deleted")
	}
}

func TestInterp_FieldDeleted(t *testing.T) {
	prog, _ := syntax.Parse("output.a = 1\noutput.b = 2\noutput.b = deleted()", "", nil)
	interp := New(prog)
	interp.RegisterFunction("deleted", FunctionSpec{Fn: func(_ []any) any { return Deleted }})
	output, _, _, err := interp.Run(nil, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	m := output.(map[string]any)
	if _, exists := m["b"]; exists {
		t.Fatal("expected b to be deleted")
	}
	if m["a"] != int64(1) {
		t.Fatal("expected a to be 1")
	}
}

// -----------------------------------------------------------------------
// Array and object literals
// -----------------------------------------------------------------------

func TestInterp_ArrayLiteral(t *testing.T) {
	output, _, _ := run(t, `output = [1, 2, 3]`, nil)
	arr := output.([]any)
	if len(arr) != 3 || arr[0] != int64(1) {
		t.Fatalf("expected [1,2,3], got %v", arr)
	}
}

func TestInterp_ObjectLiteral(t *testing.T) {
	output, _, _ := run(t, `output = {"a": 1, "b": 2}`, nil)
	m := output.(map[string]any)
	if m["a"] != int64(1) || m["b"] != int64(2) {
		t.Fatalf("expected {a:1,b:2}, got %v", m)
	}
}

func TestInterp_VoidInArrayIsError(t *testing.T) {
	runExpectError(t, `output = [1, if false { 2 }, 3]`, nil, "void in array")
}

func TestInterp_VoidInObjectIsError(t *testing.T) {
	runExpectError(t, `output = {"a": if false { 1 }}`, nil, "void in object")
}

// -----------------------------------------------------------------------
// Recursion
// -----------------------------------------------------------------------

func TestInterp_Recursion(t *testing.T) {
	src := "map factorial(n) {\n  if n <= 1 { 1 } else { n * factorial(n - 1) }\n}\noutput = factorial(5)"
	output, _, _ := run(t, src, nil)
	if output != int64(120) {
		t.Fatalf("expected 120, got %v", output)
	}
}

// -----------------------------------------------------------------------
// Numeric promotion edge cases
// -----------------------------------------------------------------------

func TestInterp_IntPlusFloatPromotion(t *testing.T) {
	output, _, _ := run(t, `output = 5 + 3.0`, nil)
	if output != 8.0 {
		t.Fatalf("expected 8.0, got %v (%T)", output, output)
	}
}

func TestInterp_IntPlusFloatExceedsPrecision(t *testing.T) {
	runExpectError(t, `output = 9007199254740993 + 1.0`, nil, "float64 exact range")
}

func TestInterp_Uint64ExceedsInt64Range(t *testing.T) {
	// This would require uint64 values in the system — skip for now
	// since literals are always int64. Tested when uint64 values
	// enter through .uint64() conversion (stdlib phase).
}

func TestInterp_VoidInVarDeclarationIsError(t *testing.T) {
	runExpectError(t, `$x = if false { 42 }`, nil, "void")
}

func TestInterp_VoidInVarReassignmentSkips(t *testing.T) {
	output, _, _ := run(t, "$x = 10\n$x = if false { 42 }\noutput = $x", nil)
	if output != int64(10) {
		t.Fatalf("expected 10, got %v", output)
	}
}

// -----------------------------------------------------------------------
// Copy-on-write
// -----------------------------------------------------------------------

func TestInterp_CopyOnWrite(t *testing.T) {
	input := map[string]any{"x": int64(1)}
	output, _, _ := run(t, "$v = input\n$v.x = 99\noutput.original = input.x\noutput.copy = $v.x", input)
	m := output.(map[string]any)
	if m["original"] != int64(1) {
		t.Fatalf("expected original 1, got %v", m["original"])
	}
	if m["copy"] != int64(99) {
		t.Fatalf("expected copy 99, got %v", m["copy"])
	}
}

// -----------------------------------------------------------------------
// Stack path vs scope path equivalence
// -----------------------------------------------------------------------

// TestScopeAndStackPathsAgree runs the same program through both the
// unresolved (scope-based) and resolved (stack-based) paths and verifies
// they produce identical output.
func TestScopeAndStackPathsAgree(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		input any
	}{
		{
			name:  "variables and lambdas",
			src:   "$x = 10\n$y = 20\noutput.sum = $x + $y\noutput.mapped = [1, 2, 3].map(n -> n * $x)",
			input: nil,
		},
		{
			name:  "map call with params",
			src:   "map add(a, b) {\n  a + b\n}\noutput.v = add(3, 7)",
			input: nil,
		},
		{
			name:  "nested field access",
			src:   "$data = input\noutput.name = $data.user.name",
			input: map[string]any{"user": map[string]any{"name": "Alice"}},
		},
		{
			name:  "if expression with vars",
			src:   "$x = 5\noutput.v = if $x > 3 { \"big\" } else { \"small\" }",
			input: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := syntax.Parse(tc.src, "", nil)
			if len(errs) > 0 {
				t.Fatalf("parse errors:\n%s", syntax.FormatErrors(errs))
			}

			// Scope path: no optimize, no resolve.
			// Use a deep copy of the program since Optimize mutates in place.
			scopeProg, _ := syntax.Parse(tc.src, "", nil)
			scopeInterp := New(scopeProg)
			scopeInterp.RegisterStdlib()
			scopeInterp.lambdaMethods = make(map[string]MethodSpec, 16)
			scopeInterp.RegisterLambdaMethods()
			scopeOut, scopeMeta, scopeDel, scopeErr := scopeInterp.Run(tc.input, map[string]any{})

			// Stack path: optimize + resolve with opcodes and slots.
			syntax.Optimize(prog)
			methods, functions := StdlibNames()
			methodOpc, funcOpc := StdlibOpcodes()
			syntax.Resolve(prog, syntax.ResolveOptions{
				Methods: methods, Functions: functions,
				MethodOpcodes: methodOpc, FunctionOpcodes: funcOpc,
			})
			stackInterp := NewWithStdlib(prog)
			stackOut, stackMeta, stackDel, stackErr := stackInterp.Run(tc.input, map[string]any{})

			// Compare.
			if (scopeErr == nil) != (stackErr == nil) {
				t.Fatalf("error mismatch: scope=%v stack=%v", scopeErr, stackErr)
			}
			if scopeDel != stackDel {
				t.Fatalf("deleted mismatch: scope=%v stack=%v", scopeDel, stackDel)
			}
			if !reflect.DeepEqual(scopeOut, stackOut) {
				t.Fatalf("output mismatch:\n  scope: %v\n  stack: %v", scopeOut, stackOut)
			}
			if !reflect.DeepEqual(scopeMeta, stackMeta) {
				t.Fatalf("metadata mismatch:\n  scope: %v\n  stack: %v", scopeMeta, stackMeta)
			}
		})
	}
}
