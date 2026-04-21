package translator_test

import (
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestNeverPanics — Layer 5 (property). Feeding the translator arbitrary
// strings (including malformed V1) must always either return a Report or an
// error, never panic. This is the single most important robustness property.
func TestNeverPanics(t *testing.T) {
	inputs := []string{
		"",
		" ",
		"\n",
		"\x00",
		"root",
		"root =",
		"root = ",
		"root = \"unterminated",
		"root = ???",
		"{}",
		strings.Repeat("root = this\n", 1000),
		"let x = 1\n$x = 2", // invalid V1 (var reassignment)
		"root.a =1",         // invalid V1 (no whitespace around =)
		"!!true",            // invalid V1 (double-not)
		"this[0]",           // invalid V1 (bracket indexing)
		"root = 5 / 0",      // V1 compile-time divide-by-zero
		`root = {5: "x"}`,   // V1 compile-time invalid key
	}

	for _, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Migrate(%q) panicked: %v", in, r)
				}
			}()
			_, _ = translator.Migrate(in, translator.Options{})
		}()
	}
}

// TestV2OutputAlwaysParses — Layer 5 (property). For every Migrate call that
// returns a Report (rather than an error), the V2 text must parse cleanly
// through syntax.Parse. Migrate itself enforces this internally, but a test
// double-checks the contract and guards against regressions.
func TestV2OutputAlwaysParses(t *testing.T) {
	inputs := []string{
		`root = this`,
		`root = 1 + 2`,
		`root.foo = this.bar`,
		`root = [1, 2, 3]`,
		`root = {"a": 1}`,
		`root = if true { "y" } else { "n" }`,
		`let x = 5
root = $x`,
		`meta key = "v"
root.data = this`,
		`root = this.x.or("default")`,
		`root = [1,2,3].map_each(x -> x + 1)`,
	}
	for _, in := range inputs {
		rep, err := translator.Migrate(in, translator.Options{})
		if err != nil {
			t.Logf("skip non-parsing input: %q -> %v", in, err)
			continue
		}
		if _, errs := syntax.Parse(rep.V2Mapping, "", nil); len(errs) > 0 {
			t.Errorf("Migrate produced invalid V2 for %q:\nV2:\n%s\nerrors: %v", in, rep.V2Mapping, errs)
		}
	}
}

// TestCoverageAlwaysComputed — Coverage.Ratio must be populated and
// in [0, 1] for every successful Migrate.
func TestCoverageAlwaysComputed(t *testing.T) {
	inputs := []string{
		``,
		`root = this`,
		`root.foo = 1`,
		`let x = 1
root = $x`,
	}
	for _, in := range inputs {
		rep, err := translator.Migrate(in, translator.Options{})
		if err != nil {
			continue
		}
		if rep.Coverage.Ratio < 0 || rep.Coverage.Ratio > 1 {
			t.Errorf("Coverage.Ratio out of [0,1] for %q: %v", in, rep.Coverage.Ratio)
		}
	}
}

// TestReportWellFormed — every Change record must carry non-empty Rule / non-
// zero position / non-empty Explanation. Catches rules that forget to set
// fields.
func TestReportWellFormed(t *testing.T) {
	sources := []string{
		`root = this`,
		`root = foo`,
		`meta k = this.x | "fb"`,
		`map foo { root = this * 2 }
root.v = (5).apply("foo")`,
	}
	for _, src := range sources {
		rep, err := translator.Migrate(src, translator.Options{Verbose: true})
		if err != nil {
			continue
		}
		for i, c := range rep.Changes {
			if c.RuleID == translator.RuleUnknown {
				t.Errorf("src=%q change[%d] has RuleUnknown", src, i)
			}
			if c.Line <= 0 {
				t.Errorf("src=%q change[%d] has non-positive line %d", src, i, c.Line)
			}
			if c.Explanation == "" {
				t.Errorf("src=%q change[%d] has empty Explanation", src, i)
			}
		}
	}
}
