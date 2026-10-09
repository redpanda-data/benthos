// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestChangeConversionCarriesEveryField is a white-box guard on the
// public<->internal Change conversion. Unlike wrapper_conversion_test.go (which
// only sees fields a real migration happens to populate), this sets EVERY field
// to a distinct non-zero value and checks both directions. EndLine/EndColumn/
// Original/Translated are not populated by any current rule, so only a direct
// unit test like this catches a conversion that silently drops them.
func TestChangeConversionCarriesEveryField(t *testing.T) {
	in := translator.Change{
		Line:        1,
		Column:      2,
		EndLine:     3,
		EndColumn:   4,
		Severity:    translator.SeverityWarning,
		Category:    translator.CategorySemanticChange,
		RuleID:      translator.RuleSliceByteVsCodepoint,
		SpecRef:     "§14#48",
		Original:    "v1.snippet()",
		Translated:  "v2.snippet()",
		Explanation: "explanation text",
	}

	pub := changeFrom(in)
	if pub.Line != 1 || pub.Column != 2 || pub.EndLine != 3 || pub.EndColumn != 4 {
		t.Errorf("changeFrom dropped a position field: %+v", pub)
	}
	if pub.Severity != Severity(translator.SeverityWarning) ||
		pub.Category != Category(translator.CategorySemanticChange) ||
		pub.RuleID != RuleID(translator.RuleSliceByteVsCodepoint) {
		t.Errorf("changeFrom dropped an enum field: %+v", pub)
	}
	if pub.SpecRef != "§14#48" || pub.Original != "v1.snippet()" ||
		pub.Translated != "v2.snippet()" || pub.Explanation != "explanation text" {
		t.Errorf("changeFrom dropped a string field: %+v", pub)
	}

	// Round-trip back to internal must reproduce the original exactly.
	if back := changeTo(pub); back != in {
		t.Errorf("changeTo(changeFrom(x)) != x:\n got: %+v\nwant: %+v", back, in)
	}
}
