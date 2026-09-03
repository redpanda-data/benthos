package translator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestCoalesceDuplicatesFallback is the regression for the `|`/`.or` rewrite
// duplicating a side-effecting fallback. `x | f` and `x.or(f)` become
// `x.or(f).catch(_ -> f)`, evaluating f twice on the error path. When f is
// nondeterministic/side-effecting (uuid_v4(), .apply(), ...) that diverges
// from V1's single evaluation and must be flagged; when f is pure it must not.
func TestCoalesceDuplicatesFallback(t *testing.T) {
	migrate := func(t *testing.T, v1 string) *translator.Report {
		t.Helper()
		rep, err := translator.Migrate(context.Background(), v1, translator.Options{MinCoverage: 0.0001})
		var cerr *translator.CoverageError
		switch {
		case err == nil:
		case errors.As(err, &cerr):
			rep = cerr.Report
		default:
			t.Fatalf("Migrate(%q) failed: %v", v1, err)
		}
		return rep
	}

	flagged := []string{
		`root = this.a | uuid_v4()`,
		`root = this.a.or(uuid_v4())`,
		`root = this.a | now()`,
		`root = this.a.or(this.b.apply("m"))`,
		`root = this.a | count("c")`,                            // registry-impure (stateful counter)
		`root = this.a | this.xs.map_each(_ -> uuid_v4())`,      // nondeterministic nested in a lambda body
		`root = this.a | {"id": uuid_v4()}`,                     // nested in an object-literal VALUE
		`root = this.a | {uuid_v4(): "x"}`,                      // nested in an object-literal KEY
		`root = this.a | if this.b { uuid_v4() }`,               // nested in an if branch
		`root = this.a | [1, uuid_v4()]`,                        // nested in an array element
		`root = this.a | match this.b { _ => uuid_v4() }`,       // nested in a match case body
		`root = this.a | {"o": {"id": [now()]}}`,                // 2+ levels deep (object > object > array)
		`root = this.a | if this.b { this.c } else { ksuid() }`, // nested in an if ELSE branch
	}
	for _, v1 := range flagged {
		rep := migrate(t, v1)
		if !hasRule(rep.Changes, translator.RuleCoalesceDuplicatesFallback) {
			t.Errorf("expected RuleCoalesceDuplicatesFallback for %q; got:\n%s", v1, changeList(rep.Changes))
		}
	}

	pure := []string{
		`root = this.a | this.b`,
		`root = this.a.or("default")`,
		`root = this.a | this.b.uppercase()`,
		`root = this.a | if this.b { this.c } else { this.d }`, // compound but pure: must NOT flag
		`root = this.a | [this.b, this.c]`,                     // pure array literal
	}
	for _, v1 := range pure {
		rep := migrate(t, v1)
		if hasRule(rep.Changes, translator.RuleCoalesceDuplicatesFallback) {
			t.Errorf("did not expect RuleCoalesceDuplicatesFallback for pure fallback %q; got:\n%s", v1, changeList(rep.Changes))
		}
	}
}
