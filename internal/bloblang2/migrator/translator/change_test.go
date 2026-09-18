package translator

import (
	"strings"
	"testing"
)

func TestCoverageRatio(t *testing.T) {
	for _, c := range []struct {
		name      string
		coverage  Coverage
		wantRatio float64
	}{
		{"zero is 1.0", Coverage{}, 1.0},
		{"all exact", Coverage{Total: 10, Translated: 10}, 1.0},
		{"all rewritten", Coverage{Total: 10, Rewritten: 10}, 0.9},
		{"mixed", Coverage{Total: 10, Translated: 5, Rewritten: 5}, 0.95},
		{"unsupported hurts", Coverage{Total: 10, Translated: 5, Unsupported: 5}, 0.5},
		{"below threshold", Coverage{Total: 4, Translated: 1, Rewritten: 1, Unsupported: 2}, (1.0 + 0.9) / 4},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := computeRatio(c.coverage)
			if got != c.wantRatio {
				t.Fatalf("computeRatio got %v want %v", got, c.wantRatio)
			}
		})
	}
}

func TestRecorderEmitsByVerbosity(t *testing.T) {
	r := newRecorder(Options{Verbose: false})
	r.Exact()
	r.Rewritten(Change{Severity: SeverityInfo, RuleID: RuleRootToOutput})
	r.Rewritten(Change{Severity: SeverityWarning, RuleID: RuleOrCatchesErrors})
	rep := r.finalise("output = input")

	if got, want := rep.Coverage.Total, 3; got != want {
		t.Fatalf("Total got %d want %d", got, want)
	}
	if got, want := rep.Coverage.Translated, 1; got != want {
		t.Fatalf("Translated got %d want %d", got, want)
	}
	if got, want := rep.Coverage.Rewritten, 2; got != want {
		t.Fatalf("Rewritten got %d want %d", got, want)
	}
	// Verbose=false should suppress Info but keep Warning.
	if got, want := len(rep.Changes), 1; got != want {
		t.Fatalf("Changes got %d want %d (non-verbose should drop Info)", got, want)
	}
	if rep.Changes[0].Severity != SeverityWarning {
		t.Fatalf("expected Warning, got %v", rep.Changes[0].Severity)
	}
}

func TestRecorderVerboseIncludesInfo(t *testing.T) {
	r := newRecorder(Options{Verbose: true})
	r.Rewritten(Change{Severity: SeverityInfo, RuleID: RuleRootToOutput})
	r.Rewritten(Change{Severity: SeverityWarning, RuleID: RuleOrCatchesErrors})
	rep := r.finalise("")
	if got, want := len(rep.Changes), 2; got != want {
		t.Fatalf("verbose should record both; got %d want %d", got, want)
	}
}

func TestRecorderWarningsAsErrors(t *testing.T) {
	r := newRecorder(Options{Verbose: true, TreatWarningsAsErrors: true})
	r.Rewritten(Change{Severity: SeverityWarning, RuleID: RuleOrCatchesErrors})
	rep := r.finalise("")
	if got := rep.Changes[0].Severity; got != SeverityError {
		t.Fatalf("expected promotion to Error, got %v", got)
	}
}

func TestRecorderUnsupportedFixesFields(t *testing.T) {
	r := newRecorder(Options{Verbose: true})
	r.Unsupported(Change{Severity: SeverityInfo, Category: CategoryIdiomRewrite, RuleID: RuleMethodDoesNotExist})
	rep := r.finalise("")
	if rep.Changes[0].Severity != SeverityError {
		t.Fatalf("Unsupported should force Error")
	}
	if rep.Changes[0].Category != CategoryUnsupported {
		t.Fatalf("Unsupported should force CategoryUnsupported")
	}
	if rep.Coverage.Unsupported != 1 {
		t.Fatalf("Unsupported counter not incremented")
	}
}

func TestCoverageError(t *testing.T) {
	e := &CoverageError{
		Coverage: Coverage{Total: 10, Translated: 5, Unsupported: 5, Ratio: 0.5},
		Min:      0.75,
	}
	if !strings.Contains(e.Error(), "below threshold") {
		t.Fatalf("error message missing context: %q", e.Error())
	}
}

func TestRuleIDStringIsUnique(t *testing.T) {
	// Guard against accidentally giving two RuleIDs the same String.
	// This also documents the expected naming pattern.
	seen := map[string]RuleID{}
	for id := RuleUnknown; id <= RuleUnsupportedConstruct; id++ {
		s := id.String()
		if existing, ok := seen[s]; ok {
			t.Errorf("RuleID %d and %d both stringify to %q", existing, id, s)
		}
		seen[s] = id
	}
}
