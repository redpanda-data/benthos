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

// builtInRules returns the rules registered automatically on every
// Migrator constructed via New. They migrate the three V1 mapping
// processors to bloblang_v2, threading each mapping body through the
// bundled Bloblang V1->V2 translator.
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
func bloblangProcessorRule(mode bloblmig.Mode) Rule {
	return func(ctx *Context, c *Component) Result {
		body, ok := c.BodyString()
		if !ok {
			return ctx.Unsupported("expected scalar string body for " + c.Name + " processor")
		}
		v2Source, rep, err := ctx.MigrateBloblang(body, mode)
		if err != nil {
			return ctx.Unsupported(err.Error())
		}
		return ctx.ReplaceWithBloblangReport(targetBloblangV2, v2Source, rep)
	}
}
