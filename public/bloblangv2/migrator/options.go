// Copyright 2026 Redpanda Data, Inc.

package migrator

// Options controls a single Migrate call. Per-instance configuration
// (registered rules) lives on the Migrator; per-call configuration
// (verbosity, coverage threshold, mode) lives here.
type Options struct {
	// MinCoverage is the minimum Coverage.Ratio required before
	// Migrate returns successfully. If the computed ratio falls below
	// this value Migrate returns *CoverageError. Default 0.75.
	MinCoverage float64

	// Verbose emits Info-severity Changes. Without it, only Warning
	// and Error Changes are recorded.
	Verbose bool

	// TreatWarningsAsErrors promotes Warning-severity Changes to
	// Error. Useful for CI gates.
	TreatWarningsAsErrors bool

	// Files is a virtual filesystem for `import` resolution, keyed by
	// the path used in the V1 source.
	Files map[string]string

	// Mode selects how the V1 mapping's implicit root is treated.
	// Default (zero value) is ModeMutation — V2's `output` semantics
	// align with V1's `mutation` processor. Use ModeMapping when
	// translating mappings authored for V1's `mapping` processor (the
	// translator prepends `output = input`).
	Mode Mode
}
