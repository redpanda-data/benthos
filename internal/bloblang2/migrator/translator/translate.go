package translator

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

// translator holds the per-call state: the Change/Coverage recorder, a
// scope stack for lambda / named-capture parameter names, and helpers.
//
// Translation rules live in sibling files:
//
//   - statements.go — statement-level translators (assignment, let, if,
//     map decl, import).
//   - expressions.go — expression-level translators (literals, paths,
//     binary/unary operators, method/function calls, lambdas, match/if
//     expressions).
//   - methods.go — per-method V1→V2 rewrites dispatched from
//     translateMethodCall.
type translator struct {
	rec *recorder
	// scopes is a stack of named context frames. An ident is resolved from
	// the innermost frame outward; if not found, it falls back to the
	// legacy V1 bare-ident form (this.<name>). Each entry in a frame is
	// the parameter name introduced by a lambda or .(name -> body).
	scopes []scopeFrame
	// thisRebindStack tracks names that V1 `this` should resolve to inside
	// a V2 map body. When non-empty, the translator emits IdentExpr with
	// the top of the stack instead of `input` for a V1 ThisExpr.
	thisRebindStack []string
	// mapNamespace maps a V1 map name to the V2 namespace it lives in. For
	// locally-declared maps the namespace is "" (unqualified). For
	// imported maps it is the alias assigned to the import statement. Used
	// by `.apply("name")` rewrites to qualify the resulting V2 call.
	mapNamespace map[string]string
	// files is a snapshot of the outer Options.Files, carried on the
	// translator so translateImport can parse imported file contents to
	// learn the map names they declare.
	files map[string]string
	// ctxStack tracks the nearest enclosing construct that changes how
	// sentinel values (nothing(), deleted()) should be emitted in V2.
	// The top of the stack wins. See ctxKind for the enumerated contexts.
	ctxStack []ctxKind
}

// ctxKind is a translator-side marker for the kind of position we're
// currently rendering into. Used by nothing() rewrites to choose between
// void() (skip assignment) and deleted() (elide from collection).
type ctxKind int

const (
	// ctxCollectionLit is pushed while translating an element of an
	// array or an object-entry value — positions where V1's nothing()
	// silently elided in V1 and V2's deleted() serves the same role.
	ctxCollectionLit ctxKind = iota + 1
	// ctxVarDeclRHS is pushed while translating the RHS of a `let $x = …`
	// binding. V1 deletes the variable on nothing(); V2 errors on void
	// in a var-decl RHS — there is no semantic-preserving translation.
	ctxVarDeclRHS
)

// pushCtx pushes a translation context kind. Pair with popCtx.
func (t *translator) pushCtx(k ctxKind) { t.ctxStack = append(t.ctxStack, k) }

// popCtx removes the innermost context.
func (t *translator) popCtx() {
	if n := len(t.ctxStack); n > 0 {
		t.ctxStack = t.ctxStack[:n-1]
	}
}

// currentCtx returns the innermost context, or 0 if none.
func (t *translator) currentCtx() ctxKind {
	if n := len(t.ctxStack); n > 0 {
		return t.ctxStack[n-1]
	}
	return 0
}

// pushThisRebind makes V1 `this` translate to the given V2 identifier name
// (typically a map parameter) while the callback is active.
func (t *translator) pushThisRebind(name string) { t.thisRebindStack = append(t.thisRebindStack, name) }

func (t *translator) popThisRebind() {
	if n := len(t.thisRebindStack); n > 0 {
		t.thisRebindStack = t.thisRebindStack[:n-1]
	}
}

func (t *translator) currentThisRebind() (string, bool) {
	if n := len(t.thisRebindStack); n > 0 {
		return t.thisRebindStack[n-1], true
	}
	return "", false
}

// scopeFrame is one level of named-context bindings.
type scopeFrame struct {
	names map[string]struct{}
}

