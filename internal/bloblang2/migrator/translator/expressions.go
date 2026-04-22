package translator

import (
	"fmt"
	"strconv"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

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
		// `this.foo` = V2 `input.foo`. V2 errors if the field is absent
		// or the receiver isn't an object — V1 silently returned null.
		// Emit as NullSafe so the V2 form tolerates a null/absent
		// receiver the way V1 did, and flag as a SemanticChange so the
		// wider divergence (V2 is type-strict on non-object receivers)
		// surfaces on the Report.
		t.rec.Rewritten(Change{
			Line: x.TokPos.Line, Column: x.TokPos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID: RuleBareIdentToInput, SpecRef: "§14#1",
			Explanation: fmt.Sprintf(`bare identifier %q rewritten as "input.%s"; V2 errors on absent fields where V1 returned null`, x.Name, x.Name),
		})
		return &syntax.FieldAccessExpr{
			Receiver: &syntax.InputExpr{TokenPos: pos(x.TokPos)},
			Field:    x.Name,
			FieldPos: pos(x.TokPos),
			NullSafe: true,
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
			SpecRef:     "§14#48",
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
	case v1ast.TokSlash:
		t.rec.Note(Change{
			Line: b.OpPos.Line, Column: b.OpPos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleIntDivReturnsFloat,
			SpecRef:     "§14#5",
			Explanation: "V1 / on int operands returns float64; V2 preserves integer division when both operands are int",
		})
	case v1ast.TokStar, v1ast.TokMinus:
		t.rec.Note(Change{
			Line: b.OpPos.Line, Column: b.OpPos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§14#26",
			Explanation: "V1 arithmetic silently wraps on int64 overflow and coerces across numeric types; V2 errors on overflow and on integers outside the float64 exact range (2^53)",
		})
	case v1ast.TokPercent:
		t.rec.Note(Change{
			Line: b.OpPos.Line, Column: b.OpPos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleModuloFloatTruncation,
			SpecRef:     "§14#39",
			Explanation: "V1 % silently truncates float operands to int64 before mod; V2 uses fmod and preserves float64",
		})
	case v1ast.TokEq, v1ast.TokNeq:
		t.rec.Note(Change{
			Line: b.OpPos.Line, Column: b.OpPos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleBoolNumberEquality,
			SpecRef:     "§14#38",
			Explanation: "V1 ==/!= coerces across types (bool==1 is true, string==bytes compares bytes); V2 requires matching types",
		})
	case v1ast.TokLt, v1ast.TokLte, v1ast.TokGt, v1ast.TokGte:
		t.rec.Note(Change{
			Line: b.OpPos.Line, Column: b.OpPos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 <, <=, >, >= accept some cross-type operands and perform coercion; V2 errors unless both operands are numeric/string/bytes of the same family",
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
		// V2 rejects out-of-bounds array indices at runtime where V1
		// returned null; flag so such tests surface as known divergences.
		t.rec.Note(Change{
			Line: f.Seg.Pos.Line, Column: f.Seg.Pos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleNoBracketIndexing,
			Explanation: "V1 numeric path access on arrays tolerates out-of-bounds (returns null); V2 errors",
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
	// V1 `now()` returns a string (RFC3339Nano); V2 returns a typed
	// timestamp. Downstream comparisons and formatting differ.
	if f.Name == "now" && len(f.Args) == 0 && !f.Named {
		t.rec.Note(Change{
			Line: f.NamePos.Line, Column: f.NamePos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleNowReturnsString,
			SpecRef:     "§14#57",
			Explanation: "V1 now() returns a string; V2 returns a typed timestamp",
		})
	}
	// V1 `range(a, b, step)` with a descending range and explicit step
	// includes one additional element compared with V2 (boundary
	// arithmetic differs).
	if f.Name == "range" {
		t.rec.Note(Change{
			Line: f.NamePos.Line, Column: f.NamePos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 range(a, b, step) boundary arithmetic differs from V2 on descending ranges — audit array length",
		})
	}
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
	// V1 `nothing()` is a sentinel that means different things in
	// different positions. V2 split the concepts: `void()` for
	// "skip this assignment", `deleted()` for "omit from this
	// collection". The translator disambiguates by looking at the
	// current rendering context (see ctxKind) and emitting the V2
	// form that matches V1's intent at each site.
	if f.Name == "nothing" && len(f.Args) == 0 && !f.Named {
		switch t.currentCtx() {
		case ctxCollectionLit:
			t.rec.Rewritten(Change{
				Line: f.NamePos.Line, Column: f.NamePos.Column,
				Severity:    SeverityInfo,
				Category:    CategoryIdiomRewrite,
				RuleID:      RuleMethodDoesNotExist,
				SpecRef:     "§14#71",
				Explanation: "V1 `nothing()` inside a collection literal rewritten as V2 `deleted()` (both elide the entry)",
			})
			return &syntax.CallExpr{
				TokenPos: pos(f.NamePos),
				Name:     "deleted",
			}
		case ctxVarDeclRHS:
			// V1 `let $x = nothing()` deletes $x. V2 errors on
			// void in a variable declaration and has no equivalent
			// delete-a-variable construct. Emit `void()` so the V2
			// runtime fires the documented error at the right site,
			// and flag Error-severity so the migrator user sees that
			// manual rewrite is required.
			t.rec.Unsupported(Change{
				Line: f.NamePos.Line, Column: f.NamePos.Column,
				RuleID:      RuleUnsupportedConstruct,
				SpecRef:     "§14#17",
				Explanation: "V1 `let $x = nothing()` deletes the variable; V2 has no equivalent — emitted `void()` which will error at runtime. Rewrite this `let` by hand.",
			})
			return &syntax.CallExpr{
				TokenPos: pos(f.NamePos),
				Name:     "void",
			}
		}
		t.rec.Rewritten(Change{
			Line: f.NamePos.Line, Column: f.NamePos.Column,
			Severity:    SeverityInfo,
			Category:    CategoryIdiomRewrite,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§14#36",
			Explanation: "V1 `nothing()` sentinel rewritten as V2 `void()`",
		})
		return &syntax.CallExpr{
			TokenPos: pos(f.NamePos),
			Name:     "void",
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

// translateArrayLit rewrites `[elem, ...]`. Pushes ctxCollectionLit
// while translating each element so nested `nothing()` calls lower to
// V2 `deleted()` (which elides from the array, matching V1) rather
// than V2 `void()` (which would error in array-literal position).
func (t *translator) translateArrayLit(a *v1ast.ArrayLit) syntax.Expr {
	out := &syntax.ArrayLiteral{LBracketPos: pos(a.TokPos)}
	for _, elem := range a.Elems {
		t.pushCtx(ctxCollectionLit)
		v := t.translateExpr(elem)
		t.popCtx()
		if v != nil {
			out.Elements = append(out.Elements, v)
		}
	}
	t.rec.Exact()
	return out
}

// translateObjectLit rewrites `{key: value, ...}`. Pushes ctxCollectionLit
// while translating each entry's value for the same reason arrays do.
// Keys are translated without the context — a sentinel as an object key
// would be malformed in either language.
func (t *translator) translateObjectLit(o *v1ast.ObjectLit) syntax.Expr {
	out := &syntax.ObjectLiteral{LBracePos: pos(o.TokPos)}
	for _, entry := range o.Entries {
		key := t.translateExpr(entry.Key)
		t.pushCtx(ctxCollectionLit)
		value := t.translateExpr(entry.Value)
		t.popCtx()
		if key == nil || value == nil {
			continue
		}
		out.Entries = append(out.Entries, syntax.ObjectEntry{Key: key, Value: value})
	}
	t.rec.Exact()
	return out
}

// translateIfExpr rewrites `if/else if/else` expression form.
//
// V1 without `else` produces a `nothing` sentinel, which silently elided
// from collection literals and skipped assignments. V2 produces void,
// which errors in collection literals. When we're translating an
// if-without-else inside a collection-literal context, synthesize an
// explicit `else { deleted() }` so the resulting V2 expression elides
// the entry rather than erroring.
func (t *translator) translateIfExpr(i *v1ast.IfExpr) syntax.Expr {
	out := &syntax.IfExpr{TokenPos: pos(i.TokPos)}
	for _, br := range i.Branches {
		t.flagNonBoolCond(br.Cond, i.TokPos)
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
	} else if t.currentCtx() == ctxCollectionLit {
		t.rec.Rewritten(Change{
			Line: i.TokPos.Line, Column: i.TokPos.Column,
			Severity: SeverityInfo, Category: CategoryIdiomRewrite,
			RuleID: RuleIfNoElseNothing, SpecRef: "§14#71",
			Explanation: "V1 if-without-else inside a collection literal elides the entry; V2 errors — synthesized `else { deleted() }` to preserve the elision",
		})
		out.Else = &syntax.ExprBody{Result: &syntax.CallExpr{
			TokenPos: pos(i.TokPos),
			Name:     "deleted",
		}}
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

// flagNonBoolCond emits a SemanticChange note when a V1 if condition is a
// literal `null` — V1 treats null as falsy while V2 errors on non-bool
// conditions. Broader analysis (variables, method calls) isn't feasible
// without type inference; this covers the obvious static cases and leaves
// runtime-only divergences to be caught by the general bool-strictness flag.
func (t *translator) flagNonBoolCond(cond v1ast.Expr, tokPos v1ast.Pos) {
	lit, ok := cond.(*v1ast.Literal)
	if !ok {
		return
	}
	if lit.Kind == v1ast.LitBool {
		return
	}
	t.rec.Note(Change{
		Line: tokPos.Line, Column: tokPos.Column,
		Severity: SeverityInfo, Category: CategorySemanticChange,
		RuleID:      RuleAndOrSameLevel,
		Explanation: "V1 accepts non-bool if-conditions (null is falsy; int/string error); V2 requires a boolean condition",
	})
}

// translateMatchExpr rewrites `match [subject] { cases }`.
func (t *translator) translateMatchExpr(m *v1ast.MatchExpr) syntax.Expr {
	out := &syntax.MatchExpr{TokenPos: pos(m.TokPos), BindingSlot: -1}
	if m.Subject != nil {
		out.Subject = t.translateExpr(m.Subject)
	} else {
		// Subject-less match (V1 boolean-case form). V2 requires each
		// case pattern to evaluate to bool; V1 coerced non-bool patterns
		// (int/string/null) silently. Flag so runtime divergences surface.
		t.rec.Note(Change{
			Line: m.TokPos.Line, Column: m.TokPos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleMatchSubjectRebinds,
			SpecRef:     "§8",
			Explanation: "V1 boolean-case match coerces non-boolean case patterns; V2 errors when a case doesn't evaluate to bool",
		})
	}
	hasWildcard := false
	for _, c := range m.Cases {
		if c.Wildcard {
			hasWildcard = true
		}
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
	// A V1 match without a wildcard that produces void inside a
	// collection literal would elide the entry; V2 errors. Synthesize
	// `_ => deleted()` so the elision carries over.
	if !hasWildcard && t.currentCtx() == ctxCollectionLit {
		t.rec.Rewritten(Change{
			Line: m.TokPos.Line, Column: m.TokPos.Column,
			Severity: SeverityInfo, Category: CategoryIdiomRewrite,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§14#71",
			Explanation: "V1 match-without-wildcard inside a collection literal elides the entry on no match; V2 errors — synthesized `_ => deleted()` to preserve the elision",
		})
		out.Cases = append(out.Cases, syntax.MatchCase{
			Wildcard: true,
			Body: &syntax.CallExpr{
				TokenPos: pos(m.TokPos),
				Name:     "deleted",
			},
		})
	}
	t.rec.Exact()
	return out
}

// translateMapExpr rewrites the path-scoped `recv.(expr)` form.
//
// V1 has two shapes:
//   - `recv.(name -> body)` — bind name to recv, `this` unchanged.
//   - `recv.(body)`          — rebind `this` to recv inside body.
//
// V2's .into(lambda) method (§13.12) maps cleanly onto the named form:
// `recv.(name -> body)` → `recv.into(name -> body)`. The un-named form
// rebinds `this`, which V2 lambdas don't do directly — we synthesize a
// named param and rewrite references to `this` inside the body to that
// name (handled by pushThisRebind during translation of the body).
func (t *translator) translateMapExpr(m *v1ast.MapExpr) syntax.Expr {
	recv := t.translateExpr(m.Recv)
	if recv == nil {
		return nil
	}
	if lambda, ok := m.Body.(*v1ast.Lambda); ok {
		translatedLambda := t.translateLambda(lambda)
		if translatedLambda == nil {
			return nil
		}
		t.rec.Rewritten(Change{
			Line: m.TokPos.Line, Column: m.TokPos.Column,
			Severity: SeverityInfo, Category: CategoryIdiomRewrite,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§5.4",
			Explanation: "V1 recv.(name -> body) rewritten as V2 recv.into(name -> body)",
		})
		return &syntax.MethodCallExpr{
			Receiver:  recv,
			Method:    "into",
			MethodPos: pos(m.TokPos),
			Args:      []syntax.CallArg{{Value: translatedLambda}},
		}
	}
	// Un-named form: synthesize a lambda that rebinds `this` to a fresh
	// name while the body is translated, then wrap as .into($name -> body).
	const paramName = "__this"
	t.pushScope(paramName)
	t.pushThisRebind(paramName)
	body := t.translateExpr(m.Body)
	t.popThisRebind()
	t.popScope()
	if body == nil {
		return nil
	}
	t.rec.Rewritten(Change{
		Line: m.TokPos.Line, Column: m.TokPos.Column,
		Severity: SeverityInfo, Category: CategoryIdiomRewrite,
		RuleID:      RuleMethodDoesNotExist,
		SpecRef:     "§5.4",
		Explanation: "V1 recv.(body) (un-named, this-rebinding) rewritten as V2 recv.into(__this -> body) with `this` references replaced",
	})
	return &syntax.MethodCallExpr{
		Receiver:  recv,
		Method:    "into",
		MethodPos: pos(m.TokPos),
		Args: []syntax.CallArg{{Value: &syntax.LambdaExpr{
			TokenPos: pos(m.TokPos),
			Params:   []syntax.Param{{Name: paramName, Pos: pos(m.TokPos), SlotIndex: -1}},
			Body:     &syntax.ExprBody{Result: body},
		}}},
	}
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
