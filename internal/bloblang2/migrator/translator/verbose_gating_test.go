package translator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestNonVerboseKeepsSemanticChanges is the regression for the migrator
// dropping real divergence in default (non-verbose) reports. emit previously
// suppressed ALL SeverityInfo Changes unless Verbose, which hid genuine
// semantic-change notes (arithmetic/comparison/byte-offset shifts) recorded at
// Info. The gate must suppress only benign idiom rewrites (identical
// semantics); semantic changes must always surface.
func TestNonVerboseKeepsSemanticChanges(t *testing.T) {
	const v1 = `root = this.a / this.b`

	migrate := func(t *testing.T, opts translator.Options) *translator.Report {
		t.Helper()
		opts.MinCoverage = 0.0001 // bypass the 0.75 default-fallback
		rep, err := translator.Migrate(context.Background(), v1, opts)
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

	// Default (non-verbose): the int-division result-type shift is
	// Info+SemanticChange — it MUST be surfaced.
	rep := migrate(t, translator.Options{})
	if !hasRule(rep.Changes, translator.RuleIntDivReturnsFloat) {
		t.Errorf("non-verbose report dropped the int-division semantic-change note (RuleIntDivReturnsFloat); got:\n%s", changeList(rep.Changes))
	}
	// ...but the benign this->input idiom rewrite (Info+IdiomRewrite) stays
	// suppressed unless verbose.
	if hasRule(rep.Changes, translator.RuleThisToInput) {
		t.Errorf("non-verbose report should suppress the benign this->input idiom rewrite; got:\n%s", changeList(rep.Changes))
	}

	// Verbose surfaces the idiom rewrite as well.
	repV := migrate(t, translator.Options{Verbose: true})
	if !hasRule(repV.Changes, translator.RuleThisToInput) {
		t.Errorf("verbose report should include the this->input idiom rewrite; got:\n%s", changeList(repV.Changes))
	}
}
