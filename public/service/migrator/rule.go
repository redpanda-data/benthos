// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	bloblmig "github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// Target identifies the plugin a Rule applies to. ComponentType is the
// core component family (e.g. "processor", "input", "output", "cache",
// "buffer", "rate_limit", "metrics", "tracer", "scanner") and Name is
// the plugin name registered for that type (e.g. "bloblang",
// "mutation", "mapping").
type Target struct {
	ComponentType string
	Name          string
}

// Rule is the callback shape for a custom plugin migration. Rules are
// registered with Migrator.RegisterRule, keyed by Target. The callback
// receives a Context (helpers + Result constructors) and a Component
// describing the matched plugin instance, and returns a Result
// describing the outcome.
//
// Custom rules win on collision with the built-ins (the downstream
// rule fully replaces the built-in for that Target).
type Rule func(ctx *Context, c *Component) Result

// resultKind is the discriminant for Result.
type resultKind int

const (
	resultUnset resultKind = iota
	resultReplace
	resultSkip
	resultUnsupported
)

// Result is the outcome of a Rule. Construct via Context.Replace,
// Context.Skip, or Context.Unsupported — the zero value is invalid.
type Result struct {
	kind resultKind

	// replacement holds the new plugin name and body for resultReplace.
	replacement replacement

	// reason carries the explanation for resultSkip / resultUnsupported.
	reason string
}

// replacement is the payload of a resultReplace Result.
type replacement struct {
	name string
	body any // string for scalar bodies, structured Go value otherwise.

	// bloblangReport, when non-nil, is the report produced by the
	// bundled Bloblang V1->V2 translation that produced this body. The
	// outer migrator surfaces it on the per-component Change record so
	// callers can inspect mapping-level coverage and warnings.
	bloblangReport *bloblmig.Report
}
