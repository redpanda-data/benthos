package translator

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

// methodRewrite applies V1 → V2 method-shape translations on a V1 MethodCall.
// Returns a non-nil V2 expression on success, or nil to signal "fall through
// to the default 1:1 translation".
//
// Rules are ordered by the V1 method name; each rule may:
//   - rename the method (e.g. map_each -> map),
//   - convert the method call to a different V2 node shape (e.g. index -> []),
//   - leave it alone (default).
func (t *translator) methodRewrite(m *v1ast.MethodCall, recv syntax.Expr) syntax.Expr {
	switch m.Name {

	// ----- Simple renames (V2 name differs, same shape) -----
	case "map_each":
		// V1 .map_each accepts arrays and objects; V2 splits that: `.map`
		// for arrays and `.map_values` for objects. Detect object-literal
		// receivers at translate time; everything else defaults to `.map`
		// with a SemanticChange flag so object-receiver cases surface in
		// the Report.
		if _, isObj := m.Recv.(*v1ast.ObjectLit); isObj {
			return t.simpleRename(m, recv, "map_values")
		}
		return t.rewrittenRename(m, recv, "map", Change{
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 .map_each() accepts arrays and objects; V2 .map() is array-only — use .map_values() if the receiver is an object",
		})
	case "enumerated":
		return t.simpleRename(m, recv, "enumerate")
	case "key_values":
		return t.simpleRename(m, recv, "iter")
	case "map_each_key":
		// V1 .map_each_key == V2 .map_keys (exact match — both take lambda).
		return t.simpleRename(m, recv, "map_keys")
	case "assign":
		// V1 assign is deep-merge; V2 merge is the equivalent (V2 has no
		// separate assign method). See Quirk #50 for V1 naming confusion.
		return t.rewrittenRename(m, recv, "merge",
			Change{
				RuleID:      RuleMethodDoesNotExist,
				Severity:    SeverityInfo,
				Category:    CategoryIdiomRewrite,
				Explanation: "V1 .assign() renamed to V2 .merge()",
			})

	// ----- Array indexing: .index(n) -> [n] -----
	case "index":
		return t.indexToBracket(m, recv)

	// ----- Dynamic key access: .get(k) -> [k] -----
	case "get":
		return t.indexToBracket(m, recv)

	// ----- Apply: recv.apply("name") -> name(recv) -----
	case "apply":
		return t.applyToCall(m, recv)

	// ----- Numeric coercion: V1 .number() -> V2 .float64() -----
	case "number":
		return t.rewrittenRename(m, recv, "float64",
			Change{
				RuleID:      RuleMethodDoesNotExist,
				Severity:    SeverityWarning,
				Category:    CategorySemanticChange,
				Explanation: "V1 .number() is float64; V2 .float64() preserves that, but downstream code expecting int64 results may break",
			})

	// ----- Variadic .without("a","b","c") -> .without(["a","b","c"]) -----
	case "without":
		return t.withoutVariadicToArray(m, recv)

	// ----- .find(value) -> .find(x -> x == value) -----
	case "find":
		return t.findValueToLambda(m, recv)

	// ----- .exists(path) -> (path != null).catch(false) -----
	case "exists":
		return t.existsToNullCheck(m, recv)

	// ----- V2 .catch requires a lambda; V1 accepts a plain value -----
	case "catch":
		return t.catchValueToLambda(m, recv)

	// ----- Flag known semantic divergences without rewriting -----
	case "length":
		t.flagMethodDivergence(m, "V1 .length() on strings counts bytes; V2 counts codepoints (§14#40)")
		return nil
	case "or":
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID: RuleOrCatchesErrors, SpecRef: "§12.2",
			Explanation: "V1 .or() catches errors AND nulls; V2 .or() catches nulls only — consider .catch(_ -> x) for error fallbacks",
		})
		return nil

	// ----- V2 is stricter about receiver type; flag and pass through -----
	case "merge":
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID: RuleMethodDoesNotExist, SpecRef: "§14#50",
			Explanation: "V1 .merge() tolerates null receiver/arg; V2 errors. Audit whether operands can be null.",
		})
		return nil
	case "filter", "filter_entries", "all", "any":
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID: RuleMethodDoesNotExist,
			Explanation: "V1 " + "." + m.Name + "() accepts arrays and objects; V2 is strict about receiver type",
		})
		return nil
	case "sum", "min", "max":
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID: RuleMethodDoesNotExist,
			Explanation: "V1 " + "." + m.Name + "() always returns float64; V2 may preserve integer type — audit numeric comparisons",
		})
		return nil
	}
	return nil
}

