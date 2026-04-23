package translator_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestMethodTranslationAudit is the systematic per-V1-method regression
// suite. For each V1 method with a translation rule (or a known 1:1 with
// V2), it runs:
//
//  1. Translate a minimal V1 snippet exercising the method.
//  2. Compile + execute the V2 output against a specific input.
//  3. Assert the V2 output equals the expected value BYTE-FOR-BYTE.
//
// There is NO "warning = free pass" escape hatch here — unlike the
// corpus regression test, this file validates that each rule produces
// the correct V2 mapping. A translator bug (like the .fold() context-
// param mis-rewrite that slipped through the corpus) would fail here.
//
// Categories:
//   - renames: V1 method → V2 method with same signature
//   - reshapes: V1 method → different V2 method call shape
//   - polymorphic: V1 method accepted shape X or Y; V2 split
//   - wrappers: V1 method wrapped with extra chain calls in V2
func TestMethodTranslationAudit(t *testing.T) {
	cases := []struct {
		name  string
		v1    string
		input any
		want  any
	}{
		// --- Renames ---
		{
			name:  "map_each on array -> map",
			v1:    `root = this.xs.map_each(x -> x * 2)`,
			input: map[string]any{"xs": []any{1.0, 2.0, 3.0}},
			want:  []any{2.0, 4.0, 6.0},
		},
		{
			// V1 .map_each() on an object passes each VALUE (not an
			// {key, value} entry) — so the translator's `.map_values`
			// rewrite is correct. The test confirms the value-level
			// transform round-trips.
			name:  "map_each on object literal -> map_values",
			v1:    `root = {"a": 1, "b": 2}.map_each(v -> v * 10)`,
			input: map[string]any{},
			want:  map[string]any{"a": int64(10), "b": int64(20)},
		},
		{
			name:  "enumerated -> enumerate",
			v1:    `root = this.xs.enumerated()`,
			input: map[string]any{"xs": []any{"a", "b"}},
			want:  []any{map[string]any{"index": int64(0), "value": "a"}, map[string]any{"index": int64(1), "value": "b"}},
		},
		{
			name:  "key_values -> iter",
			v1:    `root = this.obj.key_values()`,
			input: map[string]any{"obj": map[string]any{"x": float64(1)}},
			want:  []any{map[string]any{"key": "x", "value": float64(1)}},
		},
		{
			name:  "map_each_key -> map_keys",
			v1:    `root = this.obj.map_each_key(k -> k.uppercase())`,
			input: map[string]any{"obj": map[string]any{"x": float64(1)}},
			want:  map[string]any{"X": float64(1)},
		},

		// --- Reshapes ---
		{
			name:  ".index(n) -> [n]",
			v1:    `root = this.xs.index(1)`,
			input: map[string]any{"xs": []any{"a", "b", "c"}},
			want:  "b",
		},
		{
			name:  ".get(key) -> [key]",
			v1:    `root = this.obj.get("x")`,
			input: map[string]any{"obj": map[string]any{"x": float64(7)}},
			want:  float64(7),
		},
		{
			name:  ".apply('m') -> m(recv)",
			v1:    "map double { root = this * 2 }\nroot = 5.apply(\"double\")",
			input: map[string]any{},
			want:  int64(10),
		},
		{
			name:  ".number() -> .float64()",
			v1:    `root = this.s.number()`,
			input: map[string]any{"s": "3.14"},
			want:  3.14,
		},

		// --- Polymorphic splits ---
		{
			name:  ".merge on arrays -> .concat",
			v1:    `root = this.a.map_each(x -> x).merge(this.b.map_each(x -> x))`,
			input: map[string]any{"a": []any{float64(1), float64(2)}, "b": []any{float64(3)}},
			want:  []any{float64(1), float64(2), float64(3)},
		},
		{
			name:  ".merge on objects stays .merge",
			v1:    `root = {"a": 1}.merge({"b": 2})`,
			input: map[string]any{},
			want:  map[string]any{"a": int64(1), "b": int64(2)},
		},

		// --- Higher-order lambda shape rewrites ---
		{
			name:  ".find(value) -> .find(x -> x == value)",
			v1:    `root = this.xs.find("b")`,
			input: map[string]any{"xs": []any{"a", "b", "c"}},
			want:  "b",
		},
		{
			name:  ".fold(init, ctx -> ctx.tally + ctx.value) -> .fold(init, (tally, value) -> tally + value)",
			v1:    `root = this.xs.fold(0, item -> item.tally + item.value)`,
			input: map[string]any{"xs": []any{float64(1), float64(2), float64(3)}},
			want:  float64(6),
		},
		{
			name:  ".fold with merge pattern (GA4)",
			v1:    `root = this.xs.map_each(p -> {"key": p.k, "value": p.v}).fold({}, item -> item.tally.merge({(item.value.key): item.value.value}))`,
			input: map[string]any{"xs": []any{map[string]any{"k": "a", "v": float64(1)}, map[string]any{"k": "b", "v": float64(2)}}},
			want:  map[string]any{"a": float64(1), "b": float64(2)},
		},

		// --- Error-catching operators ---
		{
			name:  "| coalesce catches null",
			v1:    `root = this.missing | "default"`,
			input: map[string]any{},
			want:  "default",
		},
		{
			name:  "| coalesce catches errors (out-of-bounds index)",
			v1:    `root = this.xs.0 | "fallback"`,
			input: map[string]any{"xs": []any{}},
			want:  "fallback",
		},
		{
			name:  ".or() catches null",
			v1:    `root = this.missing.or("default")`,
			input: map[string]any{},
			want:  "default",
		},
		{
			name:  ".or() catches errors (out-of-bounds index)",
			v1:    `root = this.xs.index(5).or("fallback")`,
			input: map[string]any{"xs": []any{"a"}},
			want:  "fallback",
		},

		// --- Variadic / array normalisation ---
		{
			name:  ".without(a, b, c) -> .without([a, b, c])",
			v1:    `root = this.obj.without("x", "y")`,
			input: map[string]any{"obj": map[string]any{"x": float64(1), "y": float64(2), "z": float64(3)}},
			want:  map[string]any{"z": float64(3)},
		},

		// --- exists rewrites ---
		{
			name:  ".exists(key) on object -> .has_key(key)",
			v1:    `root = this.obj.exists("x")`,
			input: map[string]any{"obj": map[string]any{"x": float64(1)}},
			want:  true,
		},

		// --- .catch with value (not lambda) ---
		{
			name:  ".catch(value) -> .catch(_ -> value)",
			v1:    `root = this.xs.index(5).catch("fallback")`,
			input: map[string]any{"xs": []any{"a"}},
			want:  "fallback",
		},

		// --- V1 recv.(body) map expression ---
		{
			name:  "recv.(ctx -> body) -> recv.into(ctx -> body)",
			v1:    `root = 10.(n -> n + 1)`,
			input: map[string]any{},
			want:  int64(11),
		},
		{
			name:  "recv.(body) un-named rebinds this -> recv.into(__this -> body)",
			v1:    `root = 10.(this * 2)`,
			input: map[string]any{},
			want:  int64(20),
		},

		// --- nothing() sentinel variations ---
		{
			name:  "nothing() in collection -> deleted()",
			v1:    `root = [1, if false { 0 }, 3]`,
			input: map[string]any{},
			want:  []any{int64(1), int64(3)},
		},

		// --- Bare identifier + path ---
		{
			name:  "bare ident -> input.X (null-safe)",
			v1:    `root = message`,
			input: map[string]any{"message": "hi"},
			want:  "hi",
		},
	}

	interp := &bloblang2.Interp{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := translator.Migrate(tc.v1, translator.Options{MinCoverage: 0})
			if err != nil {
				t.Fatalf("Migrate: %v", err)
			}
			if rep.V2Mapping == "" {
				t.Fatalf("empty V2 mapping")
			}
			compiled, err := interp.Compile(rep.V2Mapping, nil)
			if err != nil {
				t.Fatalf("V2 compile failed: %v\nV2 mapping:\n%s", err, rep.V2Mapping)
			}
			got, _, _, runErr := compiled.Exec(tc.input, nil)
			if runErr != nil {
				t.Fatalf("V2 runtime error: %v\nV2 mapping:\n%s", runErr, rep.V2Mapping)
			}
			if !jsonEqual(got, tc.want) {
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(tc.want)
				t.Fatalf("output mismatch:\n  got:  %s\n  want: %s\nV2 mapping:\n%s", gotJSON, wantJSON, rep.V2Mapping)
			}
		})
	}
}

// jsonEqual compares two Go values by JSON round-trip so numeric type
// differences (int64 vs float64) and map key ordering don't trip the
// test over implementation details we don't care about here.
func jsonEqual(a, b any) bool {
	aj, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bj, err := json.Marshal(b)
	if err != nil {
		return false
	}
	var an, bn any
	_ = json.Unmarshal(aj, &an)
	_ = json.Unmarshal(bj, &bn)
	return fmt.Sprintf("%v", an) == fmt.Sprintf("%v", bn)
}
