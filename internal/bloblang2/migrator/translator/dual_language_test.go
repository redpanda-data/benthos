package translator_test

import "testing"

// Per-area differential (V1-vs-V2) regression suites, populated from the
// dual-engine language sweep. Every case here was confirmed to produce
// IDENTICAL output on the real V1 engine and the migrated V2 (run on
// bloblangv2.GlobalEnvironment). They guard the equivalent surface against
// regressions; known DIVERGENCES are handled separately (fixed, or asserted
// as flagged). See runDualCases in dualengine_test.go.
//
// Numbers use float64 (JSON-decoded shape); jsonEqual normalises int64/float64.

func TestDualOperators(t *testing.T) {
	ff := func(a, b float64) map[string]any { return map[string]any{"a": a, "b": b} }
	runDualCases(t, []dualCase{
		{"add", `root = this.a + this.b`, ff(1, 2)},
		{"sub", `root = this.a - this.b`, ff(10, 3)},
		{"mul", `root = this.a * this.b`, ff(-10, 3)},
		{"float div", `root = this.a / this.b`, ff(9, 3)},
		{"mixed int/float add", `root = this.a + this.b`, map[string]any{"a": int64(1), "b": 2.5}},
		{"precedence", `root = 2 + 3 * 4`, map[string]any{}},
		{"parens precedence", `root = (2 + 3) * 4`, map[string]any{}},
		{"eq same-value int/float", `root = this.a == this.b`, map[string]any{"a": int64(5), "b": 5.0}},
		{"lt", `root = this.a < this.b`, ff(1, 2)},
		{"string compare", `root = "a" < "b"`, map[string]any{}},
		{"logical and", `root = this.a > 0 && this.b > 0`, ff(1, 2)},
		{"logical short-circuit", `root = false && (this.a / this.b > 0)`, ff(1, 0)},
		{"not", `root = !(this.a > this.b)`, ff(1, 2)},
		{"string concat literal", `root = "v" + this.a`, map[string]any{"a": "x"}},
		{"nested compare chain", `root = this.a < this.b && this.b < this.c`, map[string]any{"a": 1.0, "b": 2.0, "c": 3.0}},
	})
}

func TestDualStrings(t *testing.T) {
	s := func(v string) map[string]any { return map[string]any{"s": v} }
	runDualCases(t, []dualCase{
		{"uppercase", `root = this.s.uppercase()`, s("naïve héllo")},
		{"lowercase", `root = this.s.lowercase()`, s("HELLO")},
		{"trim no-arg", `root = this.s.trim()`, s("  hi  ")},
		{"trim_prefix", `root = this.s.trim_prefix("go")`, s("gopher")},
		{"trim_suffix", `root = this.s.trim_suffix("!")`, s("hi!")},
		{"split", `root = this.s.split(",")`, s("a,b,c")},
		{"split empty sep", `root = this.s.split("")`, s("abc")},
		{"join", `root = this.xs.join("-")`, map[string]any{"xs": []any{"a", "b"}}},
		{"replace_all", `root = this.s.replace_all("o", "0")`, s("foo")},
		{"contains true", `root = this.s.contains("ell")`, s("hello")},
		{"has_prefix", `root = this.s.has_prefix("he")`, s("hello")},
		{"slice ascii", `root = this.s.slice(0, 3)`, s("abcdef")},
		{"index_of ascii", `root = this.s.index_of("lo")`, s("hello")},
		{"parse_json", `root = this.s.parse_json()`, s(`{"x":1}`)},
		{"number of string", `root = this.s.number()`, s("42.5")},
		{"encode/decode base64 roundtrip", `root = this.s.encode("base64").decode("base64").string()`, s("hi")},
		{"re_match", `root = this.s.re_match("^h.*o$")`, s("hello")},
		// format_json with no args: V1 defaults to a 4-space indent; the
		// migrator emits an explicit "    " so V2 matches. Single-key object
		// keeps output key-order deterministic across engines. The value is
		// deliberately fractional: whole-value floats are a DECLARED residual
		// divergence (V1 renders 1, V2 renders 1.0 — see formatJSONRewrite's
		// explanation), so a whole float here would not be an equivalence case.
		{"format_json default indent", `root = this.o.format_json().string()`, map[string]any{"o": map[string]any{"a": 1.5}}},
	})
}

func TestDualArrays(t *testing.T) {
	a := func(xs ...any) map[string]any { return map[string]any{"a": xs} }
	runDualCases(t, []dualCase{
		{"sort nums", `root = this.a.sort()`, a(3.0, 1.0, 2.0)},
		{"sort strings", `root = this.a.sort()`, a("b", "a", "c")},
		{"sort empty", `root = this.a.sort()`, a()},
		{"sort_by", `root = this.a.sort_by(x -> x.id)`, a(map[string]any{"id": 2.0}, map[string]any{"id": 1.0})},
		{"min", `root = this.a.min()`, a(3.0, 1.0, 2.0)},
		{"max", `root = this.a.max()`, a(3.0, 1.0, 2.0)},
		{"sum", `root = this.a.sum()`, a(3.0, 1.0, 2.0)},
		{"sum empty", `root = this.a.sum()`, a()},
		{"index positive", `root = this.a.index(0)`, a(3.0, 1.0, 2.0)},
		{"index negative", `root = this.a.index(-1)`, a(3.0, 1.0, 2.0)},
		{"slice neg", `root = this.a.slice(-2)`, a(3.0, 1.0, 2.0)},
		{"slice oob clamp", `root = this.a.slice(1, 100)`, a(3.0, 1.0, 2.0)},
		{"unique", `root = this.a.unique()`, a(1.0, 2.0, 2.0, 3.0)},
		{"flatten", `root = this.a.flatten()`, a([]any{1.0, 2.0}, []any{3.0})},
		{"append", `root = this.a.append(9)`, a(3.0, 1.0, 2.0)},
		{"enumerated", `root = this.a.enumerated()`, a("b", "a")},
		{"find value", `root = this.a.find("b")`, a("a", "b", "c")},
		{"contains", `root = this.a.contains(2)`, a(1.0, 2.0, 3.0)},
		{"filter", `root = this.a.filter(x -> x > 1)`, a(3.0, 1.0, 2.0)},
		{"map_each", `root = this.a.map_each(x -> x * 2)`, a(1.0, 2.0)},
		{"fold", `root = this.a.fold(0, item -> item.tally + item.value)`, a(3.0, 1.0, 2.0)},
		// V1 .map(fn) applies fn to the WHOLE value; migrated to V2 .into(fn).
		// Regression guard for the map->into rewrite.
		{"map whole-value into", `root = this.a.map(v -> v.length())`, a(3.0, 1.0, 2.0)},
	})
}