// catchValueToLambda wraps V1 `.catch(value)` as V2 `.catch(_ -> value)`.
// V2's .catch takes a lambda receiving the error; V1 accepts either a value
// or a lambda. We wrap plain values unconditionally — if the V1 argument was
// already a lambda the wrap is redundant but harmless.
func (t *translator) catchValueToLambda(m *v1ast.MethodCall, recv syntax.Expr) syntax.Expr {
	if len(m.Args) != 1 {
		return nil
	}
	arg := m.Args[0].Value
	// If already a V1 lambda, translate it 1:1 — no wrap needed.
	if _, isLambda := arg.(*v1ast.Lambda); isLambda {
		return nil
	}
	value := t.translateExpr(arg)
	if value == nil {
		return nil
	}
	t.rec.Rewritten(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityInfo, Category: CategoryIdiomRewrite,
		RuleID:      RuleOrCatchesErrors,
		SpecRef:     "§12.2",
		Explanation: "V1 .catch(value) wrapped in lambda for V2: .catch(_ -> value)",
	})
	wrapped := &syntax.LambdaExpr{
		TokenPos: pos(m.NamePos),
		Params:   []syntax.Param{{Discard: true, Pos: pos(m.NamePos), SlotIndex: -1}},
		Body:     &syntax.ExprBody{Result: value},
	}
	return &syntax.MethodCallExpr{
		Receiver:  recv,
		Method:    "catch",
		MethodPos: pos(m.NamePos),
		Args:      []syntax.CallArg{{Value: wrapped}},
	}
}

// simpleRename emits a V2 MethodCallExpr with a different method name, all
// other fields identical. Counts as Exact coverage.
func (t *translator) simpleRename(m *v1ast.MethodCall, recv syntax.Expr, newName string) syntax.Expr {
	args := t.translateArgs(m.Args)
	t.rec.Exact()
	return &syntax.MethodCallExpr{
		Receiver:  recv,
		Method:    newName,
		MethodPos: pos(m.NamePos),
		Args:      args,
		Named:     m.Named,
	}
}

// flagMethodDivergence emits a SemanticChange Change without rewriting the
// method call itself. Useful for methods where V1 and V2 names match but
// behaviour legitimately differs — the migrator can't always tell at
// translate time whether the divergence applies, so warn unconditionally
// and let the caller audit.
func (t *translator) flagMethodDivergence(m *v1ast.MethodCall, reason string) {
	t.rec.Rewritten(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityWarning, Category: CategorySemanticChange,
		RuleID: RuleStringLengthBytes, SpecRef: "§14#40",
		Explanation: reason,
	})
}

// rewrittenRename is simpleRename but emits a Change record describing the
// rewrite.
func (t *translator) rewrittenRename(m *v1ast.MethodCall, recv syntax.Expr, newName string, ch Change) syntax.Expr {
	args := t.translateArgs(m.Args)
	ch.Line = m.NamePos.Line
	ch.Column = m.NamePos.Column
	t.rec.Rewritten(ch)
	return &syntax.MethodCallExpr{
		Receiver:  recv,
		Method:    newName,
		MethodPos: pos(m.NamePos),
		Args:      args,
		Named:     m.Named,
	}
}

// indexToBracket translates `recv.index(n)` or `recv.get(k)` into V2's
// bracket indexing: recv[n] / recv[k]. Counts as Rewritten (idiom shift).
func (t *translator) indexToBracket(m *v1ast.MethodCall, recv syntax.Expr) syntax.Expr {
	if len(m.Args) != 1 {
		return nil
	}
	idx := t.translateExpr(m.Args[0].Value)
	if idx == nil {
		return nil
	}
	t.rec.Rewritten(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityInfo, Category: CategoryIdiomRewrite,
		RuleID:      RuleNoBracketIndexing,
		SpecRef:     "§14#10",
		Explanation: "V1 ." + m.Name + "() rewritten as V2 [] indexing",
	})
	return &syntax.IndexExpr{
		Receiver:    recv,
		Index:       idx,
		LBracketPos: pos(m.NamePos),
	}
}

// withoutVariadicToArray rewrites V1 `.without("a", "b", "c")` (variadic) to
// V2 `.without(["a", "b", "c"])` (single array argument). V2 rejects the
// variadic form at compile time.
func (t *translator) withoutVariadicToArray(m *v1ast.MethodCall, recv syntax.Expr) syntax.Expr {
	// If there's already a single array argument, pass through.
	if len(m.Args) == 1 {
		if _, ok := m.Args[0].Value.(*v1ast.ArrayLit); ok {
			return t.simpleRename(m, recv, "without")
		}
	}
	elems := make([]syntax.Expr, 0, len(m.Args))
	for _, a := range m.Args {
		v := t.translateExpr(a.Value)
		if v == nil {
			continue
		}
		elems = append(elems, v)
	}
	t.rec.Rewritten(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityInfo, Category: CategoryIdiomRewrite,
		RuleID:      RuleMethodDoesNotExist,
		Explanation: "V1 variadic .without(...) rewritten as V2 .without([...])",
	})
	return &syntax.MethodCallExpr{
		Receiver:  recv,
		Method:    "without",
		MethodPos: pos(m.NamePos),
		Args: []syntax.CallArg{{
			Value: &syntax.ArrayLiteral{LBracketPos: pos(m.NamePos), Elements: elems},
		}},
	}
}

