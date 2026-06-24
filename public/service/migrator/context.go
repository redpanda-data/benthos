// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	bloblmig "github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// Context is the helper handle a Rule receives. It exposes the
// migrator's bundled Bloblang V1->V2 translator (so rules can rewrite
// embedded mapping bodies the same way the built-in rules do) plus
// the Result constructors.
type Context struct {
	bloblang     *bloblmig.Migrator
	bloblangOpts bloblmig.Options
}

// Bloblang returns the Bloblang V1->V2 migrator wired into this
// Migrator (either the default or the one supplied via
// Options.BloblangMigrator). Use it from a custom rule when the
// plugin's config contains a Bloblang V1 mapping that should be
// translated to V2 alongside the plugin rename.
func (c *Context) Bloblang() *bloblmig.Migrator {
	return c.bloblang
}

// MigrateBloblang is a convenience that runs the bundled Bloblang
// V1->V2 migrator with the supplied mode and the per-call options
// configured on this Migrator (Verbose, MinCoverage, ...). It returns
// the V2 source and the Bloblang report; the report is attached to
// the outer Change when the result is used in a Replace.
func (c *Context) MigrateBloblang(v1Source string, mode bloblmig.Mode) (string, *bloblmig.Report, error) {
	opts := c.bloblangOpts
	opts.Mode = mode
	rep, err := c.bloblang.Migrate(v1Source, opts)
	if err != nil {
		return "", nil, err
	}
	return rep.V2Mapping, rep, nil
}

// Replace returns a Result that swaps the matched plugin for a new
// one. newName is the plugin name to register under (e.g.
// "bloblang_v2") and newBody is the scalar body for the new plugin.
//
// For structured replacements (where the new plugin's config is not a
// scalar string), use ReplaceStructured.
func (c *Context) Replace(newName, newBody string) Result {
	return Result{
		kind:        resultReplace,
		replacement: replacement{name: newName, body: newBody},
	}
}

// ReplaceWithBloblangReport is the same as Replace but additionally
// attaches a Bloblang V1->V2 *Report. The outer Migrator surfaces it
// on the per-component Change record.
func (c *Context) ReplaceWithBloblangReport(newName, newBody string, report *bloblmig.Report) Result {
	return Result{
		kind: resultReplace,
		replacement: replacement{
			name:           newName,
			body:           newBody,
			bloblangReport: report,
		},
	}
}

// ReplaceStructured returns a Result that swaps the matched plugin
// for a new one whose body is a structured Go value (encoded to YAML
// when the migration is rendered). Use Replace for the common case of
// a scalar string body.
func (c *Context) ReplaceStructured(newName string, body any) Result {
	return Result{
		kind:        resultReplace,
		replacement: replacement{name: newName, body: body},
	}
}

// Skip declines to migrate the matched plugin and falls through to
// the next rule (or leaves the component untouched if no other rule
// matches). reason, if non-empty, is recorded as an Info-severity
// note on the Report.
func (c *Context) Skip(reason string) Result {
	return Result{kind: resultSkip, reason: reason}
}

// Unsupported declares that the matched plugin cannot be migrated and
// records an Error-severity Change. The component is left untouched
// so the rewritten YAML remains parseable; the user is alerted via
// the Report.
func (c *Context) Unsupported(reason string) Result {
	return Result{kind: resultUnsupported, reason: reason}
}
