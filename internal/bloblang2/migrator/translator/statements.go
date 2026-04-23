package translator

import (
	"fmt"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

// translateStmt dispatches on concrete statement type.
func (t *translator) translateStmt(stmt v1ast.Stmt) syntax.Stmt {
	switch s := stmt.(type) {
	case *v1ast.Assignment:
		return t.translateAssignment(s)
	case *v1ast.LetStmt:
		return t.translateLet(s)
	case *v1ast.IfStmt:
		return t.translateIfStmt(s)
	case *v1ast.BareExprStmt:
		return t.translateBareExpr(s)
	default:
		t.rec.Unsupported(Change{
			Line: stmt.NodePos().Line, Column: stmt.NodePos().Column,
			RuleID:      RuleUnsupportedConstruct,
			Explanation: fmt.Sprintf("no translation rule for statement %T", stmt),
		})
		return nil
	}
}

// translateStmts maps a slice of V1 statements to V2. Each V2 node inherits
// the V1 source's leading/trailing trivia (comments + blank lines).
func (t *translator) translateStmts(stmts []v1ast.Stmt) []syntax.Stmt {
	var out []syntax.Stmt
	for _, s := range stmts {
		v2 := t.translateStmt(s)
		if v2 != nil {
			copyTrivia(s, v2)
			out = append(out, v2)
		}
	}
	return out
}

// translateAssignment rewrites a V1 `target = expr` into V2 `target = expr`.
func (t *translator) translateAssignment(a *v1ast.Assignment) syntax.Stmt {
	target := t.translateTarget(a.Target)
	value := t.translateExpr(a.Value)
	if target == nil || value == nil {
		return nil
	}
	// Whether this assignment counts as Exact or Rewritten depends on whether
	// translateTarget emitted a Change. The target helper records its own
	// coverage for the target node; here we just count the assignment itself.
	t.rec.Exact()
	return &syntax.Assignment{
		TokenPos: pos(a.Pos),
		Target:   *target,
		Value:    value,
	}
}

// translateTarget translates the LHS of an assignment.
func (t *translator) translateTarget(tgt v1ast.AssignTarget) *syntax.AssignTarget {
	switch tgt.Kind {
	case v1ast.TargetRoot:
		t.rec.Exact()
		return &syntax.AssignTarget{
			Pos:  pos(tgt.Pos),
			Root: syntax.AssignOutput,
			Path: t.translatePathSegments(tgt.Path),
		}
	case v1ast.TargetThis:
		// V1 accepts `this = v` / `this.foo = v` but produces a literal "this"
		// top-level key rather than aliasing to root (§14#72). V2 has no
		// equivalent — translate to the most-likely-intended `output`.
		t.rec.Rewritten(Change{
			Line: tgt.Pos.Line, Column: tgt.Pos.Column,
			Severity:    SeverityWarning,
			Category:    CategorySemanticChange,
			RuleID:      RuleThisTargetToOutput,
			SpecRef:     "§14#72",
			Explanation: `V1 treats "this" at target position as a literal top-level key; translated to "output"`,
		})
		return &syntax.AssignTarget{
			Pos:  pos(tgt.Pos),
			Root: syntax.AssignOutput,
			Path: t.translatePathSegments(tgt.Path),
		}
	case v1ast.TargetBare:
		// Bare-path target: `foo.bar = v` → `output.foo.bar = v`.
		t.rec.Rewritten(Change{
			Line: tgt.Pos.Line, Column: tgt.Pos.Column,
			Severity:    SeverityInfo,
			Category:    CategoryIdiomRewrite,
			RuleID:      RuleBarePathToOutput,
			SpecRef:     "§14#2",
			Explanation: "bare-path assignment target rewritten with explicit output root",
		})
		return &syntax.AssignTarget{
			Pos:  pos(tgt.Pos),
			Root: syntax.AssignOutput,
			Path: t.translatePathSegments(tgt.Path),
		}
	case v1ast.TargetMeta:
		t.rec.Rewritten(Change{
			Line: tgt.Pos.Line, Column: tgt.Pos.Column,
			Severity:    SeverityInfo,
			Category:    CategoryIdiomRewrite,
			RuleID:      RuleMetaTargetToOutputMeta,
			Explanation: "meta target translated to output@",
		})
		return &syntax.AssignTarget{
			Pos:        pos(tgt.Pos),
			Root:       syntax.AssignOutput,
			MetaAccess: true,
			Path:       t.translatePathSegments(tgt.Path),
		}
	}
	t.rec.Unsupported(Change{
		Line: tgt.Pos.Line, Column: tgt.Pos.Column,
		RuleID:      RuleUnsupportedConstruct,
		Explanation: fmt.Sprintf("unknown target kind %v", tgt.Kind),
	})
	return nil
}

// translatePathSegments converts V1 path segments to V2 path segments on an
// AssignTarget.
func (t *translator) translatePathSegments(segs []v1ast.PathSegment) []syntax.PathSegment {
	out := make([]syntax.PathSegment, 0, len(segs))
	for _, s := range segs {
		out = append(out, syntax.PathSegment{
			Kind: syntax.PathSegField,
			Name: s.Name,
			Pos:  pos(s.Pos),
		})
	}
	return out
}

// translateLet rewrites `let x = expr` to V2 equivalent. V2 expresses
// variable declaration the same way at statement position.
func (t *translator) translateLet(l *v1ast.LetStmt) syntax.Stmt {
	if l.NameQuoted {
		// §7.2 quoted binding names with non-identifier characters are
		// write-only in V1. Translate to an unquoted best-effort name if
		// possible; emit a SemanticChange otherwise.
		t.rec.Rewritten(Change{
			Line: l.Pos.Line, Column: l.Pos.Column,
			Severity:    SeverityWarning,
			Category:    CategorySemanticChange,
			RuleID:      RuleUnsupportedConstruct,
			SpecRef:     "§7.2 / §14#76",
			Explanation: "V1 quoted let-binding name preserved verbatim in V2 (may not be readable)",
		})
	} else {
		t.rec.Exact()
	}
	t.pushCtx(ctxVarDeclRHS)
	value := t.translateExpr(l.Value)
	t.popCtx()
	if value == nil {
		return nil
	}
	return &syntax.Assignment{
		TokenPos: pos(l.Pos),
		Target: syntax.AssignTarget{
			Pos:     pos(l.NamePos),
			Root:    syntax.AssignVar,
			VarName: l.Name,
		},
		Value: value,
	}
}

// translateIfStmt rewrites statement-form if/else if/else.
func (t *translator) translateIfStmt(i *v1ast.IfStmt) syntax.Stmt {
	t.rec.Exact()
	out := &syntax.IfStmt{TokenPos: pos(i.Pos)}
	for _, br := range i.Branches {
		t.flagBranchLets(br.Body)
		cond := t.translateExpr(br.Cond)
		body := t.translateStmts(br.Body)
		if cond == nil {
			continue
		}
		out.Branches = append(out.Branches, syntax.IfBranch{Cond: cond, Body: body})
	}
	if i.Else != nil {
		t.flagBranchLets(i.Else)
		out.Else = t.translateStmts(i.Else)
	}
	return out
}

// flagBranchLets emits a SemanticChange for any `let` at the top of an
// if/else branch body. V1 leaks the binding into the enclosing mapping
// scope; V2 confines it to the branch. If the binding is referenced outside
// the branch the V2 output won't compile — we flag unconditionally so the
// divergence is surfaced whether or not that reference exists.
func (t *translator) flagBranchLets(body []v1ast.Stmt) {
	for _, s := range body {
		l, ok := s.(*v1ast.LetStmt)
		if !ok {
			continue
		}
		p := l.NodePos()
		t.rec.Note(Change{
			Line: p.Line, Column: p.Column,
			Severity:    SeverityWarning,
			Category:    CategorySemanticChange,
			RuleID:      RuleBlockScopedLet,
			SpecRef:     "§11",
			Explanation: "V1 let-bindings leak out of if/else branches; V2 scopes them per block. Move this declaration to the outer scope if the variable is used after the branch.",
		})
	}
}

// translateBareExpr handles `<expr>` as the sole statement of a mapping. V1
// treats this as `root = <expr>`; we emit the equivalent V2 assignment.
func (t *translator) translateBareExpr(b *v1ast.BareExprStmt) syntax.Stmt {
	t.rec.Rewritten(Change{
		Line: b.Pos.Line, Column: b.Pos.Column,
		Severity:    SeverityInfo,
		Category:    CategoryIdiomRewrite,
		RuleID:      RuleRootToOutput,
		SpecRef:     "§7.4 / §14#16",
		Explanation: "bare-expression mapping shorthand rewritten as explicit output assignment",
	})
	val := t.translateExpr(b.Expr)
	if val == nil {
		return nil
	}
	return &syntax.Assignment{
		TokenPos: pos(b.Pos),
		Target: syntax.AssignTarget{
			Pos:  pos(b.Pos),
			Root: syntax.AssignOutput,
		},
		Value: val,
	}
}

// translateMapDecl translates `map foo { ... }` to V2.
//
// V1 map bodies are statement lists that assemble a `root` value; the map's
// implicit receiver is accessible as `this`. V2 map bodies are ExprBody:
// zero or more variable assignments followed by a single result expression,
// and parameters are explicit.
//
// The translation strategy:
//  1. Give the V2 map a single parameter named "in" — the receiver.
//  2. Rebind V1 `this` to that parameter inside the body.
//  3. Translate V1 `let` statements to V2 VarAssigns, kept in order.
//  4. Translate the last `root = expr` (or sole statement) as the Result.
//  5. If the body contains multiple `root.x = ...` assignments, assemble
//     them into an object literal as the Result.
//  6. Otherwise (complex, unsupported shapes), flag and stub.
func (t *translator) translateMapDecl(m *v1ast.MapDecl) *syntax.MapDecl {
	const paramName = "in"

	t.pushScope(paramName)
	t.pushThisRebind(paramName)
	defer t.popScope()
	defer t.popThisRebind()

	body, ok := t.tryTranslateMapBody(m)
	if !ok {
		t.rec.Rewritten(Change{
			Line: m.Pos.Line, Column: m.Pos.Column,
			Severity: SeverityWarning, Category: CategoryUnsupported,
			RuleID:      RuleMapDeclTranslation,
			Explanation: "map body shape could not be translated; emitted stub returning input",
		})
		body = &syntax.ExprBody{
			Result: &syntax.IdentExpr{
				TokenPos:  pos(m.Pos),
				Name:      paramName,
				SlotIndex: -1,
			},
		}
	} else {
		t.rec.Exact()
	}

	return &syntax.MapDecl{
		TokenPos: pos(m.Pos),
		Name:     m.Name,
		Params:   []syntax.Param{{Name: paramName, Pos: pos(m.Pos), SlotIndex: -1}},
		Body:     body,
	}
}

// tryTranslateMapBody attempts to translate a V1 map body into a V2 ExprBody.
// Returns (body, true) on success; (nil, false) when the body shape isn't
// supported by the current rules (caller substitutes a stub).
func (t *translator) tryTranslateMapBody(m *v1ast.MapDecl) (*syntax.ExprBody, bool) {
	out := &syntax.ExprBody{}
	var rootAssigns []*v1ast.Assignment
	var finalResult syntax.Expr
	for _, stmt := range m.Body {
		switch s := stmt.(type) {
		case *v1ast.LetStmt:
			val := t.translateExpr(s.Value)
			if val == nil {
				return nil, false
			}
			va := &syntax.VarAssign{
				TokenPos:  pos(s.Pos),
				Name:      s.Name,
				Value:     val,
				SlotIndex: -1,
			}
			copyTriviaTo(s, va)
			out.Assignments = append(out.Assignments, va)
		case *v1ast.Assignment:
			// Only handle root-rooted assignments here. Other target kinds
			// (meta, bare, this) aren't valid inside a map body per V1
			// semantics (§10.1), but the V1 parser may have accepted them —
			// bail.
			if s.Target.Kind != v1ast.TargetRoot {
				return nil, false
			}
			if len(s.Target.Path) == 0 {
				// Whole-root replacement: `root = expr`. Becomes the result
				// directly, superseding any previous field-level asserts.
				v := t.translateExpr(s.Value)
				if v == nil {
					return nil, false
				}
				finalResult = v
				rootAssigns = nil
				continue
			}
			// Field-level assignment: accumulate for object-literal
			// construction.
			rootAssigns = append(rootAssigns, s)
		default:
			// Unsupported map body statement kind.
			return nil, false
		}
	}

	switch {
	case finalResult != nil && len(rootAssigns) == 0:
		out.Result = finalResult
	case len(rootAssigns) > 0 && finalResult == nil:
		// Build an object literal from the accumulated root.<path> = v
		// assignments. Only one-level paths are supported here; deeper
		// paths would require nested objects which a future rule can add.
		obj := &syntax.ObjectLiteral{LBracePos: pos(m.Pos)}
		for _, a := range rootAssigns {
			if len(a.Target.Path) != 1 {
				return nil, false
			}
			v := t.translateExpr(a.Value)
			if v == nil {
				return nil, false
			}
			key := &syntax.LiteralExpr{
				TokenPos: pos(a.Target.Pos), TokenType: syntax.STRING,
				Value: a.Target.Path[0].Name,
			}
			obj.Entries = append(obj.Entries, syntax.ObjectEntry{Key: key, Value: v})
		}
		out.Result = obj
	case finalResult == nil && len(rootAssigns) == 0:
		// Empty map body: return input unchanged.
		out.Result = &syntax.IdentExpr{TokenPos: pos(m.Pos), Name: "in", SlotIndex: -1}
	default:
		// `root = X` mixed with `root.y = Y`: ambiguous. Bail.
		return nil, false
	}

	return out, true
}

// translateImport translates `import "path"` to V2. Assigns a synthetic
// namespace alias and records every map name in the imported file so that
// subsequent `.apply(name)` call sites can be qualified.
func (t *translator) translateImport(i *v1ast.ImportStmt) *syntax.ImportStmt {
	lit, ok := i.Path.(*v1ast.Literal)
	if !ok {
		t.rec.Unsupported(Change{
			Line: i.Pos.Line, Column: i.Pos.Column,
			RuleID:      RuleImportStatement,
			Explanation: "import path is not a string literal",
		})
		return nil
	}
	ns := namespaceFromPath(lit.Str)
	// Record every map in the imported file under this namespace.
	if content, ok := t.importedContent(lit.Str); ok {
		if prog, err := v1ast.Parse(content); err == nil {
			for _, m := range prog.Maps {
				// Last import wins on map-name collision; V1 rejects this
				// at parse but best-effort on our side.
				t.mapNamespace[m.Name] = ns
			}
		}
	}
	t.rec.Rewritten(Change{
		Line: i.Pos.Line, Column: i.Pos.Column,
		Severity:    SeverityInfo,
		Category:    CategoryIdiomRewrite,
		RuleID:      RuleImportStatement,
		Explanation: `V1 import rewritten with synthetic V2 namespace alias`,
	})
	return &syntax.ImportStmt{
		TokenPos:  pos(i.Pos),
		Path:      lit.Str,
		Namespace: ns,
	}
}

// importedContent retrieves the V1 source of an imported file from the
// translator's Options.Files (threaded via Migrate). Returns ("", false)
// if the file isn't in the map.
func (t *translator) importedContent(path string) (string, bool) {
	if t.files == nil {
		return "", false
	}
	if s, ok := t.files[path]; ok {
		return s, true
	}
	return "", false
}

// namespaceFromPath derives a V2 namespace alias from a V1 import path. It
// strips directories and the .blobl extension, leaves something identifier-
// safe, and falls back to "imported" for unusual shapes.
func namespaceFromPath(p string) string {
	s := p
	// Strip directory.
	if idx := lastIndexByte(s, '/'); idx >= 0 {
		s = s[idx+1:]
	}
	// Strip extension.
	if idx := lastIndexByte(s, '.'); idx >= 0 {
		s = s[:idx]
	}
	// Replace non-identifier characters with '_'.
	var b []byte
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b = append(b, byte(r))
		default:
			b = append(b, '_')
		}
	}
	if len(b) == 0 || (b[0] >= '0' && b[0] <= '9') {
		return "imported"
	}
	return string(b)
}

// lastIndexByte is a tiny replacement for strings.LastIndexByte to avoid
// an import just for one call.
func lastIndexByte(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}
