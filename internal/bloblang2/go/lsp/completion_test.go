package lsp

import (
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/eval"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

func TestFormatFunctionArity(t *testing.T) {
	tests := []struct {
		name string
		fn   string
		fi   syntax.FunctionInfo
		want string
	}{
		{"no args", "uuid_v4", syntax.FunctionInfo{Required: 0, Total: 0}, "uuid_v4()"},
		{"one required", "throw", syntax.FunctionInfo{Required: 1, Total: 1}, "throw(1 args)"},
		{"two required", "random_int", syntax.FunctionInfo{Required: 2, Total: 2}, "random_int(2 args)"},
		{"mixed", "foo", syntax.FunctionInfo{Required: 1, Total: 3}, "foo(1 to 3 args)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFunctionArity(tt.fn, tt.fi)
			if got != tt.want {
				t.Errorf("formatFunctionArity(%q, %+v) = %q, want %q", tt.fn, tt.fi, got, tt.want)
			}
		})
	}
}

func TestNewCompletionEngine(t *testing.T) {
	methods, functions := eval.StdlibNames()
	e := newCompletionEngine(methods, functions)

	// Keywords should include standard language keywords.
	keywordLabels := labelSet(e.keywords)
	for _, kw := range []string{"if", "else", "match", "map", "input", "output"} {
		if !keywordLabels[kw] {
			t.Errorf("missing keyword %q", kw)
		}
	}

	// Constants should have value kind.
	for _, item := range e.keywords {
		if item.Label == "true" || item.Label == "false" || item.Label == "null" {
			if item.Kind != completionKindValue {
				t.Errorf("keyword %q has kind %d, want %d (value)", item.Label, item.Kind, completionKindValue)
			}
		}
	}

	// Should have functions from stdlib.
	fnLabels := labelSet(e.functions)
	for _, fn := range []string{"uuid_v4", "now", "throw", "deleted", "random_int", "range", "timestamp"} {
		if !fnLabels[fn] {
			t.Errorf("missing function %q", fn)
		}
	}
	for _, item := range e.functions {
		if item.Kind != completionKindFunction {
			t.Errorf("function %q has kind %d, want %d", item.Label, item.Kind, completionKindFunction)
		}
		if !strings.HasSuffix(item.InsertText, "($0)") {
			t.Errorf("function %q insertText = %q, want suffix ($0)", item.Label, item.InsertText)
		}
	}

	// Should have methods from stdlib.
	methodLabels := labelSet(e.methods)
	for _, m := range []string{"uppercase", "lowercase", "length", "filter", "map", "fold", "keys", "values", "catch", "or"} {
		if !methodLabels[m] {
			t.Errorf("missing method %q", m)
		}
	}
	for _, item := range e.methods {
		if item.Kind != completionKindMethod {
			t.Errorf("method %q has kind %d, want %d", item.Label, item.Kind, completionKindMethod)
		}
	}
}

func TestCompleteTriggerRouting(t *testing.T) {
	methods, functions := eval.StdlibNames()
	e := newCompletionEngine(methods, functions)
	prog := mustParseProg(t, "$x = 1\noutput = $x")

	t.Run("dot trigger returns methods", func(t *testing.T) {
		items := e.complete("input.", prog, position{Line: 0, Character: 6}, &completionContext{
			TriggerKind:      2,
			TriggerCharacter: ".",
		})
		if len(items) == 0 {
			t.Fatal("expected method completions after dot")
		}
		for _, item := range items {
			if item.Kind != completionKindMethod {
				t.Errorf("got kind %d for %q after dot, want method", item.Kind, item.Label)
			}
		}
	})

	t.Run("dollar trigger returns variables", func(t *testing.T) {
		items := e.complete("$x = 1\noutput = $", prog, position{Line: 1, Character: 10}, &completionContext{
			TriggerKind:      2,
			TriggerCharacter: "$",
		})
		if len(items) == 0 {
			t.Fatal("expected variable completions after $")
		}
		found := false
		for _, item := range items {
			if item.Label == "$x" {
				found = true
			}
			if item.Kind != completionKindVariable {
				t.Errorf("got kind %d for %q after $, want variable", item.Kind, item.Label)
			}
		}
		if !found {
			t.Error("expected $x in variable completions")
		}
	})

	t.Run("no trigger returns general completions", func(t *testing.T) {
		items := e.complete("output = ", prog, position{Line: 0, Character: 9}, nil)
		if len(items) == 0 {
			t.Fatal("expected general completions")
		}
		labels := labelSet(items)
		// Should include keywords and functions.
		if !labels["if"] {
			t.Error("missing keyword 'if' in general completions")
		}
		if !labels["uuid_v4"] {
			t.Error("missing function 'uuid_v4' in general completions")
		}
	})

	t.Run("at-sign trigger returns nil", func(t *testing.T) {
		items := e.complete("output@", prog, position{Line: 0, Character: 7}, &completionContext{
			TriggerKind:      2,
			TriggerCharacter: "@",
		})
		if items != nil {
			t.Errorf("expected nil after @, got %d items", len(items))
		}
	})
}

