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

	// Files is a virtual filesystem for `import` resolution. Keys are
	// treated as canonical identifiers for the imported files: an
	// entry keyed "helpers.blobl" satisfies any V1 import statement
	// whose path string equals "helpers.blobl". Pre-populated entries
	// take precedence over FileResolver.
	Files map[string]string

	// FileResolver, when set, lazily resolves V1 imports during
	// Migrate. The migrator walks the closure of imports starting
	// from the main source and any transitively imported files,
	// calling the resolver for each unique import path it encounters.
	//
	// parentKey is the canonical key of the file the import appears
	// in (empty for imports in the main V1 source). importPath is
	// the path string as written in the import statement. The
	// returned canonicalKey identifies the resolved file for
	// de-duplication and Report.V2Files emission — two import
	// statements that resolve to the same canonicalKey are
	// translated once.
	//
	// Returning ok=false records an Unsupported import at the import
	// site and continues with the rest of the migration.
	//
	// Pre-populated Files take precedence: if Files contains
	// importPath as a key, the resolver is not consulted and
	// importPath itself is treated as the canonical key.
	FileResolver FileResolver

	// V2ImportPathRewriter, when set, rewrites V1 import path strings
	// to their V2 equivalents in the emitted V2 source. Default:
	// identity. Useful for callers that emit V2-translated files at
	// sibling paths (e.g. "helpers.blobl" -> "helpers.v5.blobl").
	// Operates on the verbatim path string from the V1 source so
	// locality is preserved (relative imports stay relative).
	V2ImportPathRewriter V2ImportPathRewriter

	// Mode selects how the V1 mapping's implicit root is treated.
	// Default (zero value) is ModeMutation — V2's `output` semantics
	// align with V1's `mutation` processor. Use ModeMapping when
	// translating mappings authored for V1's `mapping` processor (the
	// translator prepends `output = input`).
	Mode Mode
}
