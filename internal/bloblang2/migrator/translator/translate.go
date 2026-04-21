package translator

import (
	"fmt"
	"strconv"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

// translator holds the per-call state: the Change/Coverage recorder, a
// scope stack for lambda / named-capture parameter names, and helpers.
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

	// Translate statements in original order, routing map decls and imports
	// to the dedicated slices while keeping everything else in Stmts.
	for _, stmt := range p.Stmts {
		switch s := stmt.(type) {
		case *v1ast.MapDecl:
			if m := t.translateMapDecl(s); m != nil {
				out.Maps = append(out.Maps, m)
			}
		case *v1ast.ImportStmt:
			if i := t.translateImport(s); i != nil {
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
			if v2 := t.translateStmt(stmt); v2 != nil {
				out.Stmts = append(out.Stmts, v2)
			}
		}
	}

	return out
}

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
	value := t.translateExpr(l.Value)
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

// translateStmts maps a slice of V1 statements to V2.
func (t *translator) translateStmts(stmts []v1ast.Stmt) []syntax.Stmt {
	var out []syntax.Stmt
	for _, s := range stmts {
		if v2 := t.translateStmt(s); v2 != nil {
			out = append(out, v2)
		}
	}
	return out
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
			out.Assignments = append(out.Assignments, &syntax.VarAssign{
				TokenPos:  pos(s.Pos),
				Name:      s.Name,
				Value:     val,
				SlotIndex: -1,
			})
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
// translator's Options.Files (threaded via migrate). Returns ("", false) if
// the file isn't in the map.
//
// Because translator doesn't currently carry Options after Migrate has
// finished applyDefaults, we stash a pointer during translateProgram. For
// now we read from a package-level slot; see files field on translator
// below.
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
	// Trim trailing quote characters if any (unlikely but defensive).
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

// translateExpr dispatches expression translation.
func (t *translator) translateExpr(e v1ast.Expr) syntax.Expr {
	switch x := e.(type) {
	case *v1ast.Literal:
		return t.translateLiteral(x)
	case *v1ast.ThisExpr:
		// Inside a V2 map body, V1 `this` refers to the map's receiver,
		// which we surface as a named V2 parameter. Otherwise `this` is
		// the top-level input.
		if name, ok := t.currentThisRebind(); ok {
			t.rec.Exact()
			return &syntax.IdentExpr{TokenPos: pos(x.TokPos), Name: name, SlotIndex: -1}
		}
		t.rec.Rewritten(Change{
			Line: x.TokPos.Line, Column: x.TokPos.Column,
			Severity: SeverityInfo, Category: CategoryIdiomRewrite,
			RuleID: RuleThisToInput, Explanation: `"this" rewritten to "input"`,
		})
		return &syntax.InputExpr{TokenPos: pos(x.TokPos)}
	case *v1ast.RootExpr:
		t.rec.Rewritten(Change{
			Line: x.TokPos.Line, Column: x.TokPos.Column,
			Severity: SeverityInfo, Category: CategoryIdiomRewrite,
			RuleID: RuleRootToOutput, Explanation: `"root" rewritten to "output"`,
		})
		return &syntax.OutputExpr{TokenPos: pos(x.TokPos)}
	case *v1ast.VarRef:
		t.rec.Exact()
		return &syntax.VarExpr{TokenPos: pos(x.TokPos), Name: x.Name, SlotIndex: -1}
	case *v1ast.MetaRef:
		t.rec.Rewritten(Change{
			Line: x.TokPos.Line, Column: x.TokPos.Column,
			Severity: SeverityInfo, Category: CategoryIdiomRewrite,
			RuleID: RuleMetaReadToInputMeta, Explanation: "metadata reference rewritten to input@",
		})
		return t.metaReadExpr(x)
	case *v1ast.Ident:
		// If the name is a lambda parameter or named-context binding in
		// scope, emit a V2 identifier reference — not the legacy
		// bare-ident-to-input rewrite.
		if t.isBoundIdent(x.Name) {
			t.rec.Exact()
			return &syntax.IdentExpr{TokenPos: pos(x.TokPos), Name: x.Name, SlotIndex: -1}
		}
		// Otherwise, legacy bare identifier in expression position =
		// `this.foo` = V2 `input.foo`.
		t.rec.Rewritten(Change{
			Line: x.TokPos.Line, Column: x.TokPos.Column,
			Severity: SeverityInfo, Category: CategoryIdiomRewrite,
			RuleID: RuleBareIdentToInput, SpecRef: "§14#1",
			Explanation: fmt.Sprintf(`bare identifier %q rewritten as "input.%s"`, x.Name, x.Name),
		})
		return &syntax.FieldAccessExpr{
			Receiver: &syntax.InputExpr{TokenPos: pos(x.TokPos)},
			Field:    x.Name,
			FieldPos: pos(x.TokPos),
		}
	case *v1ast.BinaryExpr:
		return t.translateBinary(x)
	case *v1ast.UnaryExpr:
		return t.translateUnary(x)
	case *v1ast.ParenExpr:
		// V2 parser also produces ParenExpr? Let me inline — wrap in nothing
		// and let the printer add parens as needed via precedence.
		inner := t.translateExpr(x.Inner)
		t.rec.Exact()
		return inner
	case *v1ast.FieldAccess:
		return t.translateFieldAccess(x)
	case *v1ast.MethodCall:
		return t.translateMethodCall(x)
	case *v1ast.FunctionCall:
		return t.translateFunctionCall(x)
	case *v1ast.MetaCall:
		return t.translateMetaCall(x)
	case *v1ast.Lambda:
		return t.translateLambda(x)
	case *v1ast.ArrayLit:
		return t.translateArrayLit(x)
	case *v1ast.ObjectLit:
		return t.translateObjectLit(x)
	case *v1ast.IfExpr:
		return t.translateIfExpr(x)
	case *v1ast.MatchExpr:
		return t.translateMatchExpr(x)
	case *v1ast.MapExpr:
		return t.translateMapExpr(x)
	default:
		t.rec.Unsupported(Change{
			Line: e.NodePos().Line, Column: e.NodePos().Column,
			RuleID:      RuleUnsupportedConstruct,
			Explanation: fmt.Sprintf("no translation rule for expression %T", e),
		})
		return nil
	}
}

// translateLiteral is a straight passthrough — literals are identical in V1
// and V2 modulo the `\/` escape (which is not supported in V1, so it never
// appears in parsed input).
func (t *translator) translateLiteral(l *v1ast.Literal) syntax.Expr {
	t.rec.Exact()
	out := &syntax.LiteralExpr{TokenPos: pos(l.TokPos)}
	switch l.Kind {
	case v1ast.LitNull:
		out.TokenType = syntax.NULL
		out.Value = "null"
	case v1ast.LitBool:
		if l.Bool {
			out.TokenType = syntax.TRUE
			out.Value = "true"
		} else {
			out.TokenType = syntax.FALSE
			out.Value = "false"
		}
	case v1ast.LitInt:
		out.TokenType = syntax.INT
		out.Value = strconv.FormatInt(l.Int, 10)
	case v1ast.LitFloat:
		out.TokenType = syntax.FLOAT
		if l.Raw != "" {
			out.Value = l.Raw
		} else {
			out.Value = fmt.Sprintf("%g", l.Float)
		}
	case v1ast.LitString:
		out.TokenType = syntax.STRING
		out.Value = l.Str
	case v1ast.LitRawString:
		out.TokenType = syntax.RAW_STRING
		out.Value = l.Str
	}
	return out
}

// metaReadExpr builds the V2 form of a V1 `@name` / `meta("name")` read.
func (t *translator) metaReadExpr(m *v1ast.MetaRef) syntax.Expr {
	if m.Name == "" {
		return &syntax.InputMetaExpr{TokenPos: pos(m.TokPos)}
	}
	return &syntax.FieldAccessExpr{
		Receiver: &syntax.InputMetaExpr{TokenPos: pos(m.TokPos)},
		Field:    m.Name,
		FieldPos: pos(m.TokPos),
	}
}

// translateBinary maps V1 binary operators to V2. Most are 1:1, but `|`
// coalesce and the `&&`/`||` same-precedence quirk need care.
func (t *translator) translateBinary(b *v1ast.BinaryExpr) syntax.Expr {
	left := t.translateExpr(b.Left)
	right := t.translateExpr(b.Right)
	if left == nil || right == nil {
		return nil
	}
	// `|` coalesce: V2 has no direct binary operator for this. Rewrite to
	// `left.or(right)`. Not bit-exact because V1 `.or` also catches errors
	// where V2 doesn't, but the closest idiom.
	if b.Op == v1ast.TokPipe {
		t.rec.Rewritten(Change{
			Line: b.OpPos.Line, Column: b.OpPos.Column,
			Severity: SeverityInfo, Category: CategoryIdiomRewrite,
			RuleID: RuleCoalescePrecedence, SpecRef: "§14#4",
			Explanation: "V1 `|` coalesce rewritten as V2 `.or(...)`",
		})
		return &syntax.MethodCallExpr{
			Receiver:  left,
			Method:    "or",
			MethodPos: pos(b.OpPos),
			Args:      []syntax.CallArg{{Value: right}},
		}
	}
	op, ok := mapV1BinaryOp(b.Op)
	if !ok {
		t.rec.Unsupported(Change{
			Line: b.OpPos.Line, Column: b.OpPos.Column,
			RuleID:      RuleUnsupportedConstruct,
			Explanation: fmt.Sprintf("unmapped binary operator kind %v", b.Op),
		})
		return nil
	}
	t.flagOperatorDivergence(b)
	t.rec.Exact()
	return &syntax.BinaryExpr{Left: left, Op: op, OpPos: pos(b.OpPos), Right: right}
}

// translateUnary handles `!x` and unary `-x`.
func (t *translator) translateUnary(u *v1ast.UnaryExpr) syntax.Expr {
	inner := t.translateExpr(u.Operand)
	if inner == nil {
		return nil
	}
	op, ok := mapV1UnaryOp(u.Op)
	if !ok {
		t.rec.Unsupported(Change{
			Line: u.OpPos.Line, Column: u.OpPos.Column,
			RuleID:      RuleUnsupportedConstruct,
			Explanation: fmt.Sprintf("unmapped unary operator kind %v", u.Op),
		})
		return nil
	}
	t.rec.Exact()
	return &syntax.UnaryExpr{Op: op, OpPos: pos(u.OpPos), Operand: inner}
}

// translateFieldAccess recursively walks the V1 field-access chain.
//
// V1 path access is universally null-tolerant: reading any field of a non-
// object returns null (§12.5). V2 defaults to strict: `null.field` errors.
// To preserve V1 semantics we emit the null-safe V2 form `?.field` on every
// field access. This handles the null case; wrong-type receivers (e.g.
// `5.field`) still error in V2 even with `?.`, which is a genuine V1-V2
// divergence flagged separately when it arises.
//
// A V1 path segment whose name is an all-digit string (e.g. `this.items.0`)
// is V1's array-indexing syntax (§6.3). V2 uses bracket indexing for that,
// so we emit an IndexExpr with the numeric value rather than a literal field
// named "0".
func (t *translator) translateFieldAccess(f *v1ast.FieldAccess) syntax.Expr {
	recv := t.translateExpr(f.Recv)
	if recv == nil {
		return nil
	}
	if !f.Seg.Quoted && isAllDigits(f.Seg.Name) {
		t.rec.Rewritten(Change{
			Line: f.Seg.Pos.Line, Column: f.Seg.Pos.Column,
			Severity: SeverityInfo, Category: CategoryIdiomRewrite,
			RuleID: RuleNoBracketIndexing, SpecRef: "§14#10",
			Explanation: "V1 numeric path segment rewritten as V2 index expression",
		})
		return &syntax.IndexExpr{
			Receiver: recv,
			Index: &syntax.LiteralExpr{
				TokenPos:  pos(f.Seg.Pos),
				TokenType: syntax.INT,
				Value:     f.Seg.Name,
			},
			LBracketPos: pos(f.Seg.Pos),
			NullSafe:    true,
		}
	}
	// Flag field accesses whose receiver can't be statically guaranteed
	// to be an object. V1 returns null for field access on scalars and
	// arrays (§12.5); V2 errors. The `?.` NullSafe modifier catches null
	// but not wrong-type receivers — if the receiver's expected type
	// isn't object-ish, emit a SemanticChange so the divergence is
	// visible.
	if !objectLikeReceiver(recv) {
		t.rec.Rewritten(Change{
			Line: f.Seg.Pos.Line, Column: f.Seg.Pos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID: RuleStringLengthBytes, SpecRef: "§12.5",
			Explanation: "V1 path access on non-object returns null; V2 errors on wrong-type receivers (consider .catch(null))",
		})
	} else {
		t.rec.Exact()
	}
	return &syntax.FieldAccessExpr{
		Receiver: recv,
		Field:    f.Seg.Name,
		FieldPos: pos(f.Seg.Pos),
		NullSafe: true,
	}
}

// objectLikeReceiver returns true if the V2 receiver expression is guaranteed
// (or very likely) to evaluate to an object. We treat input/output roots and
// their chained field accesses as object-like — the common case where V1 and
// V2 agree. Variables, idents, method-call results, and index expressions are
// NOT object-guaranteed: V1 returns null for field access on scalars, V2
// errors, and without static type info we can't tell.
func objectLikeReceiver(e syntax.Expr) bool {
	switch r := e.(type) {
	case *syntax.InputExpr, *syntax.OutputExpr, *syntax.InputMetaExpr, *syntax.OutputMetaExpr:
		return true
	case *syntax.FieldAccessExpr:
		return objectLikeReceiver(r.Receiver)
	}
	return false
}

// isAllDigits returns true when s is a non-empty string of ASCII digits.
func isAllDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// translateMethodCall rewrites `recv.name(args)`. Some method names are
// renamed or reshape in V2; methodRewrite handles those. Others are 1:1.
func (t *translator) translateMethodCall(m *v1ast.MethodCall) syntax.Expr {
	recv := t.translateExpr(m.Recv)
	if recv == nil {
		return nil
	}
	if out := t.methodRewrite(m, recv); out != nil {
		return out
	}
	args := t.translateArgs(m.Args)
	t.rec.Exact()
	return &syntax.MethodCallExpr{
		Receiver:  recv,
		Method:    m.Name,
		MethodPos: pos(m.NamePos),
		Args:      args,
		Named:     m.Named,
	}
}

// translateFunctionCall rewrites top-level `name(args)` calls.
func (t *translator) translateFunctionCall(f *v1ast.FunctionCall) syntax.Expr {
	// V1 `nothing()` is a sentinel that produces the "skip assignment"
	// value. V2 expresses the same idea via the void type (no literal
	// spelling — produced by an if-without-else whose condition is false).
	// Rewrite `nothing()` to `if false { null }` so downstream assignments
	// are skipped as they were in V1.
	// V1 `parse_json` / `parse_yaml` return all numbers as float64; V2
	// distinguishes int64 and float64 based on the serialised form.
	// Downstream code that branches on .type() or compares types will
	// diverge.
	if f.Name == "parse_json" || f.Name == "parse_yaml" {
		t.rec.Note(Change{
			Line: f.NamePos.Line, Column: f.NamePos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§13",
			Explanation: "V1 " + f.Name + "() returns all numbers as float64; V2 distinguishes int64 and float64 by serialised form",
		})
	}
	// V1 `deleted()` as a top-level assignment marker has overlapping but
	// distinct V2 semantics: V2 rejects it in some positions where V1
	// silently accepted (e.g. variable assignment, comparison, method
	// receiver). Flag so divergences surface.
	if f.Name == "deleted" && len(f.Args) == 0 && !f.Named {
		t.rec.Note(Change{
			Line: f.NamePos.Line, Column: f.NamePos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§7.3",
			Explanation: "V1 deleted() is widely tolerated; V2 errors when deleted() appears in variable assignments, comparisons, or as a method receiver",
		})
	}
	if f.Name == "nothing" && len(f.Args) == 0 && !f.Named {
		t.rec.Rewritten(Change{
			Line: f.NamePos.Line, Column: f.NamePos.Column,
			Severity:    SeverityInfo,
			Category:    CategoryIdiomRewrite,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§14#36",
			Explanation: "V1 `nothing()` sentinel rewritten as V2 void-producing `if false { null }`",
		})
		return &syntax.IfExpr{
			TokenPos: pos(f.NamePos),
			Branches: []syntax.IfExprBranch{{
				Cond: &syntax.LiteralExpr{TokenPos: pos(f.NamePos), TokenType: syntax.FALSE, Value: "false"},
				Body: &syntax.ExprBody{Result: &syntax.LiteralExpr{TokenPos: pos(f.NamePos), TokenType: syntax.NULL, Value: "null"}},
			}},
		}
	}
	args := t.translateArgs(f.Args)
	t.rec.Exact()
	return &syntax.CallExpr{
		TokenPos: pos(f.NamePos),
		Name:     f.Name,
		Args:     args,
		Named:    f.Named,
	}
}

// translateMetaCall rewrites `meta(expr)` reads.
func (t *translator) translateMetaCall(m *v1ast.MetaCall) syntax.Expr {
	key := t.translateExpr(m.Key)
	if key == nil {
		return nil
	}
	t.rec.Rewritten(Change{
		Line: m.TokPos.Line, Column: m.TokPos.Column,
		Severity: SeverityInfo, Category: CategoryIdiomRewrite,
		RuleID:      RuleMetaReadToInputMeta,
		Explanation: "meta(expr) read rewritten as input@[expr]",
	})
	return &syntax.IndexExpr{
		Receiver:    &syntax.InputMetaExpr{TokenPos: pos(m.TokPos)},
		Index:       key,
		LBracketPos: pos(m.TokPos),
	}
}

// translateLambda rewrites `name -> body`. The parameter name is pushed onto
// the scope stack before translating the body so that identifier references
// to the param are resolved as named-context, not legacy bare-idents.
func (t *translator) translateLambda(l *v1ast.Lambda) syntax.Expr {
	paramName := l.Param
	if l.Discard {
		paramName = "_"
	}
	t.pushScope(paramName)
	body := t.translateExpr(l.Body)
	t.popScope()
	if body == nil {
		return nil
	}
	t.rec.Exact()
	return &syntax.LambdaExpr{
		TokenPos: pos(l.ParamPos),
		Params:   []syntax.Param{{Name: l.Param, Discard: l.Discard, Pos: pos(l.ParamPos), SlotIndex: -1}},
		Body:     &syntax.ExprBody{Result: body},
	}
}

// translateArrayLit rewrites `[elem, ...]`.
func (t *translator) translateArrayLit(a *v1ast.ArrayLit) syntax.Expr {
	out := &syntax.ArrayLiteral{LBracketPos: pos(a.TokPos)}
	for _, elem := range a.Elems {
		v := t.translateExpr(elem)
		if v != nil {
			out.Elements = append(out.Elements, v)
		}
	}
	t.rec.Exact()
	return out
}

// translateObjectLit rewrites `{key: value, ...}`.
func (t *translator) translateObjectLit(o *v1ast.ObjectLit) syntax.Expr {
	out := &syntax.ObjectLiteral{LBracePos: pos(o.TokPos)}
	for _, entry := range o.Entries {
		key := t.translateExpr(entry.Key)
		value := t.translateExpr(entry.Value)
		if key == nil || value == nil {
			continue
		}
		out.Entries = append(out.Entries, syntax.ObjectEntry{Key: key, Value: value})
	}
	t.rec.Exact()
	return out
}

// translateIfExpr rewrites `if/else if/else` expression form.
func (t *translator) translateIfExpr(i *v1ast.IfExpr) syntax.Expr {
	out := &syntax.IfExpr{TokenPos: pos(i.TokPos)}
	for _, br := range i.Branches {
		cond := t.translateExpr(br.Cond)
		body := t.translateExpr(br.Body)
		if cond == nil || body == nil {
			continue
		}
		out.Branches = append(out.Branches, syntax.IfExprBranch{Cond: cond, Body: &syntax.ExprBody{Result: body}})
	}
	if i.Else != nil {
		if body := t.translateExpr(i.Else); body != nil {
			out.Else = &syntax.ExprBody{Result: body}
		}
	} else {
		t.rec.Rewritten(Change{
			Line: i.TokPos.Line, Column: i.TokPos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID: RuleIfNoElseNothing, SpecRef: "§14#44",
			Explanation: "V1 if-without-else produces nothing sentinel; V2 behaviour may differ",
		})
	}
	t.rec.Exact()
	return out
}

// translateMatchExpr rewrites `match [subject] { cases }`.
func (t *translator) translateMatchExpr(m *v1ast.MatchExpr) syntax.Expr {
	out := &syntax.MatchExpr{TokenPos: pos(m.TokPos), BindingSlot: -1}
	if m.Subject != nil {
		out.Subject = t.translateExpr(m.Subject)
	}
	for _, c := range m.Cases {
		if lit, ok := c.Pattern.(*v1ast.Literal); ok && lit.Kind == v1ast.LitBool {
			t.rec.Note(Change{
				Line: lit.TokPos.Line, Column: lit.TokPos.Column,
				Severity:    SeverityWarning,
				Category:    CategorySemanticChange,
				RuleID:      RuleMethodDoesNotExist,
				SpecRef:     "§8",
				Explanation: "V1 allows a boolean literal as a match case pattern (equality match); V2 rejects this — rewrite using `as` binding or an explicit boolean condition.",
			})
		}
		mc := syntax.MatchCase{Wildcard: c.Wildcard}
		if c.Pattern != nil {
			mc.Pattern = t.translateExpr(c.Pattern)
		}
		if body := t.translateExpr(c.Body); body != nil {
			mc.Body = body
		} else {
			continue
		}
		out.Cases = append(out.Cases, mc)
	}
	t.rec.Exact()
	return out
}

// translateMapExpr rewrites the path-scoped `recv.(expr)` form.
func (t *translator) translateMapExpr(m *v1ast.MapExpr) syntax.Expr {
	// V2 has no `.(expr)` map-scoped form. Rewrite as a call to `.apply` on
	// the receiver — not semantically identical (apply is by-map-name) but
	// the closest V2 equivalent is an explicit let+body rewrite which we
	// don't yet support.
	t.rec.Unsupported(Change{
		Line: m.TokPos.Line, Column: m.TokPos.Column,
		RuleID:      RuleUnsupportedConstruct,
		SpecRef:     "§5.4",
		Explanation: "V1 .(expr) map-scoped subexpression has no direct V2 equivalent",
	})
	return nil
}

// translateArgs translates call arguments.
func (t *translator) translateArgs(args []v1ast.CallArg) []syntax.CallArg {
	out := make([]syntax.CallArg, 0, len(args))
	for _, a := range args {
		v := t.translateExpr(a.Value)
		if v == nil {
			continue
		}
		out = append(out, syntax.CallArg{Name: a.Name, Value: v})
	}
	return out
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

// pos converts a V1 position to a V2 position. Same structure, different
// package.
func pos(p v1ast.Pos) syntax.Pos {
	return syntax.Pos{Line: p.Line, Column: p.Column}
}

// flagOperatorDivergence records SemanticChange Notes for V1 binary operators
// whose V2 behaviour differs on non-trivial operands. These are fire-
// unconditionally diagnostics — V2 is stricter than V1 about types, so any
// arithmetic/logical op that reaches non-primitive operands at runtime may
// diverge. We record the divergence per operator kind (skip comparison and
// equality, which are stricter in V2 but already flagged elsewhere).
func (t *translator) flagOperatorDivergence(b *v1ast.BinaryExpr) {
	switch b.Op {
	case v1ast.TokAnd, v1ast.TokOr:
		t.rec.Note(Change{
			Line: b.OpPos.Line, Column: b.OpPos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleAndOrSameLevel,
			SpecRef:     "§14#6",
			Explanation: "V1 &&/|| coerce non-boolean operands; V2 requires boolean operands and errors otherwise",
		})
	case v1ast.TokPlus:
		t.rec.Note(Change{
			Line: b.OpPos.Line, Column: b.OpPos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§14#41",
			Explanation: "V1 + concatenates bytes-to-string and string-to-bytes; V2 is type-strict",
		})
	case v1ast.TokPercent:
		t.rec.Note(Change{
			Line: b.OpPos.Line, Column: b.OpPos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleModuloFloatTruncation,
			SpecRef:     "§14#39",
			Explanation: "V1 % silently truncates float operands to int64 before mod; V2 uses fmod and preserves float64",
		})
	}
}

// mapV1BinaryOp maps a V1 binary operator token kind to its V2 TokenType
// equivalent. Returns false for kinds that don't have a direct V2 mapping
// (e.g. the `|` coalesce, which is handled specially in translateBinary).
func mapV1BinaryOp(k v1ast.TokenKind) (syntax.TokenType, bool) {
	switch k {
	case v1ast.TokPlus:
		return syntax.PLUS, true
	case v1ast.TokMinus:
		return syntax.MINUS, true
	case v1ast.TokStar:
		return syntax.STAR, true
	case v1ast.TokSlash:
		return syntax.SLASH, true
	case v1ast.TokPercent:
		return syntax.PERCENT, true
	case v1ast.TokEq:
		return syntax.EQ, true
	case v1ast.TokNeq:
		return syntax.NE, true
	case v1ast.TokLt:
		return syntax.LT, true
	case v1ast.TokLte:
		return syntax.LE, true
	case v1ast.TokGt:
		return syntax.GT, true
	case v1ast.TokGte:
		return syntax.GE, true
	case v1ast.TokAnd:
		return syntax.AND, true
	case v1ast.TokOr:
		return syntax.OR, true
	}
	return 0, false
}

// mapV1UnaryOp maps V1 unary tokens (! and -) to V2.
func mapV1UnaryOp(k v1ast.TokenKind) (syntax.TokenType, bool) {
	switch k {
	case v1ast.TokBang:
		return syntax.BANG, true
	case v1ast.TokMinus:
		return syntax.MINUS, true
	}
	return 0, false
}