func TestCompleteTriggerDetectionFromText(t *testing.T) {
	methods, functions := eval.StdlibNames()
	e := newCompletionEngine(methods, functions)

	t.Run("detect dot from text when no trigger context", func(t *testing.T) {
		items := e.complete("input.", nil, position{Line: 0, Character: 6}, nil)
		if len(items) == 0 {
			t.Fatal("expected method completions when cursor after dot")
		}
		if items[0].Kind != completionKindMethod {
			t.Errorf("expected method kind, got %d", items[0].Kind)
		}
	})

	t.Run("detect dollar from text when no trigger context", func(t *testing.T) {
		prog := mustParseProg(t, "$foo = 1\noutput = $foo")
		items := e.complete("$foo = 1\noutput = $", prog, position{Line: 1, Character: 10}, nil)
		if len(items) == 0 {
			t.Fatal("expected variable completions when cursor after $")
		}
	})
}

func TestVariableCompletions(t *testing.T) {
	methods, functions := eval.StdlibNames()
	e := newCompletionEngine(methods, functions)

	t.Run("nil program returns nil", func(t *testing.T) {
		items := e.variableCompletions(nil)
		if items != nil {
			t.Errorf("expected nil, got %d items", len(items))
		}
	})

	t.Run("extracts top-level variables", func(t *testing.T) {
		prog := mustParseProg(t, "$alpha = 1\n$beta = 2\noutput = $alpha + $beta")
		items := e.variableCompletions(prog)
		labels := labelSet(items)
		if !labels["$alpha"] || !labels["$beta"] {
			t.Errorf("expected $alpha and $beta, got %v", labelSlice(items))
		}
	})

	t.Run("deduplicates variables", func(t *testing.T) {
		prog := mustParseProg(t, "$x = 1\n$x = 2\noutput = $x")
		items := e.variableCompletions(prog)
		if len(items) != 1 {
			t.Errorf("expected 1 unique variable, got %d", len(items))
		}
	})

	t.Run("extracts variables from if branches", func(t *testing.T) {
		prog := mustParseProg(t, "if true {\n  $inner = 1\n  output = $inner\n}")
		items := e.variableCompletions(prog)
		labels := labelSet(items)
		if !labels["$inner"] {
			t.Errorf("expected $inner from if body, got %v", labelSlice(items))
		}
	})

	t.Run("extracts variables from match cases", func(t *testing.T) {
		prog := mustParseProg(t, "match input {\n  1 => {\n    $matched = true\n    output = $matched\n  }\n}")
		items := e.variableCompletions(prog)
		labels := labelSet(items)
		if !labels["$matched"] {
			t.Errorf("expected $matched from match body, got %v", labelSlice(items))
		}
	})

	t.Run("results are sorted", func(t *testing.T) {
		prog := mustParseProg(t, "$zebra = 1\n$alpha = 2\n$mid = 3\noutput = $zebra")
		items := e.variableCompletions(prog)
		for i := 1; i < len(items); i++ {
			if items[i].Label < items[i-1].Label {
				t.Errorf("items not sorted: %q before %q", items[i-1].Label, items[i].Label)
			}
		}
	})
}

func TestGeneralCompletionsIncludesMaps(t *testing.T) {
	methods, functions := eval.StdlibNames()
	e := newCompletionEngine(methods, functions)

	prog := mustParseProg(t, "map double(x) {\n  x * 2\n}\noutput = double(input)")
	items := e.generalCompletions(prog)
	labels := labelSet(items)
	if !labels["double"] {
		t.Error("expected user map 'double' in general completions")
	}

	// Verify the map completion has correct metadata.
	for _, item := range items {
		if item.Label == "double" {
			if item.Kind != completionKindFunction {
				t.Errorf("map completion kind = %d, want %d", item.Kind, completionKindFunction)
			}
			if item.Detail != "map double" {
				t.Errorf("map completion detail = %q, want %q", item.Detail, "map double")
			}
		}
	}
}

func TestGeneralCompletionsNilProgram(t *testing.T) {
	methods, functions := eval.StdlibNames()
	e := newCompletionEngine(methods, functions)

	items := e.generalCompletions(nil)
	if len(items) == 0 {
		t.Fatal("expected keywords and functions even with nil program")
	}
	labels := labelSet(items)
	if !labels["if"] || !labels["uuid_v4"] {
		t.Error("expected at least keywords and stdlib functions")
	}
}

// mustParseProg parses a bloblang2 source and returns the optimized program.
func mustParseProg(t *testing.T, source string) *syntax.Program {
	t.Helper()
	prog, errs := syntax.Parse(source, "", nil)
	if len(errs) > 0 {
		t.Fatalf("parse error: %s", syntax.FormatErrors(errs))
	}
	syntax.Optimize(prog)
	return prog
}

func labelSet(items []completionItem) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item.Label] = true
	}
	return s
}

func labelSlice(items []completionItem) []string {
	s := make([]string, len(items))
	for i, item := range items {
		s[i] = item.Label
	}
	return s
}
