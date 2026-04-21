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

	if v1Source == "" {
		return newRecorder(opts).finalise(""), nil
	}

	// 1. Parse V1 source into an AST.
	prog, err := v1ast.Parse(v1Source)
	if err != nil {
		return nil, fmt.Errorf("migrator: parsing V1 source: %w", err)
	}

	// 2. Walk the V1 AST, producing a V2 AST plus Changes and Coverage.
	tr := &translator{rec: newRecorder(opts)}
	v2Prog := tr.translateProgram(prog)

	// 3. Print the V2 AST.
	v2Source := syntax.Print(v2Prog)

	// 4. Sanity-check: the V2 output must compile. If it doesn't, that's a
	// translator bug — return a distinctive error rather than silently
	// producing broken V2.
	if _, errs := syntax.Parse(v2Source, "", nil); len(errs) > 0 {
		return nil, fmt.Errorf("migrator: emitted V2 failed to parse (internal bug): %v\n\nemitted:\n%s", errs, v2Source)
	}

	// 5. Finalise the report and check coverage.
	rep := tr.rec.finalise(v2Source)
	if cerr := checkCoverage(rep, opts.MinCoverage); cerr != nil {
		return nil, cerr
	}
	return rep, nil
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
