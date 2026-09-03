// Copyright 2026 Redpanda Data, Inc.

package migrator

import "github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"

// MethodRule is the callback shape for a custom V1 method-call
// translation rule. Rules are registered with
// Migrator.RegisterMethodRule, keyed by the V1 method name. The
// callback receives a Context (helpers + Result constructors) and
// the wrapped V1 method-call node, and returns a Result describing
// the outcome.
//
// Custom rules win on name collision with the built-ins (the
// downstream rule fully replaces the built-in for that name).
type MethodRule func(ctx *Context, m *V1MethodCall) Result

// FunctionRule is the function-call analogue of MethodRule.
type FunctionRule func(ctx *Context, f *V1FunctionCall) Result

// resultKind is the discriminant for Result.
type resultKind int

const (
	resultUnset resultKind = iota
	resultReplace
	resultSkip
	resultUnsupported
)

// Result is the outcome of a rule. Construct via Context.Replace,
// Context.Skip, or Context.Unsupported — the zero value is invalid.
type Result struct {
	kind resultKind
	// expr is the V2 expression for resultReplace.
	expr syntax.Expr
	// reason carries the explanation for resultSkip / resultUnsupported.
	reason string
}
