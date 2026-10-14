package translator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestFilterAllAnyNoDoubleCount is the regression for the double-count bug in
// the .filter()/.all()/.any() rewrites. Those methods flag a Warning (V2 is
// strict about the receiver type) via the query-form rename path, which
// records a Rewritten Change. The bug: the node was ALSO tallied as Exact by
// the caller, inflating Coverage.Total by one per occurrence.
//
// The check is differential and robust to node-counting internals: each of
// filter/all/any is a method-call-with-lambda over `this.xs`, structurally
// identical to `.map_each(x -> x > 0)` (a clean rename that is counted exactly
// once). So their Coverage.Total must equal map_each's. A double-count would
// make the flagged method's Total exactly one higher.
func TestFilterAllAnyNoDoubleCount(t *testing.T) {
	total := func(t *testing.T, v1 string) int {
		t.Helper()
		rep, err := translator.Migrate(context.Background(), v1, translator.Options{MinCoverage: 0})
		var cerr *translator.CoverageError
		switch {
		case err == nil:
		case errors.As(err, &cerr):
			rep = cerr.Report
		default:
			t.Fatalf("Migrate(%q): %v", v1, err)
		}
		return rep.Coverage.Total
	}

	baseline := total(t, `root = this.xs.map_each(x -> x > 0)`)
	if baseline == 0 {
		t.Fatal("baseline Coverage.Total is zero")
	}
	for _, method := range []string{"filter", "all", "any"} {
		v1 := `root = this.xs.` + method + `(x -> x > 0)`
		if got := total(t, v1); got != baseline {
			t.Errorf(".%s() Coverage.Total = %d, want %d (matching .map_each()); a mismatch of +1 indicates the double-count regression", method, got, baseline)
		}
		// The Warning must still be recorded (the rename is not silent).
		rep, err := translator.Migrate(context.Background(), v1, translator.Options{MinCoverage: 0})
		var cerr *translator.CoverageError
		if errors.As(err, &cerr) {
			rep = cerr.Report
		} else if err != nil {
			t.Fatalf("Migrate(%q): %v", v1, err)
		}
		if !hasRule(rep.Changes, translator.RuleMethodDoesNotExist) {
			t.Errorf(".%s() did not record the expected receiver-type Warning; got:\n%s", method, changeList(rep.Changes))
		}
	}
}
