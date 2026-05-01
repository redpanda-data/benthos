// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	bloblmig "github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// Options controls a single Migrate call. Per-instance configuration
// (registered rules) lives on the Migrator; per-call configuration
// (verbosity, coverage threshold, embedded-bloblang migrator) lives
// here.
type Options struct {
	// BloblangMigrator is the Bloblang V1->V2 migrator the built-in
	// processor rules (and any custom rules that consult
	// Context.Bloblang) thread embedded mapping bodies through. If nil
	// a fresh migrator with built-in rules only is used. Supply a
	// custom one to register plugin-specific Bloblang method/function
	// rules ahead of the call.
	BloblangMigrator *bloblmig.Migrator

	// BloblangOptions is forwarded to the Bloblang V1->V2 migrator on
	// each call. The Mode field is overridden per built-in rule
	// (ModeMapping for `bloblang`/`mapping`, ModeMutation for
	// `mutation`); other fields (Verbose, MinCoverage, Files,
	// TreatWarningsAsErrors) pass through unchanged.
	BloblangOptions bloblmig.Options

	// MinCoverage is the minimum aggregate coverage ratio required
	// across all migrated plugin instances before Migrate returns
	// successfully. The ratio is computed as (Rewritten) /
	// (Rewritten + Unsupported); plugins skipped or untouched do not
	// affect it. Default 0 (no gate).
	MinCoverage float64

	// Verbose emits Info-severity Changes (e.g. Skip notes). Without
	// it, only Warning and Error Changes are recorded.
	Verbose bool
}
