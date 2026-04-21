package v1spec_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1spec"
)

// TestV1Quirks exercises V1-specific behaviours that the YAML corpus doesn't
// cover because those behaviours have no V2 counterpart to translate from.
// Each case documents a specific claim from bloblang_v1_spec.md so the spec
// and reality cannot silently drift.
//
// Each entry is one of:
//   - output: a concrete Go value the mapping must produce (reflect.DeepEqual)
//   - runtimeErr: substring that must appear in the runtime error
//   - compileErr: substring that must appear in the compile error
//
// Exactly one of the three must be set per case.
func TestV1Quirks(t *testing.T) {
	for _, c := range []struct {
		// Section reference in bloblang_v1_spec.md for traceability.
		spec string
		// Human-readable claim being verified.
		name string
		// The V1 mapping.
		mapping string
		// Optional input document (default nil).
		input any

		output     any
		runtimeErr string
		compileErr string
	}{
		// §2.1 — whitespace and newline rules around assignment =
		{
			spec: "§2.1", name: "assignment = needs whitespace before",
			mapping: "root.a =1", compileErr: "expected whitespace",
		},
		{
			spec: "§2.1", name: "assignment = needs whitespace after",
			mapping: "root.a= 1", compileErr: "expected whitespace",
		},
		{
			spec: "§2.1", name: "let = needs whitespace",
			mapping: "let x=5", compileErr: "expected whitespace",
		},
		{
			spec: "§2.1", name: "binary + does not need whitespace",
			mapping: "root = 1+2", output: int64(3),
		},

		// §5.1 — ! is single-use (not stackable)
		{
			spec: "§5.1", name: "double-not !!x is a parse error",
			mapping: "root = !!true", compileErr: "expected query",
		},
		{
			spec: "§5.1", name: "parenthesised double-not works",
			mapping: "root = !(!true)", output: true,
		},

		// §3 — .type() on sentinels
		{
			spec: "§3", name: "deleted().type() returns \"delete\"",
			mapping: "root = deleted().type()", output: "delete",
		},
		{
			spec: "§3", name: "nothing().type() returns \"nothing\"",
			mapping: "root = nothing().type()", output: "nothing",
		},
		{
			spec: "§3", name: "now() returns a string, not a timestamp",
			mapping: "root = now().type()", output: "string",
		},
		{
			spec: "§3", name: ".number() always returns float64",
			// Going through a method that is type-strict between int and float.
			mapping: `root = "42".number()`, output: float64(42),
		},

		// §4.3 — string escapes
		{
			spec: "§4.3", name: "backslash-slash escape is not supported",
			mapping: `root = "\/"`, compileErr: "failed to unescape",
		},
		{
			spec: "§4.3", name: "triple-quoted string is raw",
			mapping: "root = \"\"\"line1\\nline2\"\"\"", output: `line1\nline2`,
		},

		// §4.5 — object key classification
		{
			spec: "§4.5", name: "int literal as object key is a parse error",
			mapping: `root = {5: "v"}`, compileErr: "object keys must be strings",
		},
		{
			spec: "§4.5", name: "bare ident as object key parses (legacy this.ident dynamic key)",
			mapping: `root = {a: 1}`, input: map[string]any{"a": "dyn"},
			output: map[string]any{"dyn": int64(1)},
		},
		{
			spec: "§4.5", name: "bare ident key with null this.ident errors at runtime",
			mapping: `root = {a: 1}`, input: map[string]any{},
			runtimeErr: "invalid key type",
		},

		// §5.3 — constant-folding scope
		{
			spec: "§5.3", name: "constant folding: literal divide-by-zero is a compile error",
			mapping: "root = 5 / 0", compileErr: "divide by zero",
		},
		{
			spec: "§5.3", name: "constant folding: literal type mismatch on + is a compile error",
			mapping: `root = 5 + "x"`, compileErr: "cannot add types",
		},
		{
			spec: "§5.3", name: "&& does NOT constant-fold; literal operands defer to runtime",
			mapping: `root = true && "x"`, runtimeErr: "expected bool",
		},
		{
			spec: "§5.3", name: "|| does NOT constant-fold; literal operands defer to runtime",
			mapping: `root = false || "x"`, runtimeErr: "expected bool",
		},
		{
			spec: "§5.3", name: "| (coalesce) does NOT constant-fold; runs at runtime",
			mapping: `root = null | "fallback"`, output: "fallback",
		},

		// §5.3 / §14#24 — short-circuit applies at the operator, not through null-safe access
		{
			spec: "§14#24", name: "short-circuit: false && X never evaluates X",
			// Use a runtime divisor so the RHS is not constant-folded. If
			// short-circuit weren't working, this would raise divide-by-zero.
			mapping: "root = false && (1 / this.zero > 0)",
			input:   map[string]any{"zero": int64(0)},
			output:  false,
		},
		{
			spec: "§14#24", name: "this != null && this.foo > 0 is NOT safe on {}",
			mapping: "root = this != null && this.foo > 0", input: map[string]any{},
			runtimeErr: "compare types null",
		},

		// §5.3 — comparison operand coercion
		{
			spec: "§5.3", name: "true == 1 is true (bool path coerces number to bool)",
			mapping: "root = true == 1", output: true,
		},
		{
			spec: "§5.3", name: "1 == true is false (number path cannot coerce bool)",
			mapping: "root = 1 == true", output: false,
		},
		{
			spec: "§5.3", name: "&& coerces a numeric RHS to bool",
			mapping: "root = true && 1", output: true,
		},
		{
			spec: "§5.3", name: "|| rejects a string RHS",
			mapping: `root = false || "x"`, runtimeErr: "expected bool",
		},
		{
			spec: "§5.3", name: "% silently truncates float operands to int64",
			mapping: "root = 7.5 % 2.5", output: int64(1),
		},

		// §6.4 — target grammar
		{
			spec: "§6.4", name: "this as assignment target creates literal \"this\" key",
			mapping: `this.foo = "bar"`,
			output:  map[string]any{"this": map[string]any{"foo": "bar"}},
		},
		{
			spec: "§6.4", name: "meta(expr) = v is not a valid assignment target",
			mapping: `meta("foo") = "bar"`, compileErr: "",
			// Any compile error is acceptable — the parser refuses this form.
		},
		{
			spec: "§6.4", name: "meta = <non-object> errors at runtime",
			mapping: `meta = "string"`, runtimeErr: "object value",
		},
		{
			spec: "§6.4", name: "$x = value (var reassignment) is a parse error",
			mapping: "let x = 1\n$x = 2", compileErr: "",
			// Any compile error is acceptable.
		},

		// §6.3 — numeric-segment writes
		{
			spec: "§6.3", name: "numeric path write into NEW parent creates object key",
			mapping: `root.items.0 = "x"`,
			output:  map[string]any{"items": map[string]any{"0": "x"}},
		},
		{
			spec: "§6.3", name: "numeric path write into EXISTING array indexes the array",
			mapping: `root.items = [1, 2, 3]
root.items.0 = "x"`,
			output: map[string]any{"items": []any{"x", int64(2), int64(3)}},
		},
		{
			spec: "§6.3", name: "numeric path write OOB on existing array errors at runtime",
			mapping: `root.items = [1, 2]
root.items.5 = "x"`,
			runtimeErr: "exceeded target array size",
		},

		// §6.5 — iterator vs non-iterator argument rebinding
		{
			spec: "§6.5", name: "iterator method with non-lambda arg rebinds this to element",
			mapping: `root = [1, 2, 3].map_each(this * 10)`,
			output:  []any{int64(10), int64(20), int64(30)},
		},
		{
			spec: "§6.5", name: "iterator method with lambda pops context; body this is outer",
			mapping: `root = [1, 2, 3].map_each(x -> x + this.offset)`,
			input:   map[string]any{"offset": int64(100)},
			output:  []any{int64(101), int64(102), int64(103)},
		},

		// §6.1 / §9.4 — path access null-tolerance universal
		{
			spec: "§12.5", name: "field access on string returns null",
			mapping: `root = "hello".missing`, output: nil,
		},
		{
			spec: "§12.5", name: "field access on number returns null",
			mapping: `root = (5).missing`, output: nil,
		},
		{
			spec: "§9.4", name: "deleted().foo returns null via null-tolerant path access",
			mapping: `root = deleted().foo`, output: nil,
		},
		{
			spec: "§9.4", name: "nothing().foo returns null",
			mapping: `root = nothing().foo`, output: nil,
		},
		{
			spec: "§12.5", name: ".index() OOB is a runtime error, not null",
			mapping:    `root = [1, 2, 3].index(10)`,
			runtimeErr: "out of bounds",
		},

		// §9.4 — sentinels in array/object literals
		{
			spec: "§9.4", name: "nothing() in array literal is elided",
			mapping: "root = [1, nothing(), 3]",
			output:  []any{int64(1), int64(3)},
		},
		{
			spec: "§9.4", name: "deleted() in array literal is elided",
			mapping: "root = [1, deleted(), 3]",
			output:  []any{int64(1), int64(3)},
		},
		{
			spec: "§9.4", name: "nothing() in object literal elides the key",
			mapping: `root = {"a": 1, "b": nothing()}`,
			output:  map[string]any{"a": int64(1)},
		},
		{
			spec: "§9.4", name: "deleted() in object literal elides the key",
			mapping: `root = {"a": 1, "b": deleted()}`,
			output:  map[string]any{"a": int64(1)},
		},

		// §7.2 — quoted let names are write-only
		{
			spec: "§7.2", name: "let with quoted non-ident name parses",
			mapping: `let "has space" = 5
root = 1`, output: int64(1),
		},
		{
			spec: "§7.2", name: "reading a quoted-non-ident var is a parse error",
			mapping: `let "has space" = 5
root = $"has space"`, compileErr: "expected query",
		},

		// §7.3 — statement-form if with null condition errors (vs expression form)
		{
			spec: "§7.3", name: "statement-form if null errors",
			mapping: `if null { root.x = 1 } else { root.x = 2 }`,
			runtimeErr: "non-boolean",
		},
		{
			spec: "§8.3", name: "expression-form if null treats null as falsy",
			mapping: `root = if null { "yes" } else { "no" }`,
			output:  "no",
		},

		// §8.3 / §8.4 — nothing sentinel from if/match
		{
			spec: "§8.3", name: "if-without-else no-match returns nothing sentinel (field absent)",
			mapping: `root.a = 1
root.a = if false { 99 }`,
			output: map[string]any{"a": int64(1)},
		},
		{
			spec: "§8.4", name: "match with no matching arm returns nothing (assignment skipped)",
			mapping: `root.a = 1
root.a = match this.t { "no" => 2 }`,
			input:  map[string]any{"t": "yes"},
			output: map[string]any{"a": int64(1)},
		},
		{
			spec: "§14#17", name: "root = nothing() preserves input unchanged",
			mapping: `root = nothing()`,
			input:   map[string]any{"pass": "through"},
			output:  map[string]any{"pass": "through"},
		},

		// §8.4 — match literal classification after constant folding
		{
			spec: "§8.4", name: "match pattern (2+1) is constant-folded and used as literal",
			mapping: `root = match 3 { (2+1) => "matched" }`,
			output:  "matched",
		},

		// §8.5 — sort multi-param lambda rejected
		{
			spec: "§8.5", name: "sort(left, right -> ...) multi-param lambda is a parse error",
			mapping:    `root = [3,1,2].sort(left, right -> left > right)`,
			compileErr: "wrong number of arguments",
		},
		{
			spec: "§8.5", name: "sort(left > right) implicit-param form works",
			mapping: `root = [3,1,2].sort(left > right)`,
			output:  []any{int64(3), int64(2), int64(1)},
		},

		// §9.0 — methods that do NOT exist in V1 (require impl/pure NOT to help)
		{
			spec: "§9.0", name: "sqrt method does not exist",
			mapping: `root = (4).sqrt()`, compileErr: "unrecognised method",
		},
		{
			spec: "§9.0", name: "map_values method does not exist",
			mapping: `root = {"a":1}.map_values(v -> v * 2)`, compileErr: "unrecognised method",
		},
		{
			spec: "§9.0", name: "map_entries method does not exist",
			mapping: `root = {"a":1}.map_entries(e -> e)`, compileErr: "unrecognised method",
		},
		{
			spec: "§9.0", name: "filter_entries method does not exist",
			mapping: `root = {"a":1}.filter_entries(e -> true)`, compileErr: "unrecognised method",
		},
		{
			spec: "§9.0", name: "collect method does not exist",
			mapping: `root = [1,2].collect(x -> x)`, compileErr: "unrecognised method",
		},
		{
			spec: "§9.0", name: "chunk method does not exist",
			mapping: `root = [1,2,3].chunk(2)`, compileErr: "unrecognised method",
		},
		{
			spec: "§9.0", name: "char method does not exist",
			mapping: `root = (65).char()`, compileErr: "unrecognised method",
		},
		{
			spec: "§9.0", name: "array reverse does not exist (strings only)",
			mapping:    `root = [1,2,3].reverse()`,
			runtimeErr: "expected string value, got array",
		},
		{
			spec: "§9.0", name: ".round(N) with precision arg does not exist",
			mapping:    `root = 3.14.round(2)`,
			compileErr: "wrong number of arguments",
		},
		{
			spec: "§9.0", name: "ts_add_duration does not exist (use ts_add_iso8601)",
			mapping: `root = now().ts_add_duration("1h")`, compileErr: "unrecognised method",
		},

		// §9.0 — methods that DO exist (impl/pure loaded by harness)
		{
			spec: "§9.0", name: "abs exists (impl/pure)",
			mapping: `root = (-5).abs()`, output: int64(5),
		},
		{
			spec: "§9.0", name: "int64 typed conversion exists (impl/pure)",
			mapping: `root = "42".int64()`, output: int64(42),
		},
		{
			spec: "§9.0", name: "ceil is a core method (no impl/pure required)",
			mapping: `root = 3.2.ceil()`, output: int64(4),
		},
		{
			spec: "§9.0", name: "round (zero-arg) is a core method",
			mapping: `root = 3.6.round()`, output: int64(4),
		},

		// §14#64 — throw error mentions `why`, not `throw`
		{
			spec: "§14#64", name: "throw with non-string arg errors mentioning `why`",
			mapping: `root = throw(5)`, compileErr: "why",
		},

		// §14#67 — path-collision runtime error
		{
			spec: "§14#67", name: "assigning into scalar path errors with \"non-object type\"",
			mapping: `root.user = "Alice"
root.user.name = "Jane"`,
			runtimeErr: "non-object type",
		},
	} {
		t.Run(c.spec+"/"+c.name, func(t *testing.T) {
			m, cerr := v1spec.V1Interp{}.Compile(c.mapping, nil)

			if c.compileErr != "" || (c.compileErr == "" && cerr != nil && c.runtimeErr == "" && c.output == nil) {
				// Compile-error case.
				if cerr == nil {
					t.Fatalf("expected compile error containing %q, got success", c.compileErr)
				}
				if c.compileErr != "" && !strings.Contains(cerr.Error(), c.compileErr) {
					t.Fatalf("compile error %q does not contain %q", cerr.Error(), c.compileErr)
				}
				return
			}
			if cerr != nil {
				t.Fatalf("unexpected compile error: %v", cerr)
			}

			out, _, _, rerr := m.Exec(c.input, map[string]any{})
			if c.runtimeErr != "" {
				if rerr == nil {
					t.Fatalf("expected runtime error containing %q, got success (output=%#v)", c.runtimeErr, out)
				}
				if !strings.Contains(rerr.Error(), c.runtimeErr) {
					t.Fatalf("runtime error %q does not contain %q", rerr.Error(), c.runtimeErr)
				}
				return
			}
			if rerr != nil {
				t.Fatalf("unexpected runtime error: %v", rerr)
			}

			if !reflect.DeepEqual(out, c.output) {
				t.Fatalf("output mismatch:\n  got:  %#v (%T)\n  want: %#v (%T)", out, out, c.output, c.output)
			}
		})
	}
}