// findValueToLambda rewrites V1 `.find(value)` (searches for literal value)
// to V2 `.find(x -> x == value)` (predicate form).
func (t *translator) findValueToLambda(m *v1ast.MethodCall, recv syntax.Expr) syntax.Expr {
	if len(m.Args) != 1 {
		return nil
	}
	needle := t.translateExpr(m.Args[0].Value)
	if needle == nil {
		return nil
	}
	// Build the predicate: x -> x == needle.
	paramName := "x"
	predicate := &syntax.LambdaExpr{
		TokenPos: pos(m.NamePos),
		Params:   []syntax.Param{{Name: paramName, Pos: pos(m.NamePos), SlotIndex: -1}},
		Body: &syntax.ExprBody{
			Result: &syntax.BinaryExpr{
				Left: &syntax.IdentExpr{
					TokenPos:  pos(m.NamePos),
					Name:      paramName,
					SlotIndex: -1,
				},
				Op:    syntax.EQ,
				OpPos: pos(m.NamePos),
				Right: needle,
			},
		},
	}
	t.rec.Rewritten(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityInfo, Category: CategoryIdiomRewrite,
		RuleID:      RuleMethodDoesNotExist,
		Explanation: "V1 .find(value) rewritten as V2 .find(x -> x == value)",
	})
	return &syntax.MethodCallExpr{
		Receiver:  recv,
		Method:    "find",
		MethodPos: pos(m.NamePos),
		Args:      []syntax.CallArg{{Value: predicate}},
	}
}

// existsToNullCheck rewrites V1 `.exists()` into V2. V1 has two shapes:
//
//   - `.exists(key)` on an object: checks for key presence -> V2 `.has_key(key)`.
//   - `.exists()` on a value: non-null check -> V2 `(recv != null).catch(false)`.
func (t *translator) existsToNullCheck(m *v1ast.MethodCall, recv syntax.Expr) syntax.Expr {
	// One-arg form is `has_key` on V2.
	if len(m.Args) == 1 {
		return t.rewrittenRename(m, recv, "has_key",
			Change{
				RuleID:      RuleMethodDoesNotExist,
				Severity:    SeverityInfo,
				Category:    CategoryIdiomRewrite,
				Explanation: "V1 .exists(key) rewritten as V2 .has_key(key)",
			})
	}
	if len(m.Args) != 0 {
		t.rec.Unsupported(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 .exists() with more than one arg has no V2 rewrite",
		})
		return nil
	}
	// Zero-arg form: recv != null, caught to false for non-null receivers
	// with unreadable types.
	t.rec.Rewritten(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityWarning, Category: CategorySemanticChange,
		RuleID:      RuleMethodDoesNotExist,
		Explanation: "V1 .exists() rewritten as (recv != null).catch(false)",
	})
	neq := &syntax.BinaryExpr{
		Left:  recv,
		Op:    syntax.NE,
		OpPos: pos(m.NamePos),
		Right: &syntax.LiteralExpr{TokenPos: pos(m.NamePos), TokenType: syntax.NULL, Value: "null"},
	}
	return &syntax.MethodCallExpr{
		Receiver:  neq,
		Method:    "catch",
		MethodPos: pos(m.NamePos),
		Args: []syntax.CallArg{{
			Value: &syntax.LiteralExpr{TokenPos: pos(m.NamePos), TokenType: syntax.FALSE, Value: "false"},
		}},
	}
}

// applyToCall translates `recv.apply("mapName")` into V2 `mapName(recv)`.
// V1 maps take a single implicit receiver passed via apply; V2 maps are
// ordinary callables so the receiver becomes the first positional argument.
func (t *translator) applyToCall(m *v1ast.MethodCall, recv syntax.Expr) syntax.Expr {
	if len(m.Args) != 1 {
		return nil
	}
	// The argument should be a string literal naming the map. If it's
	// something dynamic (e.g. .apply(this.kind)), V2 can't express the
	// dynamic dispatch — flag as unsupported.
	nameLit, ok := m.Args[0].Value.(*v1ast.Literal)
	if !ok || (nameLit.Kind != v1ast.LitString && nameLit.Kind != v1ast.LitRawString) {
		t.rec.Unsupported(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			RuleID:      RuleUnsupportedConstruct,
			Explanation: "V1 .apply() with dynamic map name has no V2 equivalent",
		})
		return nil
	}
	// If the map lives in an imported namespace, qualify the V2 call.
	namespace := t.mapNamespace[nameLit.Str]
	t.rec.Rewritten(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityInfo, Category: CategoryIdiomRewrite,
		RuleID:      RuleMapDeclTranslation,
		SpecRef:     "§10.2",
		Explanation: "V1 recv.apply(\"name\") rewritten as V2 name(recv)",
	})
	return &syntax.CallExpr{
		TokenPos:  pos(m.NamePos),
		Name:      nameLit.Str,
		Namespace: namespace,
		Args:      []syntax.CallArg{{Value: recv}},
	}
}
