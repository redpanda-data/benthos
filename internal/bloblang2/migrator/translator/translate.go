package translator

import (
	"fmt"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

// translator holds the per-call state: the Change/Coverage recorder and a
// handful of helpers.
type translator struct {
	rec *recorder
}

// translateProgram walks a parsed V1 program and produces a V2 program. Every
// V1 node contributes to Coverage via recorder calls.
func (t *translator) translateProgram(p *v1ast.Program) *syntax.Program {
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
		cond := t.translateExpr(br.Cond)
		body := t.translateStmts(br.Body)
		if cond == nil {
			continue
		}
		out.Branches = append(out.Branches, syntax.IfBranch{Cond: cond, Body: body})
	}
	if i.Else != nil {
		out.Else = t.translateStmts(i.Else)
	}
	return out
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
// V1 map bodies are statement lists producing a `root` value. V2 map bodies
// are ExprBody: zero or more variable assignments followed by a single result
// expression. The translator emits a best-effort ExprBody by:
//  1. collecting all V1 `let` statements as var assignments,
//  2. using the final `root = expr` as the result expression (if present).
//
// This is not a perfect translation — V1 maps with multiple `root.x = …`
// assignments cannot be represented as a single-result ExprBody without
// constructing an object literal. We flag those cases as unsupported until a
// dedicated rule is written.
func (t *translator) translateMapDecl(m *v1ast.MapDecl) *syntax.MapDecl {
	t.rec.Rewritten(Change{
		Line: m.Pos.Line, Column: m.Pos.Column,
		Severity:    SeverityWarning,
		Category:    CategoryUnsupported,
		RuleID:      RuleMapDeclTranslation,
		Explanation: "map declaration translation is not yet implemented; body emitted as placeholder",
	})
	// TODO: proper translation of V1 map bodies into V2 ExprBody.
	// For now, emit a stub ExprBody with a null result so the V2 printer
	// can at least serialise the declaration.
	return &syntax.MapDecl{
		TokenPos: pos(m.Pos),
		Name:     m.Name,
		Body: &syntax.ExprBody{
			Result: &syntax.LiteralExpr{
				TokenPos:  pos(m.Pos),
				TokenType: syntax.NULL,
				Value:     "null",
			},
		},
	}
}

// translateImport translates `import "path"` to V2.
func (t *translator) translateImport(i *v1ast.ImportStmt) *syntax.ImportStmt {
	// V1 imports have no namespace alias; V2 requires `as name` unless
	// importing anonymously. Emit a warning — the caller must choose a
	// namespace or flatten the maps.
	lit, ok := i.Path.(*v1ast.Literal)
	if !ok {
		t.rec.Unsupported(Change{
			Line: i.Pos.Line, Column: i.Pos.Column,
			RuleID:      RuleImportStatement,
			Explanation: "import path is not a string literal",
		})
		return nil
	}
	t.rec.Rewritten(Change{
		Line: i.Pos.Line, Column: i.Pos.Column,
		Severity:    SeverityWarning,
		Category:    CategorySemanticChange,
		RuleID:      RuleImportStatement,
		Explanation: `V1 import has no namespace; V2 requires "as name" — chose a default name`,
	})
	return &syntax.ImportStmt{
		TokenPos:  pos(i.Pos),
		Path:      lit.Str,
		Namespace: "imported",
	}
}

// translateExpr dispatches expression translation.
func (t *translator) translateExpr(e v1ast.Expr) syntax.Expr {
	switch x := e.(type) {
	case *v1ast.Literal:
		return t.translateLiteral(x)
	case *v1ast.ThisExpr:
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
		// Legacy bare identifier in expression position = `this.foo` = V2
		// `input.foo`.
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
		out.Value = fmt.Sprintf("%d", l.Int)
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
func (t *translator) translateFieldAccess(f *v1ast.FieldAccess) syntax.Expr {
	recv := t.translateExpr(f.Recv)
	if recv == nil {
		return nil
	}
	t.rec.Exact()
	return &syntax.FieldAccessExpr{
		Receiver: recv,
		Field:    f.Seg.Name,
		FieldPos: pos(f.Seg.Pos),
	}
}

// translateMethodCall rewrites `recv.name(args)`.
func (t *translator) translateMethodCall(m *v1ast.MethodCall) syntax.Expr {
	recv := t.translateExpr(m.Recv)
	if recv == nil {
		return nil
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
		RuleID: RuleMetaReadToInputMeta,
		Explanation: "meta(expr) read rewritten as input@[expr]",
	})
	return &syntax.IndexExpr{
		Receiver:    &syntax.InputMetaExpr{TokenPos: pos(m.TokPos)},
		Index:       key,
		LBracketPos: pos(m.TokPos),
	}
}

// translateLambda rewrites `name -> body`.
func (t *translator) translateLambda(l *v1ast.Lambda) syntax.Expr {
	body := t.translateExpr(l.Body)
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
