// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// TestReportConversionFidelity guards the public/internal boundary: the public
// Report/Change/Coverage/CoverageError are distinct wrapper types converted
// from the internal translator types, so the conversion must carry every field
// through. A dropped field here would surface as a zero value.
func TestReportConversionFidelity(t *testing.T) {
	// `this.a / this.b` emits RuleIntDivReturnsFloat (Info + SemanticChange).
	rep, err := migrator.Migrate(context.Background(), `root = this.a / this.b`, migrator.Options{MinCoverage: 0.0001})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if rep.V2Mapping == "" {
		t.Error("V2Mapping not carried through conversion")
	}
	if rep.Coverage.Total == 0 {
		t.Error("Coverage.Total is zero — coverage not carried through")
	}
	var found *migrator.Change
	for i := range rep.Changes {
		if rep.Changes[i].RuleID == migrator.RuleIntDivReturnsFloat {
			found = &rep.Changes[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected a Change with RuleID RuleIntDivReturnsFloat")
	}
	if found.Severity != migrator.SeverityInfo {
		t.Errorf("Severity not carried: got %v", found.Severity)
	}
	if found.Category != migrator.CategorySemanticChange {
		t.Errorf("Category not carried: got %v", found.Category)
	}
	if found.Explanation == "" {
		t.Error("Explanation not carried through conversion")
	}
	if found.Line == 0 {
		t.Error("Line position not carried through conversion")
	}
	// Column and SpecRef are separate wrapper fields a naive conversion could
	// silently drop (they'd read as 0 / "" and every other assertion would
	// still pass). The div rule populates both: the `/` sits past column 0 and
	// carries SpecRef "§14#5".
	if found.Column == 0 {
		t.Error("Column not carried through conversion")
	}
	if found.SpecRef != "§14#5" {
		t.Errorf("SpecRef not carried through conversion: got %q, want %q", found.SpecRef, "§14#5")
	}

	// CoverageError path: an impossible threshold must yield a *CoverageError
	// whose (converted) Report is reachable.
	_, err = migrator.Migrate(context.Background(), `from "x"`, migrator.Options{MinCoverage: 1.0})
	var cerr *migrator.CoverageError
	if !errors.As(err, &cerr) {
		t.Fatalf("expected *CoverageError, got %T (%v)", err, err)
	}
	if cerr.Report == nil {
		t.Error("CoverageError.Report not carried through conversion")
	}
	if cerr.Min != 1.0 {
		t.Errorf("CoverageError.Min not carried: got %v", cerr.Min)
	}

	// A CoverageError's Report must carry its converted Changes, not just be
	// non-nil. error_source_label() has no V2 equivalent → an Unsupported
	// change that also drives the ratio below 1.0.
	_, err = migrator.Migrate(context.Background(), `root = error_source_label()`, migrator.Options{MinCoverage: 1.0})
	var cerr2 *migrator.CoverageError
	if !errors.As(err, &cerr2) {
		t.Fatalf("expected *CoverageError for a no-V2-equivalent function, got %T (%v)", err, err)
	}
	if cerr2.Report == nil || len(cerr2.Report.Changes) == 0 {
		t.Fatal("CoverageError.Report.Changes not carried through conversion")
	}
	var unsup *migrator.Change
	for i := range cerr2.Report.Changes {
		if cerr2.Report.Changes[i].RuleID == migrator.RuleUnsupportedConstruct {
			unsup = &cerr2.Report.Changes[i]
			break
		}
	}
	if unsup == nil {
		t.Fatal("expected an Unsupported change in the CoverageError report")
	}
	if unsup.Severity != migrator.SeverityError || unsup.Category != migrator.CategoryUnsupported {
		t.Errorf("Unsupported change fields not carried: severity=%v category=%v", unsup.Severity, unsup.Category)
	}
	if cerr2.Report.Coverage.Unsupported == 0 {
		t.Error("CoverageError.Report.Coverage.Unsupported not carried")
	}
}
