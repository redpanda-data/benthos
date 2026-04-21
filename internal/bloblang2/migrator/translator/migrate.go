package translator

import (
	"fmt"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

// Migrate translates a V1 Bloblang mapping into V2.
//
// On success it returns a *Report containing:
//   - the V2 mapping text,
//   - a slice of Change records describing any semantic divergences,
//   - a Coverage summary of how much of the input was successfully translated.
//
// Migrate returns an error only when the weighted coverage ratio falls below
// opts.MinCoverage (default 0.75). The returned error is *CoverageError and
// still carries the best-effort Report via its Report field.
//
// A zero Options value behaves like DefaultOptions().
func Migrate(v1Source string, opts Options) (*Report, error) {
	opts = applyDefaults(opts)

	// 1. Translate imported files first so the main source's sanity check
	// can resolve them as V2.
	v2Files, err := translateFiles(opts.Files, opts)
	if err != nil {
		return nil, fmt.Errorf("migrator: translating imported file: %w", err)
	}

	// 2. Translate the main V1 source against the V2 import map.
	rep, err := migrateSource(v1Source, opts, v2Files)
	if err != nil {
		return nil, err
	}
	rep.V2Files = v2Files

	// 3. Coverage gate.
	if cerr := checkCoverage(rep, opts.MinCoverage); cerr != nil {
		return nil, cerr
	}
	return rep, nil
}

// migrateSource is the core V1→V2 translation step for a single source string.
// It doesn't touch opts.Files — v2Files is supplied by the caller as a fully
// translated V2 import map. This keeps the recursive translateFiles loop from
// re-invoking Migrate (which would re-enter translateFiles and loop).
//
// The post-translation sanity-check Parse is non-fatal: if it fails, we record
// an Unsupported Change tagged RuleEmittedInvalidV2 and still return the
// Report with the emitted text. Most V1-invalid inputs (chained comparisons,
// missing imports, duplicate namespaces) echo as V2 parse errors here — they
// are not translator bugs but honest V2 rejections of V1-invalid input, and
// should flow through to the caller's Compile for classification.
func migrateSource(v1Source string, opts Options, v2Files map[string]string) (*Report, error) {
	if v1Source == "" {
		return newRecorder(opts).finalise(""), nil
	}

	prog, err := v1ast.Parse(v1Source)
	if err != nil {
		return nil, fmt.Errorf("migrator: parsing V1 source: %w", err)
	}

	tr := &translator{rec: newRecorder(opts), files: opts.Files}
	v2Prog := tr.translateProgram(prog)
	v2Source := syntax.Print(v2Prog)

	if _, errs := syntax.Parse(v2Source, "", v2Files); len(errs) > 0 {
		tr.rec.Note(Change{
			Line: 1, Column: 1,
			Severity:    SeverityError,
			Category:    CategoryUnsupported,
			RuleID:      RuleEmittedInvalidV2,
			Explanation: fmt.Sprintf("emitted V2 failed to parse: %v", errs),
		})
	}

	return tr.rec.finalise(v2Source), nil
}

// translateFiles migrates each file in the Files map from V1 to V2 source,
// so V2's Parse sees V2 content wherever it resolves an import.
//
// The outer loop is a fixpoint: files whose imports are all already
// translated complete first, their results feed the next round, and so on.
// This handles nested import chains (A imports B imports C) without
// recursing into Migrate. After the fixpoint settles, any files still
// pending (cycles, or files with unresolvable imports) get one final pass
// with all siblings visible — remaining errors are fatal.
func translateFiles(in map[string]string, outerOpts Options) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	innerOpts := outerOpts
	innerOpts.MinCoverage = 0

	pending := make(map[string]string, len(in))
	for k, v := range in {
		pending[k] = v
	}
	out := make(map[string]string, len(in))

	for {
		progress := false
		for path, src := range pending {
			rep, err := migrateSource(src, innerOpts, out)
			if err != nil || hasUnresolvedImport(rep) {
				continue
			}
			out[path] = rep.V2Mapping
			delete(pending, path)
			progress = true
		}
		if !progress || len(pending) == 0 {
			break
		}
	}
	// Leftovers (cycles, or files with genuinely missing imports): one last
	// pass with all translated siblings visible. Any remaining error is
	// still fatal.
	for path, src := range pending {
		rep, err := migrateSource(src, innerOpts, out)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out[path] = rep.V2Mapping
	}
	return out, nil
}

// hasUnresolvedImport reports whether the report signals an emitted-invalid-V2
// change, which during the fixpoint most likely means a sibling file hasn't
// been translated yet. The caller retries next round.
func hasUnresolvedImport(rep *Report) bool {
	for _, c := range rep.Changes {
		if c.RuleID == RuleEmittedInvalidV2 {
			return true
		}
	}
	return false
}

// applyDefaults fills in zero-valued options with DefaultOptions().
func applyDefaults(opts Options) Options {
	if opts.MinCoverage == 0 {
		opts.MinCoverage = 0.75
	}
	return opts
}

// checkCoverage returns a *CoverageError when the report's Ratio is below
// opts.MinCoverage. Returns nil otherwise.
func checkCoverage(rep *Report, minCoverage float64) error {
	if rep.Coverage.Ratio >= minCoverage {
		return nil
	}
	return &CoverageError{
		Coverage: rep.Coverage,
		Min:      minCoverage,
		Report:   rep,
	}
}
