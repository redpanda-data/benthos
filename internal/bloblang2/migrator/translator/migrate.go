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

	// 1. Walk the closure of imports rooted at the main source. Both
	// pre-populated Files and the FileResolver feed into the resulting
	// fileSet; canonical keys serve as the identity for dedup and for
	// Report.V2Files emission.
	fs, err := buildFileSet(v1Source, opts)
	if err != nil {
		return nil, fmt.Errorf("migrator: walking import closure: %w", err)
	}

	// 2. Translate every file in the closure (except the main source)
	// from V1 to V2 so the main source's V2 sanity-check Parse can
	// resolve its imports. v2Contents is canonical-keyed.
	v2Contents, err := translateFiles(fs, opts)
	if err != nil {
		return nil, fmt.Errorf("migrator: translating imported file: %w", err)
	}

	// 3. Project canonical-keyed V2 contents to V2-path-keyed contents
	// for the sanity-check Parse, then translate the main V1 source.
	parseFiles := projectToV2Paths(fs, v2Contents, opts.V2ImportPathRewriter)
	rep, err := migrateSource(v1Source, "", opts, fs, parseFiles)
	if err != nil {
		return nil, err
	}
	rep.V2Files = v2Contents

	// 4. Coverage gate.
	if cerr := checkCoverage(rep, opts.MinCoverage); cerr != nil {
		return nil, cerr
	}
	return rep, nil
}

// migrateSource is the core V1→V2 translation step for a single source string.
// parentKey is the canonical key of the file being translated, or empty for
// the main source. fs is the closure built upstream; v2Files is the
// already-translated V2 import map (keyed by the V2 path string emitted into
// the V2 source after V2ImportPathRewriter is applied) used by the
// post-translation sanity-check Parse.
//
// The post-translation sanity-check Parse is non-fatal: if it fails, we record
// an Unsupported Change tagged RuleEmittedInvalidV2 and still return the
// Report with the emitted text. Most V1-invalid inputs (chained comparisons,
// missing imports, duplicate namespaces) echo as V2 parse errors here — they
// are not translator bugs but honest V2 rejections of V1-invalid input, and
// should flow through to the caller's Compile for classification.
func migrateSource(v1Source, parentKey string, opts Options, fs *fileSet, v2Files map[string]string) (*Report, error) {
	if v1Source == "" {
		return newRecorder(opts).finalise(""), nil
	}

	prog, err := v1ast.Parse(v1Source)
	if err != nil {
		return nil, fmt.Errorf("migrator: parsing V1 source: %w", err)
	}

	tr := &translator{
		rec:                  newRecorder(opts),
		parentKey:            parentKey,
		fileSet:              fs,
		v2ImportPathRewriter: opts.V2ImportPathRewriter,
		customMethodRules:    opts.CustomMethodRules,
		customFunctionRules:  opts.CustomFunctionRules,
	}
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

// translateFiles migrates every file in fs.contents (excluding the
// main source — which migrateSource translates separately) from V1 to
// V2. The returned map is canonical-keyed: it's the shape we surface
// to callers via Report.V2Files.
//
// The outer loop is a fixpoint: files whose imports are all already
// translated complete first, their results feed the next round, and so on.
// This handles nested import chains (A imports B imports C) without
// recursing into Migrate. After the fixpoint settles, any files still
// pending (cycles, or files with unresolvable imports) get one final pass
// with all siblings visible — remaining errors are fatal.
func translateFiles(fs *fileSet, outerOpts Options) (map[string]string, error) {
	if len(fs.contents) == 0 {
		return nil, nil
	}
	innerOpts := outerOpts
	innerOpts.MinCoverage = 0

	// pending is keyed by canonical key.
	pending := make(map[string]string, len(fs.contents))
	for k, v := range fs.contents {
		pending[k] = v
	}
	// out is keyed by canonical key.
	out := make(map[string]string, len(fs.contents))

	for {
		progress := false
		for canonical, src := range pending {
			parseFiles := projectToV2Paths(fs, out, outerOpts.V2ImportPathRewriter)
			rep, err := migrateSource(src, canonical, innerOpts, fs, parseFiles)
			if err != nil || hasUnresolvedImport(rep) {
				continue
			}
			out[canonical] = rep.V2Mapping
			delete(pending, canonical)
			progress = true
		}
		if !progress || len(pending) == 0 {
			break
		}
	}
	// Leftovers (cycles, or files with genuinely missing imports): one last
	// pass with all translated siblings visible. Any remaining error is
	// still fatal.
	for canonical, src := range pending {
		parseFiles := projectToV2Paths(fs, out, outerOpts.V2ImportPathRewriter)
		rep, err := migrateSource(src, canonical, innerOpts, fs, parseFiles)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", canonical, err)
		}
		out[canonical] = rep.V2Mapping
	}

	return out, nil
}

// projectToV2Paths maps canonical-keyed V2 contents into a map keyed by
// the V2 path strings that appear in V2 import statements (i.e. after
// V2ImportPathRewriter is applied). When no rewriter is set canonical
// keys equal V1 path strings equal V2 path strings, so the map is
// returned with canonical keys unchanged.
func projectToV2Paths(fs *fileSet, v2Contents map[string]string, rewriter V2ImportPathRewriter) map[string]string {
	if rewriter == nil {
		// Fast path: the V2 import statements in the emitted source carry
		// the same path strings as the V1 source, and canonical keys
		// equal those path strings whenever no FileResolver is in use.
		// For FileResolver-driven flows the canonical keys are whatever
		// the resolver returned; the V2 import statements still carry
		// the original V1 path strings (no rewriter), so we project via
		// siteIndex.
		out := make(map[string]string, len(v2Contents))
		for canonical, content := range v2Contents {
			out[canonical] = content
		}
		for site, canonical := range fs.siteIndex {
			if c, ok := v2Contents[canonical]; ok {
				out[site.importPath] = c
			}
		}
		return out
	}
	out := make(map[string]string, len(v2Contents))
	for canonical, content := range v2Contents {
		out[rewriter(canonical)] = content
	}
	for site, canonical := range fs.siteIndex {
		if c, ok := v2Contents[canonical]; ok {
			out[rewriter(site.importPath)] = c
		}
	}
	return out
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
