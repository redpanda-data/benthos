package translator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

func mustMigrate(t *testing.T, v1 string) *translator.Report {
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
	return rep
}

// TestRewriteReturnNilNotDoubleCounted guards the recordedByRewrite coverage
// guard for the expressions.go method path — the .slice/.contains/.reverse
// class of rules that record a Rewritten/Unsupported flag and then return nil
// to reuse the 1:1 V2 shape. Reverting the guard makes each also tally Exact,
// bumping Total (and Translated) by one. filter_count_test covers only the
// methods.go queryFormRename path, so this pins the expressions.go path with
// exact counts. If a legitimate change alters how these translate, update the
// constants — but confirm the delta isn't a resurrected double-count first.
func TestRewriteReturnNilNotDoubleCounted(t *testing.T) {
	cases := []struct {
		v1                         string
		total, translated, rewrite int
	}{
		// `this.s` -> `input?.s` is one Rewritten; `.slice` is the second.
		{`root = this.s.slice(0, 3)`, 7, 5, 2},
		{`root = this.xs.reverse()`, 5, 3, 2},
	}
	for _, c := range cases {
		rep := mustMigrate(t, c.v1)
		cov := rep.Coverage
		if cov.Total != c.total || cov.Translated != c.translated || cov.Rewritten != c.rewrite {
			t.Errorf("%q coverage = {Total:%d Translated:%d Rewritten:%d}, want {%d %d %d} — a Total/Translated bump of 1 means the recordedByRewrite guard regressed (Rewritten node also counted Exact)",
				c.v1, cov.Total, cov.Translated, cov.Rewritten, c.total, c.translated, c.rewrite)
		}
		if cov.Total != cov.Translated+cov.Rewritten+cov.Unsupported {
			t.Errorf("%q counter invariant broken: Total %d != T+R+U %d", c.v1, cov.Total, cov.Translated+cov.Rewritten+cov.Unsupported)
		}
	}
}

// TestContainsNotFlagged guards that .contains() is passed through 1:1 with no
// Warning/Error Change: V2 numeric equality uses promotion (1 == 1.0), so there
// is no divergence to flag. Prevents re-adding the (factually wrong) type-strict
// warning that was removed.
func TestContainsNotFlagged(t *testing.T) {
	for _, v1 := range []string{
		`root = this.xs.contains("b")`,
		`root = this.xs.contains(1)`,
		`root = "hello".contains("ell")`,
	} {
		rep := mustMigrate(t, v1)
		for _, ch := range rep.Changes {
			if ch.Severity == translator.SeverityWarning || ch.Severity == translator.SeverityError {
				t.Errorf("%q: unexpected %v Change on .contains: %s", v1, ch.Severity, ch.Explanation)
			}
		}
	}
}

// TestNoV2EquivalentIsUnsupported guards that functions with no V2 equivalent
// are recorded Unsupported (dragging coverage down), not passed through as a
// Note+Exact that reports a perfect ratio for V2 that will not compile.
func TestNoV2EquivalentIsUnsupported(t *testing.T) {
	interp := &bloblang2.Interp{}
	for _, v1 := range []string{
		`root = error_source_label()`,
		`root = error_source_name()`,
		`root = error_source_path()`,
		`root = json("foo")`,
	} {
		rep := mustMigrate(t, v1)
		if rep.Coverage.Unsupported == 0 {
			t.Errorf("%q: expected an Unsupported count, got coverage %+v", v1, rep.Coverage)
		}
		if !hasRule(rep.Changes, translator.RuleUnsupportedConstruct) {
			t.Errorf("%q: expected RuleUnsupportedConstruct; got:\n%s", v1, changeList(rep.Changes))
		}
		if rep.Coverage.Ratio >= 1.0 {
			t.Errorf("%q: ratio %.3f should be < 1.0 (V2 does not compile)", v1, rep.Coverage.Ratio)
		}
		// The emitted V2 genuinely does not compile — that is the point.
		if _, err := interp.Compile(rep.V2Mapping, nil); err == nil {
			t.Errorf("%q: expected emitted V2 not to compile (no V2 equivalent), but it did: %q", v1, rep.V2Mapping)
		}
	}
}
