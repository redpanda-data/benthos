package translator_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestMapEachBareIfKeepsElement is the regression for the map_each
// else-less-`if` mis-rewrite. V1 keeps the original element when the condition
// is false (nothing sentinel); V2 must get an explicit `else { <element> }`
// rather than void (which errors) or `deleted()` (which drops the element).
func TestMapEachBareIfKeepsElement(t *testing.T) {
	mig := func(v1 string) *translator.Report {
		t.Helper()
		rep, err := translator.Migrate(context.Background(), v1, translator.Options{MinCoverage: 0.0001})
		var c *translator.CoverageError
		switch {
		case err == nil:
		case errors.As(err, &c):
			rep = c.Report
		default:
			t.Fatalf("Migrate(%q): %v", v1, err)
		}
		return rep
	}

	// Main case: else { x } synthesized, not deleted(), not bare void.
	rep := mig(`root = this.xs.map_each(x -> if x > 1 { x * 10 })`)
	if !strings.Contains(rep.V2Mapping, "else") {
		t.Errorf("expected synthesized else; got:\n%s", rep.V2Mapping)
	}
	if strings.Contains(rep.V2Mapping, "deleted()") {
		t.Errorf("map_each keep-element must not synthesize deleted(); got:\n%s", rep.V2Mapping)
	}

	// Inverse bug: a map_each nested inside an array literal must NOT let the
	// outer ctxCollectionLit leak into the lambda body and synthesize
	// deleted(); it must still keep the element.
	rep2 := mig(`root = [this.xs.map_each(x -> if x > 1 { x * 10 })]`)
	if strings.Contains(rep2.V2Mapping, "deleted()") {
		t.Errorf("inverse bug: collection-lit ctx leaked into lambda body -> deleted(); got:\n%s", rep2.V2Mapping)
	}
	if !strings.Contains(rep2.V2Mapping, "else") {
		t.Errorf("expected else{x} even inside an array literal; got:\n%s", rep2.V2Mapping)
	}
}
