// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// Context is the per-rule handle handed to a custom MethodRule or
// FunctionRule. It exposes the translator helpers a rule needs
// (recursive translation, scope / this-rebind management, source
// position translation, additional diagnostics) plus the three
// Result constructors that drive coverage tracking.
//
// The Context value is only valid for the duration of the rule
// invocation that produced it; storing it for later use is undefined.
type Context struct {
	t         translator.Translator
	defaultV1 v1Position // V1 source position to use when a Result reason omits one
}

type v1Position struct {
	Line   int
	Column int
}

// Translate recursively translates a V1 sub-expression into V2. Use
// for the receiver, arguments, or any nested V1 node a rule passes
// through to V2 rather than transforms itself. Returns nil if
// translation cannot proceed (the translator already emitted the
// appropriate diagnostic).
func (c *Context) Translate(e V1Expr) V2Expr {
	if e == nil {
		return nil
	}
	out := c.t.TranslateExpr(e.unwrapV1())
	if out == nil {
		return nil
	}
	return wrapV2(out)
}

// PushScope pushes a named-context frame for the duration of a
// translation walk inside the rule. Each name becomes a bound
// identifier (lambda parameter) so V1 bare-ident references resolve
// to the parameter rather than `input.<name>`. Pair every PushScope
// with a corresponding PopScope.
func (c *Context) PushScope(names ...string) { c.t.PushScope(names...) }

// PopScope removes the innermost scope frame.
func (c *Context) PopScope() { c.t.PopScope() }

// PushThisRebind makes V1 `this` translate to the given V2 identifier
// while subsequent Translate calls walk inside it. Used when a rule
// synthesizes a V2 lambda whose parameter takes over what V1 `this`
// referred to (e.g. wrapping a query-form predicate). Pair with
// PopThisRebind.
func (c *Context) PushThisRebind(name string) { c.t.PushThisRebind(name) }

// PopThisRebind removes the innermost this-rebinding.
func (c *Context) PopThisRebind() { c.t.PopThisRebind() }

// Pos translates a public V1 source position to a public V2 source
// position. Mostly a convenience — the structures match — but routing
// through this method keeps rule code stable if either side's Pos
// representation changes.
func (c *Context) Pos(p Pos) Pos { return p }

// Note records an additional Change record alongside the rule's
// Result. Coverage counters are not affected; this hook is for
// flagging semantic divergences or extra context the rule wants in
// the Report. Line/Column are filled from the V1 node currently
// being processed when the Change leaves them as zero.
func (c *Context) Note(ch Change) {
	if ch.Line == 0 {
		ch.Line = c.defaultV1.Line
	}
	if ch.Column == 0 {
		ch.Column = c.defaultV1.Column
	}
	c.t.EmitChange(ch)
}

// Replace produces a Result that swaps the V1 node for the supplied
// V2 expression. The translator records a Rewritten Change carrying
// the rule's name as part of the explanation.
func (c *Context) Replace(e V2Expr) Result {
	if e == nil {
		return Result{kind: resultUnsupported, reason: "rule returned a nil replacement"}
	}
	return Result{kind: resultReplace, expr: e.unwrapV2()}
}

// Skip produces a Result that falls through to the default 1:1
// translation. Use this when V1 and V2 forms agree byte-for-byte but
// the rule wanted to attach a reason or guard against a future
// built-in being added under the same name.
func (c *Context) Skip(reason string) Result {
	return Result{kind: resultSkip, reason: reason}
}

// Unsupported produces a Result that flags the V1 construct as
// untranslatable. The translator records an Error-severity
// Unsupported Change and emits a `// MIGRATION:` comment in the V2
// output where the V1 node sat.
func (c *Context) Unsupported(reason string) Result {
	return Result{kind: resultUnsupported, reason: reason}
}
