package translator_test

import (
	"context"
	"errors"
	"testing"

	v1blobl "github.com/redpanda-data/benthos/v4/public/bloblang"
	bloblangv2 "github.com/redpanda-data/benthos/v4/public/bloblangv2"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestQueryFormMethodsEmitValidV2 guards a whole bug category: V1 methods that
// take a lambda/query argument where V2 requires an explicit lambda. If the
// migrator fails to wrap the query form (e.g. the .fold query-form bug), the
// emitted V2 compiles but blows up at exec ("second argument must be a
// lambda").
//
// The contract for every case: EITHER the migration flags the construct
// Unsupported (no faithful V2 equivalent), OR the emitted V2 must compile AND
// execute AND produce the same value as the V1 engine.
//
// IMPORTANT: the V2 side runs on bloblangv2.GlobalEnvironment() — the SAME
// engine users run migrated mappings on — NOT a bare bloblang2.Interp{}, which
// registers only a subset of the stdlib (it is missing .with/.zip/.format/
// .find_by and others, so validating against it gives false "unknown method"
// failures). Dual-engine comparison (real V1 vs real V2) is what makes this
// robust: a translation that runs but returns the wrong thing fails as loudly
// as one that won't run.
func TestQueryFormMethodsEmitValidV2(t *testing.T) {
	arr := map[string]any{"xs": []any{
		map[string]any{"k": "a", "v": float64(3)},
		map[string]any{"k": "b", "v": float64(1)},
		map[string]any{"k": "c", "v": float64(2)},
	}}
	nums := map[string]any{"ns": []any{float64(1), float64(2), float64(3)}}
	obj := map[string]any{"obj": map[string]any{"x": float64(1), "y": float64(2)}}
	names := map[string]any{"names": []any{"a", "b", "c"}}

	cases := []struct {
		name            string
		v1              string
		in              any
		wantUnsupported bool // no faithful V2 equivalent → must flag Unsupported
	}{
		// Query form (implicit `this` = element / context) — the risky shape.
		{"map_each query-form", `root = this.xs.map_each(this.v * 2)`, arr, false},
		{"filter query-form", `root = this.xs.filter(this.v > 1)`, arr, false},
		{"all query-form", `root = this.xs.all(this.v > 0)`, arr, false},
		{"any query-form", `root = this.xs.any(this.v > 2)`, arr, false},
		{"sort_by query-form", `root = this.xs.sort_by(this.v)`, arr, false},
		{"sort_by then map_each", `root = this.xs.sort_by(this.v).map_each(this.k)`, arr, false},
		{"unique query-form", `root = this.xs.unique(this.k)`, arr, false},
		{"map_each_key query-form", `root = this.obj.map_each_key(this.uppercase())`, obj, false},
		{"find_by query-form", `root = this.xs.find_by(this.k == "b")`, arr, false},
		{"find_all_by query-form", `root = this.xs.find_all_by(this.v > 1)`, arr, false},
		{"fold sum query-form", `root = this.ns.fold(0, this.tally + this.value)`, nums, false},
		{"fold value-only query-form", `root = this.ns.fold(0, this.value)`, nums, false},
		{"fold merge query-form", `root = this.xs.fold({}, this.tally.merge({(this.value.k): this.value.v}))`, arr, false},
		{"find value form", `root = this.names.find("b")`, names, false},

		// Variadic forms V2 supports via a single array argument.
		{"without variadic", `root = this.obj.without("x")`, obj, false},
		{"with variadic", `root = this.obj.with("x", "y")`, obj, false},
		{"zip variadic", `root = this.ns.zip(this.ns)`, nums, false},
		{"format variadic", `root = "%v".format(this.ns.length())`, nums, false},

		// Explicit-lambda form — guard it stays valid too.
		{"map_each lambda-form", `root = this.xs.map_each(x -> x.v * 2)`, arr, false},
		{"fold lambda-form", `root = this.ns.fold(0, item -> item.tally + item.value)`, nums, false},

		// Fold referencing the context as a whole object has no V2 (tally,
		// value) equivalent — must be flagged Unsupported, not emitted as a
		// broken non-lambda .fold arg.
		{"fold bare-this (no V2 equivalent)", `root = this.ns.fold(0, this)`, nums, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := translator.Migrate(context.Background(), tc.v1, translator.Options{MinCoverage: 0})
			var cerr *translator.CoverageError
			if errors.As(err, &cerr) {
				rep = cerr.Report
			} else if err != nil {
				t.Fatalf("Migrate: %v", err)
			}

			if tc.wantUnsupported {
				if !hasRule(rep.Changes, translator.RuleUnsupportedConstruct) {
					t.Fatalf("expected RuleUnsupportedConstruct (no faithful V2 equivalent); V2:\n%s\nchanges:\n%s",
						rep.V2Mapping, changeList(rep.Changes))
				}
				return
			}

			// Not Unsupported → the emitted V2 must compile, run (on the REAL
			// V2 environment), and match V1.
			exec, cErr := bloblangv2.GlobalEnvironment().Parse(rep.V2Mapping)
			if cErr != nil {
				t.Fatalf("emitted V2 did not compile: %v\nV2:\n%s", cErr, rep.V2Mapping)
			}
			gotV2, runErr := exec.Query(tc.in)
			if runErr != nil {
				t.Fatalf("emitted V2 errored at exec: %v\nV2:\n%s", runErr, rep.V2Mapping)
			}

			v1ex, pErr := v1blobl.Parse(tc.v1)
			if pErr != nil {
				t.Fatalf("V1 parse (test setup): %v", pErr)
			}
			gotV1, v1Err := v1ex.Query(tc.in)
			if v1Err != nil {
				t.Fatalf("V1 exec (test setup — pick an input V1 accepts): %v", v1Err)
			}
			if !jsonEqual(gotV1, gotV2) {
				t.Fatalf("V1/V2 mismatch:\n  V1: %v\n  V2: %v\nV2 mapping:\n%s", gotV1, gotV2, rep.V2Mapping)
			}
		})
	}
}