func TestDualObjects(t *testing.T) {
	obj := func() map[string]any {
		return map[string]any{"o": map[string]any{"a": 1.0, "b": 2.0, "c": 3.0}}
	}
	runDualCases(t, []dualCase{
		// NB: values() is nondeterministic in V1 (Go map order) — use keys().sort()
		// or values().sort() for a deterministic comparison.
		{"keys sorted", `root = this.o.keys().sort()`, obj()},
		{"values sorted", `root = this.o.values().sort()`, obj()},
		{"keys length", `root = this.o.keys().length()`, obj()},
		{"without top-level key", `root = this.o.without("a")`, obj()},
		{"without missing key", `root = this.o.without("zzz")`, obj()},
		{"exists present", `root = this.o.exists("a")`, obj()},
		{"exists missing", `root = this.o.exists("zzz")`, obj()},
		{"get present", `root = this.o.get("a")`, obj()},
		{"get missing", `root = this.o.get("zzz")`, obj()},
		{"length", `root = this.o.length()`, obj()},
		{"type", `root = this.o.type()`, obj()},
		{"map_each_key", `root = this.o.map_each_key(k -> k.uppercase())`, obj()},
		{"key_values sorted", `root = this.o.key_values().sort_by(e -> e.key)`, obj()},
		{"merge disjoint", `root = this.a.merge(this.b)`, map[string]any{"a": map[string]any{"x": 1.0}, "b": map[string]any{"y": 2.0}}},
		{"object literal dynamic key", `root = {this.k: this.v}`, map[string]any{"k": "name", "v": "bob"}},
	})
}

func TestDualControlFlow(t *testing.T) {
	runDualCases(t, []dualCase{
		{"if true", `root = if this.a == 1 { "y" } else { "n" }`, map[string]any{"a": 1.0}},
		{"if false", `root = if this.a == 1 { "y" } else { "n" }`, map[string]any{"a": 2.0}},
		{"else-if", `root = if this.a == 1 { "one" } else if this.a == 2 { "two" } else { "other" }`, map[string]any{"a": 2.0}},
		{"match equality hit", `root = match this.a { 1 => "one", _ => "?" }`, map[string]any{"a": 1.0}},
		{"match wildcard", `root = match this.a { 1 => "one", _ => "?" }`, map[string]any{"a": 9.0}},
		{"match this-rebind object subject", `root = match this.p { _ => this.name }`, map[string]any{"p": map[string]any{"name": "bob"}}},
		{"deleted in object value", `root = {"keep": this.a, "gone": deleted()}`, map[string]any{"a": 1.0}},
		{"deleted in array elided", `root = [1, if false { 0 }, 3]`, map[string]any{}},
		{"deep path", `root = this.a.b.c`, map[string]any{"a": map[string]any{"b": map[string]any{"c": 5.0}}}},
		{"missing field null", `root = this.nope`, map[string]any{"a": 1.0}},
		{"catch on missing", `root = this.nope.index(5).catch("fb")`, map[string]any{}},
		{"or on missing", `root = this.nope.or("d")`, map[string]any{}},
		{"coalesce null", `root = this.nope | "d"`, map[string]any{}},
		{"let and use", `let x = this.a + 1
root = $x`, map[string]any{"a": 1.0}},
		{"apply map", `map double { root = this * 2 }
root = this.a.apply("double")`, map[string]any{"a": 5.0}},
	})
}

func TestDualTypesErrors(t *testing.T) {
	runDualCases(t, []dualCase{
		{"number of numeric string", `root = this.s.number()`, map[string]any{"s": "42.5"}},
		{"string of fractional float", `root = this.n.string()`, map[string]any{"n": 42.5}},
		{"string of bool", `root = this.b.string()`, map[string]any{"b": true}},
		{"bool of true string", `root = this.s.bool()`, map[string]any{"s": "true"}},
		{"bool of false string", `root = this.s.bool()`, map[string]any{"s": "false"}},
		{"floor", `root = this.n.floor()`, map[string]any{"n": 3.7}},
		{"ceil", `root = this.n.ceil()`, map[string]any{"n": 3.2}},
		{"round non-half", `root = this.n.round()`, map[string]any{"n": 3.2}},
		{"not_null present", `root = this.a.not_null()`, map[string]any{"a": 1.0}},
		{"catch value", `root = this.s.number().catch(-1)`, map[string]any{"s": "42"}},
		{"coalesce falls through", `root = this.nope | "d"`, map[string]any{}},
		{"coalesce chain", `root = this.a | this.b | "z"`, map[string]any{"b": "found"}},
		{"throw caught", `root = throw("boom").catch("rescued")`, map[string]any{}},
	})
}