// pushScope adds a named-context frame. Callers must pair with popScope.
func (t *translator) pushScope(names ...string) {
	frame := scopeFrame{names: map[string]struct{}{}}
	for _, n := range names {
		if n != "" && n != "_" {
			frame.names[n] = struct{}{}
		}
	}
	t.scopes = append(t.scopes, frame)
}

// popScope removes the innermost frame.
func (t *translator) popScope() {
	if len(t.scopes) == 0 {
		return
	}
	t.scopes = t.scopes[:len(t.scopes)-1]
}

// isBoundIdent reports whether name matches a named-context binding in any
// active scope.
func (t *translator) isBoundIdent(name string) bool {
	for i := len(t.scopes) - 1; i >= 0; i-- {
		if _, ok := t.scopes[i].names[name]; ok {
			return true
		}
	}
	return false
}

// translateProgram walks a parsed V1 program and produces a V2 program. Every
// V1 node contributes to Coverage via recorder calls.
func (t *translator) translateProgram(p *v1ast.Program) *syntax.Program {
	if t.mapNamespace == nil {
		t.mapNamespace = map[string]string{}
	}
	// Register locally-declared map names first (unqualified namespace) so
	// later .apply() calls resolve correctly.
	for _, m := range p.Maps {
		t.mapNamespace[m.Name] = ""
	}

	out := &syntax.Program{}

	// ModeMapping prelude: V1 `mapping` starts `root` as the input
	// document, whereas V2 `output` starts as `{}`. Prepend an explicit
	// `output = input` so a V1 mapping whose statements only tweak
	// individual fields continues to pass the input through.
	if t.rec.opts.Mode == ModeMapping {
		t.rec.Rewritten(Change{
			Line:        1,
			Column:      1,
			Severity:    SeverityInfo,
			Category:    CategoryIdiomRewrite,
			RuleID:      RuleRootToOutput,
			Explanation: "ModeMapping: prepended `output = input` to preserve V1 mapping pass-through default",
		})
		out.Stmts = append(out.Stmts, &syntax.Assignment{
			TokenPos: syntax.Pos{Line: 1, Column: 1},
			Target: syntax.AssignTarget{
				Pos:  syntax.Pos{Line: 1, Column: 1},
				Root: syntax.AssignOutput,
			},
			Value: &syntax.InputExpr{TokenPos: syntax.Pos{Line: 1, Column: 1}},
		})
	}

	// Translate statements in original order, routing map decls and imports
	// to the dedicated slices while keeping everything else in Stmts. Each
	// V2 node inherits its V1 source's leading/trailing trivia.
	for _, stmt := range p.Stmts {
		switch s := stmt.(type) {
		case *v1ast.MapDecl:
			if m := t.translateMapDecl(s); m != nil {
				copyTriviaTo(s, m)
				out.Maps = append(out.Maps, m)
			}
		case *v1ast.ImportStmt:
			if i := t.translateImport(s); i != nil {
				copyTriviaTo(s, i)
				out.Imports = append(out.Imports, i)
			}
		case *v1ast.FromStmt:
			// `from "path"` replaces the whole mapping in V1 with zero V2
			// equivalent short of inlining the imported file. We flag and
			// drop — caller should manually inline.
			t.rec.Unsupported(Change{
				Line: s.Pos.Line, Column: s.Pos.Column,
				RuleID:      RuleFromStatement,
				SpecRef:     "§10.5 / §14#12",
				Explanation: `V1 "from" replaces the whole mapping — inline the imported file manually`,
			})
		default:
			v2 := t.translateStmt(stmt)
			if v2 != nil {
				copyTrivia(stmt, v2)
				out.Stmts = append(out.Stmts, v2)
			}
		}
	}

	return out
}

// pos converts a V1 position to a V2 position. Same structure, different
// package.
func pos(p v1ast.Pos) syntax.Pos {
	return syntax.Pos{Line: p.Line, Column: p.Column}
}
