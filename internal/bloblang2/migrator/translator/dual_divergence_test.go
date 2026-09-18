package translator_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	v1blobl "github.com/redpanda-data/benthos/v4/public/bloblang"
	bloblangv2 "github.com/redpanda-data/benthos/v4/public/bloblangv2"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestDualDivergences is the counterpart to the equivalence suites: each case
// is a V1 construct that CANNOT be translated to a semantically identical V2
// form, so the migrator must FLAG it (SeverityWarning or SeverityError) rather
// than silently emit a diverging translation.
//
// Two things are asserted per case:
//  1. The migrated V2 still compiles and runs on the REAL V2 engine
//     (bloblangv2.GlobalEnvironment) — a flagged divergence must not produce
//     broken output.
//  2. At least one emitted Change is SeverityWarning/Error whose Explanation
//     contains wantExpl — proving the user is warned about this exact hazard.
//
// The `divergesOn` input additionally documents (via a sub-assertion) that the
// two engines genuinely produce different values there, so a case can't rot
// into a false alarm if the semantics ever converge — if V1==V2 on that input,
// the divergence is gone and the flag (and this case) should be reconsidered.
func TestDualDivergences(t *testing.T) {
	cases := []struct {
		name       string
		v1         string
		divergesOn any    // input on which V1 and V2 genuinely differ
		wantExpl   string // substring the warning's Explanation must contain
	}{
		{
			name:       "get dot-path arg",
			v1:         `root = this.o.get("x.y")`,
			divergesOn: map[string]any{"o": map[string]any{"x": map[string]any{"y": 1.0}}},
			wantExpl:   "path",
		},
		{
			name:       "exists dot-path arg",
			v1:         `root = this.o.exists("x.y")`,
			divergesOn: map[string]any{"o": map[string]any{"x": map[string]any{"y": 1.0}}},
			wantExpl:   "path",
		},
		{
			name:       "without dot-path arg",
			v1:         `root = this.o.without("x.y")`,
			divergesOn: map[string]any{"o": map[string]any{"x": map[string]any{"y": 1.0}, "z": 2.0}},
			wantExpl:   "path",
		},
		{
			name:       "bool permissive coercion",
			v1:         `root = this.s.bool()`,
			divergesOn: map[string]any{"s": "TRUE"},
			wantExpl:   "coerc",
		},
		{
			name:       "round half away-from-zero vs banker's",
			v1:         `root = this.n.round()`,
			divergesOn: map[string]any{"n": 2.5},
			wantExpl:   "round",
		},
		{
			name:       "all on empty array vacuous truth",
			v1:         `root = this.a.all(x -> x > 0)`,
			divergesOn: map[string]any{"a": []any{}},
			wantExpl:   "empty",
		},
		{
			name:       "trim cutset dropped",
			v1:         `root = this.s.trim("xy")`,
			divergesOn: map[string]any{"s": "xyhixy"},
			wantExpl:   "trim",
		},
		{
			name:       "match without wildcard",
			v1:         `root = match this.a { 5 => "five" }`,
			divergesOn: map[string]any{"a": 1.0},
			wantExpl:   "wildcard",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := translator.Migrate(context.Background(), tc.v1, translator.Options{
				Mode:        translator.ModeMapping,
				MinCoverage: 0,
			})
			var cerr *translator.CoverageError
			if errors.As(err, &cerr) {
				rep = cerr.Report
			} else if err != nil {
				t.Fatalf("migrate: %v", err)
			}

			// 1. Migrated V2 must still compile and run.
			v2ex, err := bloblangv2.GlobalEnvironment().Parse(rep.V2Mapping)
			if err != nil {
				t.Fatalf("migrated V2 did not compile: %v\nV2:\n%s", err, rep.V2Mapping)
			}

			// 2. A warning/error whose Explanation names the hazard.
			if !hasFlaggedExpl(rep.Changes, tc.wantExpl) {
				t.Errorf("expected a Warning/Error Change mentioning %q; got:\n%s",
					tc.wantExpl, changeList(rep.Changes))
			}

			// 3. Sanity: the engines really do diverge on divergesOn. If they
			//    agree, the divergence has been resolved and this case (and the
			//    flag it guards) should be revisited.
			v1ex, perr := v1blobl.Parse(tc.v1)
			if perr != nil {
				t.Fatalf("V1 parse (test setup): %v", perr)
			}
			gotV1, v1Err := v1ex.Query(tc.divergesOn)
			gotV2, v2Err := v2ex.Query(tc.divergesOn)
			// A divergence counts if either engine errors where the other
			// doesn't, or both succeed with different values.
			diverged := (v1Err == nil) != (v2Err == nil) ||
				(v1Err == nil && v2Err == nil && !jsonEqual(gotV1, gotV2))
			if !diverged {
				t.Errorf("expected V1/V2 to diverge on %v but both gave %v (v1Err=%v v2Err=%v) — divergence resolved?",
					tc.divergesOn, gotV1, v1Err, v2Err)
			}
		})
	}
}

// hasFlaggedExpl reports whether any Warning/Error change's Explanation
// contains sub (case-insensitive).
func hasFlaggedExpl(changes []translator.Change, sub string) bool {
	sub = strings.ToLower(sub)
	for _, c := range changes {
		if c.Severity == translator.SeverityWarning || c.Severity == translator.SeverityError {
			if strings.Contains(strings.ToLower(c.Explanation), sub) {
				return true
			}
		}
	}
	return false
}
