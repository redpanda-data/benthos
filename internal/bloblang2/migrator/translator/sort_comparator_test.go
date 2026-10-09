package translator_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestSortComparatorNotMisrewritten is the regression for #436's `.sort`
// comparator finding. V1 `.sort(left > right)` uses implicit left/right
// comparator identifiers; V2 has no comparator sort. The translator must NOT
// emit `output.sort(input.left > input.right)` (nonsense produced by running
// the bare-identifier rewrite over the dropped comparator) and must flag the
// construct Unsupported rather than silently counting it as an exact
// translation.
func TestSortComparatorNotMisrewritten(t *testing.T) {
	const v1 = `root = this.items.sort(left > right)`

	rep, err := translator.Migrate(context.Background(), v1, translator.Options{MinCoverage: 0.0001})
	var cerr *translator.CoverageError
	switch {
	case err == nil:
	case errors.As(err, &cerr):
		rep = cerr.Report
	default:
		t.Fatalf("Migrate(%q) failed: %v", v1, err)
	}

	if strings.Contains(rep.V2Mapping, "input.left") || strings.Contains(rep.V2Mapping, "input.right") {
		t.Errorf("comparator was mis-rewritten into input.left/input.right; got:\n%s", rep.V2Mapping)
	}
	if !strings.Contains(rep.V2Mapping, ".sort()") {
		t.Errorf("expected a plain ascending .sort() in the output; got:\n%s", rep.V2Mapping)
	}
	if !hasRule(rep.Changes, translator.RuleUnsupportedConstruct) {
		t.Errorf("expected an Unsupported change (RuleUnsupportedConstruct) for the dropped comparator; got:\n%s", changeList(rep.Changes))
	}

	// A plain no-arg .sort() must still translate cleanly (no Unsupported).
	rep2, err := translator.Migrate(context.Background(), `root = this.items.sort()`, translator.Options{MinCoverage: 0.0001})
	if err != nil {
		var c2 *translator.CoverageError
		if !errors.As(err, &c2) {
			t.Fatalf("Migrate(no-arg sort) failed: %v", err)
		}
		rep2 = c2.Report
	}
	if hasRule(rep2.Changes, translator.RuleUnsupportedConstruct) {
		t.Errorf("no-arg .sort() should not be Unsupported; got:\n%s", changeList(rep2.Changes))
	}
}
