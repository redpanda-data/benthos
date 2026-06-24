// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	bloblmig "github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// targetBloblangV2 is the V2 processor name registered in
// internal/impl/pure/processor_bloblang_v2.go. It uses an underscore
// (matching Benthos's snake_case plugin naming convention) — not
// "bloblangv2".
const targetBloblangV2 = "bloblang_v2"

// targetBloblangV2File is the V2 file-backed processor name
// registered in internal/impl/pure/processor_bloblang_v2_file.go.
// V1 `from "path"` bodies migrate to this processor with the path
// rewritten via Options.BloblangV2ImportPathRewriter.
const targetBloblangV2File = "bloblang_v2_file"

// builtInRules returns the rules registered automatically on every
// Migrator constructed via New. They migrate the three V1 mapping
// processors to bloblang_v2 (or bloblang_v2_file when the body is a
// single `from "path"` statement), threading each mapping body
// through the bundled Bloblang V1->V2 translator.
func builtInRules() map[Target]Rule {
	return map[Target]Rule{
		{ComponentType: "processor", Name: "bloblang"}: bloblangProcessorRule(bloblmig.ModeMapping),
		{ComponentType: "processor", Name: "mapping"}:  bloblangProcessorRule(bloblmig.ModeMapping),
		{ComponentType: "processor", Name: "mutation"}: bloblangProcessorRule(bloblmig.ModeMutation),
	}
}

// bloblangProcessorRule is the shared rule body for the three V1
// mapping processors. The only per-processor variation is the
// translator Mode: `mapping` and `bloblang` use ModeMapping (the
// translator prepends `output = input` so unwritten fields pass
// through unchanged), while `mutation` uses ModeMutation (no prelude
// — V2's empty `output` matches V1's `mutation` semantics).
//
// Bodies that consist of a single `from "path"` statement are
// rewritten to bloblang_v2_file, preserving the user's file-factoring
// intent. The bloblang migrator still walks the file's V1 content
// (via the closure walker) so the referenced file gets translated to
// V2 and surfaced in Report.BloblangV2Files.
func bloblangProcessorRule(mode bloblmig.Mode) Rule {
	return func(ctx *Context, c *Component) Result {
		body, ok := c.BodyString()
		if !ok {
			return ctx.Unsupported("expected scalar string body for " + c.Name + " processor")
		}
		if v1Path, ok := bloblmig.IsFromOnly(body); ok {
			// Migrate the body through the bloblang migrator so the
			// closure walker pulls in the referenced file and emits
			// its V2 translation in Report.V2Files. The migrated body
			// itself is not used; we replace the processor with the
			// file-backed form pointing at the rewritten path.
			_, rep, err := ctx.MigrateBloblang(body, mode)
			if err != nil {
				return ctx.Unsupported(err.Error())
			}
			// Even when MigrateBloblang returns success, the report
			// can still carry a RuleFromStatement Error change when
			// the target file couldn't be resolved AND coverage
			// gating didn't escalate it (e.g. callers running with
			// MinCoverage=0). Without this guard the rule would emit
			// `bloblang_v2_file: <path>` pointing at a file that
			// won't exist in Report.BloblangV2Files — a silently
			// broken migrated config.
			if reportHasUnresolvedFrom(rep) {
				return ctx.Unsupported("from target " + v1Path + " could not be resolved")
			}
			v2Path := v1Path
			if rewriter := ctx.bloblangOpts.V2ImportPathRewriter; rewriter != nil {
				v2Path = rewriter(v1Path)
			}
			return ctx.ReplaceWithBloblangReport(targetBloblangV2File, v2Path, rep)
		}
		v2Source, rep, err := ctx.MigrateBloblang(body, mode)
		if err != nil {
			return ctx.Unsupported(err.Error())
		}
		return ctx.ReplaceWithBloblangReport(targetBloblangV2, v2Source, rep)
	}
}

// reportHasUnresolvedFrom reports whether rep contains an
// Error-severity RuleFromStatement change — the marker the bloblang
// migrator emits when a from-only target couldn't be resolved.
// Errors are never filtered by Verbose so this signal is reliable
// regardless of the caller's BloblangOptions.
func reportHasUnresolvedFrom(rep *bloblmig.Report) bool {
	if rep == nil {
		return false
	}
	for _, c := range rep.Changes {
		if c.RuleID == bloblmig.RuleFromStatement && c.Severity == bloblmig.SeverityError {
			return true
		}
	}
	return false
}
