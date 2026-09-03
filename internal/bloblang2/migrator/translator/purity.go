package translator

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

// registryImpureFuncs is the set of V1 core functions the Bloblang function
// registry marks impure — i.e. they access or mutate external state (e.g.
// `count`). Derived once from query.AllFunctions so it tracks the registry
// rather than a hand-maintained list. NOTE: the Impure flag covers stateful/IO
// functions but NOT pure-nondeterministic ones (uuid_v4, now, ...), which are
// listed separately below.
var registryImpureFuncs = func() map[string]struct{} {
	m := map[string]struct{}{}
	for _, spec := range query.AllFunctions.Docs() {
		if spec.Impure {
			m[spec.Name] = struct{}{}
		}
	}
	return m
}()

// nondeterministicV1Funcs are V1 functions that return a different value on
// each call without touching external state, so the registry does not flag
// them Impure — yet duplicating them in the `|`/`.or` coalesce rewrite still
// diverges from V1's single evaluation. Distribution-provided plugins (e.g.
// Connect's snowflake_id / counter) are NOT visible from benthos core; callers
// with such plugins should supply Options.IsImpureFunc. snowflake_id/counter
// are included here as a best-effort safety net for callers that don't.
var nondeterministicV1Funcs = map[string]struct{}{
	"uuid_v4":              {},
	"uuid_v7":              {},
	"nanoid":               {},
	"ksuid":                {},
	"random_int":           {},
	"now":                  {},
	"timestamp_unix":       {},
	"timestamp_unix_nano":  {},
	"timestamp_unix_micro": {},
	"timestamp_unix_milli": {},
	"snowflake_id":         {}, // Connect plugin
	"counter":              {}, // Connect plugin
}

// funcMayDiverge reports whether a V1 function name is nondeterministic or
// stateful, so that evaluating it more than once (as the coalesce rewrite does
// on the error path) can diverge from V1's single evaluation. It combines the
// registry's impure set, the curated nondeterministic set, and any caller
// predicate (Options.IsImpureFunc) — the latter lets a distribution report its
// own plugin functions.
func (t *translator) funcMayDiverge(name string) bool {
	if _, ok := registryImpureFuncs[name]; ok {
		return true
	}
	if _, ok := nondeterministicV1Funcs[name]; ok {
		return true
	}
	if fn := t.rec.opts.IsImpureFunc; fn != nil && fn(name) {
		return true
	}
	return false
}

// exprMayDivergeIfDuplicated reports whether evaluating e more than once could
// yield a different result or an extra side effect than a single evaluation.
// Pure reads and operators over pure operands are safe to duplicate; a
// nondeterministic/stateful function call (directly or nested — including
// inside a lambda body, array/object literal, or if/match branch) is not.
func (t *translator) exprMayDivergeIfDuplicated(e v1ast.Expr) bool {
	switch x := e.(type) {
	case *v1ast.FunctionCall:
		if t.funcMayDiverge(x.Name) {
			return true
		}
		return t.anyArgMayDiverge(x.Args)
	case *v1ast.MethodCall:
		if x.Name == "apply" { // invokes a possibly stateful/nondeterministic map
			return true
		}
		if t.funcMayDiverge(x.Name) {
			return true
		}
		return t.exprMayDivergeIfDuplicated(x.Recv) || t.anyArgMayDiverge(x.Args)
	case *v1ast.BinaryExpr:
		return t.exprMayDivergeIfDuplicated(x.Left) || t.exprMayDivergeIfDuplicated(x.Right)
	case *v1ast.UnaryExpr:
		return t.exprMayDivergeIfDuplicated(x.Operand)
	case *v1ast.ParenExpr:
		return t.exprMayDivergeIfDuplicated(x.Inner)
	case *v1ast.FieldAccess:
		return t.exprMayDivergeIfDuplicated(x.Recv)
	case *v1ast.MetaCall:
		return t.exprMayDivergeIfDuplicated(x.Key)
	case *v1ast.Lambda:
		return t.exprMayDivergeIfDuplicated(x.Body)
	case *v1ast.ArrayLit:
		for _, el := range x.Elems {
			if t.exprMayDivergeIfDuplicated(el) {
				return true
			}
		}
		return false
	case *v1ast.ObjectLit:
		for _, en := range x.Entries {
			if t.exprMayDivergeIfDuplicated(en.Key) || t.exprMayDivergeIfDuplicated(en.Value) {
				return true
			}
		}
		return false
	case *v1ast.IfExpr:
		for _, br := range x.Branches {
			if t.exprMayDivergeIfDuplicated(br.Cond) || t.exprMayDivergeIfDuplicated(br.Body) {
				return true
			}
		}
		return x.Else != nil && t.exprMayDivergeIfDuplicated(x.Else)
	case *v1ast.MatchExpr:
		if x.Subject != nil && t.exprMayDivergeIfDuplicated(x.Subject) {
			return true
		}
		for _, c := range x.Cases {
			if (c.Pattern != nil && t.exprMayDivergeIfDuplicated(c.Pattern)) || t.exprMayDivergeIfDuplicated(c.Body) {
				return true
			}
		}
		return false
	case *v1ast.MapExpr:
		return t.exprMayDivergeIfDuplicated(x.Recv) || t.exprMayDivergeIfDuplicated(x.Body)
	default:
		// Literal, Ident, ThisExpr, RootExpr, VarRef, MetaRef: pure reads.
		return false
	}
}

func (t *translator) anyArgMayDiverge(args []v1ast.CallArg) bool {
	for _, a := range args {
		if t.exprMayDivergeIfDuplicated(a.Value) {
			return true
		}
	}
	return false
}
