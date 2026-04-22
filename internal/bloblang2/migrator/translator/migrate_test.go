package translator

import (
	"errors"
	"strings"
	"testing"
)

func TestMigrateEmptyInput(t *testing.T) {
	rep, err := Migrate("", Options{})
	if err != nil {
		t.Fatalf("empty input should succeed, got %v", err)
	}
	if rep.V2Mapping != "" {
		t.Fatalf("empty V2 expected, got %q", rep.V2Mapping)
	}
	if rep.Coverage.Ratio != 1.0 {
		t.Fatalf("empty coverage should be 1.0, got %v", rep.Coverage.Ratio)
	}
}

func TestMigrateSimpleRootToOutput(t *testing.T) {
	rep, err := Migrate("root = this", Options{Verbose: true})
	if err != nil {
		t.Fatalf("simple root->output should succeed: %v", err)
	}
	if rep.V2Mapping == "" {
		t.Fatalf("expected non-empty V2 output")
	}
	// Should contain "output" and "input" since root/this are rewritten.
	if !strings.Contains(rep.V2Mapping, "output") || !strings.Contains(rep.V2Mapping, "input") {
		t.Fatalf("expected output/input in V2 text, got:\n%s", rep.V2Mapping)
	}
	if rep.Coverage.Ratio < 0.9 {
		t.Fatalf("simple translation should be near-perfect coverage, got %v", rep.Coverage.Ratio)
	}
}

func TestMigrateArithmetic(t *testing.T) {
	rep, err := Migrate("root = 1 + 2 * 3", Options{})
	if err != nil {
		t.Fatalf("arithmetic translation should succeed: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, "1") || !strings.Contains(rep.V2Mapping, "+") {
		t.Fatalf("expected arithmetic preserved, got:\n%s", rep.V2Mapping)
	}
}

func TestCheckCoverageBelowThreshold(t *testing.T) {
	rep := &Report{Coverage: Coverage{Total: 10, Translated: 5, Unsupported: 5, Ratio: 0.5}}
	err := checkCoverage(rep, 0.75)
	var ce *CoverageError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *CoverageError, got %T: %v", err, err)
	}
	if ce.Report != rep {
		t.Fatalf("expected CoverageError to carry the Report")
	}
	if ce.Min != 0.75 {
		t.Fatalf("expected Min 0.75, got %v", ce.Min)
	}
}

func TestCheckCoverageMetsThreshold(t *testing.T) {
	rep := &Report{Coverage: Coverage{Total: 10, Translated: 8, Rewritten: 2, Ratio: 0.98}}
	if err := checkCoverage(rep, 0.75); err != nil {
		t.Fatalf("above threshold should not error, got %v", err)
	}
}

func TestApplyDefaults(t *testing.T) {
	opts := applyDefaults(Options{})
	if opts.MinCoverage != 0.75 {
		t.Fatalf("default MinCoverage should be 0.75, got %v", opts.MinCoverage)
	}
	if opts.Mode != ModeMutation {
		t.Fatalf("default Mode should be ModeMutation, got %v", opts.Mode)
	}
}

func TestMigrateModeMutationNoPrelude(t *testing.T) {
	// Default mode: no `output = input` prelude.
	rep, err := Migrate("root.v = 1", Options{Mode: ModeMutation})
	if err != nil {
		t.Fatalf("mutation-mode translation should succeed: %v", err)
	}
	if strings.Contains(rep.V2Mapping, "output = input") {
		t.Fatalf("mutation mode must not inject `output = input`; got:\n%s", rep.V2Mapping)
	}
}

func TestMigrateModeMappingInjectsPrelude(t *testing.T) {
	// mapping mode: translator prepends `output = input` so the V2
	// result starts as the input document (matching V1 mapping's
	// pass-through default).
	rep, err := Migrate("root.v = 1", Options{Mode: ModeMapping, Verbose: true})
	if err != nil {
		t.Fatalf("mapping-mode translation should succeed: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, "output = input") {
		t.Fatalf("mapping mode must inject `output = input`; got:\n%s", rep.V2Mapping)
	}
	// The prelude must be the *first* statement so subsequent field
	// assignments build on top of the passed-through input.
	idxPrelude := strings.Index(rep.V2Mapping, "output = input")
	idxBody := strings.Index(rep.V2Mapping, "output.v")
	if idxBody < idxPrelude {
		t.Fatalf("prelude must precede the translated body; got:\n%s", rep.V2Mapping)
	}
}
