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
		// V1 .assign() is a deep recursive merge of nested objects; V2
		// .merge() is shallow at the top level (nested values are
		// replaced, not recursively merged). Flag so callers audit
		// nested object usage.
		return t.rewrittenRename(m, recv, "merge",
			Change{
				RuleID:      RuleMethodDoesNotExist,
				Severity:    SeverityWarning,
				Category:    CategorySemanticChange,
				SpecRef:     "§14#50",
				Explanation: "V1 .assign() recursively deep-merges nested objects; V2 .merge() replaces nested values rather than merging",
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

	// ----- V1 .fold single-param ctx-object lambda -> V2 two-param (tally, value) lambda
	case "fold":
		return t.foldContextToTwoParam(m, recv)

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
		return t.orToOrPlusCatch(m, recv)

	// ----- V1 .merge is polymorphic (object OR array); V2 splits:
	//       .merge for objects, .concat for arrays. Detect array shape from
	//       the receiver / arg and rewrite; otherwise pass through + warn.
	case "merge":
		return t.mergePolymorphicRewrite(m, recv)
	case "filter", "filter_entries", "all", "any":
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 " + "." + m.Name + "() accepts arrays and objects; V2 is strict about receiver type",
		})
		return nil
	case "sum", "min", "max":
		// V1 .sum/.min/.max are numeric-only and always return float64.
		// V2 is typed (int64 stays int64) and .min/.max also accept
		// strings (lexicographic). Flag both angles so downstream type
		// comparisons and expected-error tests surface the divergence.
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 ." + m.Name + "() is numeric-only and returns float64; V2 preserves integer type and (for min/max) also accepts strings",
		})
		return nil
	case "sort":
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 .sort() accepts any element type but produces lexicographic ordering; V2 rejects non-scalar or non-numeric elements outright",
		})
		return nil
	case "reverse":
		// V1 .reverse() errors on empty arrays/strings; V2 returns empty.
		// V1 also rejects non-array/non-string types where V2 may be more
		// lenient.
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 .reverse() errors on empty or non-sequence receivers; V2 returns the empty receiver",
		})
		return nil
	case "abs", "floor", "ceil", "round":
		// V1 numeric methods return an untyped "number"; V2 preserves the
		// typed variant (int64 stays int64, float64 stays float64). Runtime
		// values compare equal but type-introspection / JSON serialisation
		// differ.
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§14#5",
			Explanation: "V1 ." + m.Name + "() returns an unspecified numeric type; V2 preserves int64/float64 — downstream code branching on .type() may behave differently",
		})
		return nil
	case "type":
		// V1 .type() collapses int and float to "number"; V2 reports the
		// precise "int64"/"float64"/"timestamp" strings.
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§13",
			Explanation: "V1 .type() returns \"number\" for any integer/float; V2 reports int64/float64 separately (and timestamp as timestamp, not string)",
		})
		return nil
	case "parse_json", "parse_yaml":
		// V1 returns all numbers as float64; V2 distinguishes int64 and
		// float64 based on the serialised form.
		t.rec.Note(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§13",
			Explanation: "V1 ." + m.Name + "() returns all numbers as float64; V2 distinguishes int64 and float64 by serialised form",
		})
		return nil
	case "index_of":
		// V1 .index_of on strings counts bytes; V2 counts codepoints.
		t.rec.Note(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleStringLengthBytes,
			SpecRef:     "§14#40",
			Explanation: "V1 .index_of() on strings counts bytes; V2 counts codepoints",
		})
		return nil
	case "string":
		// V1 .string() on an integer-valued float64 formats as "5"; V2
		// preserves the float form and emits "5.0".
		t.rec.Note(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityInfo, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 .string() strips trailing zeros from integer-valued floats; V2 preserves the float form (5.0 stays \"5.0\")",
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
	// If already a V1 lambda, translate it 1:1 — no wrap needed. Emit a
	// note: V1 passes the error message as a string; V2 passes an error
	// object `{"what": msg}`, so handlers that concatenate or format the
	// argument will produce different output.
	if _, isLambda := arg.(*v1ast.Lambda); isLambda {
		t.rec.Note(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleOrCatchesErrors,
			SpecRef:     "§12.2",
			Explanation: "V1 .catch(err -> ...) receives the error message as a string; V2 receives an error object of shape {\"what\": msg}",
		})
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
	// V2 [] is type-strict: an out-of-bounds array index or a non-whole
	// float index errors where V1 silently returned null. Flag so the
	// divergence surfaces if the receiver or index isn't statically safe.
	t.rec.Note(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityInfo, Category: CategorySemanticChange,
		RuleID:      RuleNoBracketIndexing,
		Explanation: "V1 " + "." + m.Name + "() returns null on missing key or out-of-bounds index; V2 errors on bounds/type mismatches",
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
	namespace, known := t.mapNamespace[nameLit.Str]
	if !known {
		// V1 imports share a flat table so a map from a transitively
		// imported file is reachable by bare name; V2 namespaces each
		// import explicitly and doesn't re-export. If we can't resolve
		// the name, emit the unqualified call and flag — the V2 output
		// will compile-error at runtime pointing at the missing map.
		t.rec.Note(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleImportStatement,
			SpecRef:     "§10.2",
			Explanation: "V1 .apply(\"" + nameLit.Str + "\") resolves across transitive imports; V2 requires an explicit namespace — add `import \"x\" as ns` and call `ns::" + nameLit.Str + "()`",
		})
	}
	t.rec.Rewritten(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityInfo, Category: CategoryIdiomRewrite,
		RuleID:      RuleMapDeclTranslation,
		SpecRef:     "§10.2",
		Explanation: "V1 recv.apply(\"name\") rewritten as V2 name(recv)",
	})
	// V2 enforces a runtime recursion-depth limit on map calls where V1
	// did not. Flag so recursive / mutually-recursive maps surface.
	t.rec.Note(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityInfo, Category: CategorySemanticChange,
		RuleID:      RuleMapDeclTranslation,
		Explanation: "V2 enforces a runtime recursion-depth limit on map calls that V1 did not — deeply recursive maps may error in V2",
	})
	return &syntax.CallExpr{
		TokenPos:  pos(m.NamePos),
		Name:      nameLit.Str,
		Namespace: namespace,
		Args:      []syntax.CallArg{{Value: recv}},
	}
}

// foldContextToTwoParam rewrites V1 `.fold(init, ctx -> ...ctx.tally...ctx.value...)`
// into V2 `.fold(init, (tally, value) -> ...)`.
//
// V1's fold lambda receives a single context object with `.tally` and
// `.value` fields; V2 takes two explicit parameters. We walk the V1 body
// and replace `<paramName>.tally` / `<paramName>.value` field accesses
// with bare identifiers that resolve to the new V2 parameters, then
// assemble a two-param V2 lambda. If the body references the context
// parameter directly (not via .tally / .value) the shape isn't safely
// mechanical — we fall through to the default translation with a warning
// so the caller knows the V2 output will error at runtime.
func (t *translator) foldContextToTwoParam(m *v1ast.MethodCall, recv syntax.Expr) syntax.Expr {
	if len(m.Args) != 2 {
		return nil
	}
	lam, ok := m.Args[1].Value.(*v1ast.Lambda)
	if !ok || lam.Discard {
		// Map-ref or discard param — V1 also supports these but the shape
		// isn't recognisable from here. Pass through; translator will emit
		// V2 that errors and the warning surfaces the issue.
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§13",
			Explanation: "V1 .fold() second argument must be a one-param lambda for automatic V1→V2 rewrite; manually convert to V2 .fold(init, (tally, value) -> ...)",
		})
		return nil
	}

	paramName := lam.Param
	rewritten, unsafeRef := rewriteFoldContext(lam.Body, paramName)
	if unsafeRef {
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§13",
			Explanation: "V1 .fold() lambda references its context param outside .tally/.value; V2 has no single-value accessor — rewrite manually to use (tally, value) params",
		})
		return nil
	}

	// Translate the initial value and the rewritten body. The two synthetic
	// V2 param names are pushed onto the scope stack so the rewritten bare
	// `tally` / `value` idents resolve as lambda-param references rather
	// than the default V1 bare-ident-to-input rewrite.
	initial := t.translateExpr(m.Args[0].Value)
	if initial == nil {
		return nil
	}
	t.pushScope("tally", "value")
	v2Body := t.translateExpr(rewritten)
	t.popScope()
	if v2Body == nil {
		return nil
	}

	t.rec.Rewritten(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityInfo, Category: CategoryIdiomRewrite,
		RuleID:      RuleMethodDoesNotExist,
		SpecRef:     "§13",
		Explanation: "V1 .fold(init, ctx -> ...ctx.tally...ctx.value...) rewritten as V2 .fold(init, (tally, value) -> ...)",
	})

	lamPos := pos(lam.ParamPos)
	return &syntax.MethodCallExpr{
		Receiver:  recv,
		Method:    "fold",
		MethodPos: pos(m.NamePos),
		Args: []syntax.CallArg{
			{Value: initial},
			{Value: &syntax.LambdaExpr{
				TokenPos: lamPos,
				Params: []syntax.Param{
					{Name: "tally", Pos: lamPos, SlotIndex: -1},
					{Name: "value", Pos: lamPos, SlotIndex: -1},
				},
				Body: &syntax.ExprBody{Result: v2Body},
			}},
		},
	}
}

// orToOrPlusCatch rewrites V1 `.or(x)` (which catches null AND errors) as
// V2 `.or(x).catch(_ -> x)` so both branches are preserved. Mirrors the
// `|` coalesce rewrite in translateBinary.
func (t *translator) orToOrPlusCatch(m *v1ast.MethodCall, recv syntax.Expr) syntax.Expr {
	if len(m.Args) != 1 {
		return nil
	}
	fallback := t.translateExpr(m.Args[0].Value)
	if fallback == nil {
		return nil
	}
	t.rec.Rewritten(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityInfo, Category: CategoryIdiomRewrite,
		RuleID:      RuleOrCatchesErrors,
		SpecRef:     "§12.2",
		Explanation: "V1 .or() catches null AND errors; rewritten as V2 .or(x).catch(_ -> x) to preserve both paths",
	})
	orCall := &syntax.MethodCallExpr{
		Receiver:  recv,
		Method:    "or",
		MethodPos: pos(m.NamePos),
		Args:      []syntax.CallArg{{Value: fallback}},
	}
	catchLambda := &syntax.LambdaExpr{
		TokenPos: pos(m.NamePos),
		Params:   []syntax.Param{{Discard: true, Pos: pos(m.NamePos), SlotIndex: -1}},
		Body:     &syntax.ExprBody{Result: fallback},
	}
	return &syntax.MethodCallExpr{
		Receiver:  orCall,
		Method:    "catch",
		MethodPos: pos(m.NamePos),
		Args:      []syntax.CallArg{{Value: catchLambda}},
	}
}

// mergePolymorphicRewrite handles V1 .merge(). V1 is polymorphic:
//
//   - Object receiver + object arg  → object-level merge (V2 .merge)
//   - Array receiver + array arg    → array concatenation (V2 .concat)
//
// V2 splits these into separate methods. When both the V1 receiver and
// the V1 argument have a statically-visible array shape (array literal
// or a known array-returning method call), we rewrite to `.concat`.
// Otherwise we leave the call as `.merge` and emit a warning.
func (t *translator) mergePolymorphicRewrite(m *v1ast.MethodCall, recv syntax.Expr) syntax.Expr {
	if len(m.Args) == 1 && isArrayExpr(m.Recv) && isArrayExpr(m.Args[0].Value) {
		// Rewrite to V2 .concat(arg).
		arg := t.translateExpr(m.Args[0].Value)
		if arg == nil {
			return nil
		}
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityInfo, Category: CategoryIdiomRewrite,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§14#50",
			Explanation: "V1 .merge() on array receiver+arg rewritten as V2 .concat() (V2 .merge is object-only)",
		})
		return &syntax.MethodCallExpr{
			Receiver:  recv,
			Method:    "concat",
			MethodPos: pos(m.NamePos),
			Args:      []syntax.CallArg{{Value: arg}},
		}
	}
	// Default: pass through as .merge() with a warning.
	t.rec.Rewritten(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityWarning, Category: CategorySemanticChange,
		RuleID: RuleMethodDoesNotExist, SpecRef: "§14#50",
		Explanation: "V1 .merge() is polymorphic (objects AND arrays); V2 .merge is object-only — use .concat(other) for arrays",
	})
	return nil
}

// isArrayExpr reports whether a V1 expression is statically known to
// produce an array value. Used by merge-polymorphic dispatch and any
// future receiver-shape rules.
func isArrayExpr(e v1ast.Expr) bool {
	switch n := e.(type) {
	case *v1ast.ArrayLit:
		return true
	case *v1ast.MethodCall:
		switch n.Name {
		case "map_each", "map", "filter", "filter_entries",
			"sort", "sort_by", "unique", "reverse", "without",
			"slice", "values", "keys", "enumerated", "flatten",
			"find_all", "find_all_by", "collapse", "explode",
			"concat":
			return true
		case "split":
			// .split() on a string returns an array of strings.
			return true
		}
	case *v1ast.FunctionCall:
		if n.Name == "range" {
			return true
		}
	case *v1ast.ParenExpr:
		return isArrayExpr(n.Inner)
	case *v1ast.IfExpr:
		// Both branches must be arrays.
		for _, b := range n.Branches {
			if !isArrayExpr(b.Body) {
				return false
			}
		}
		if n.Else != nil && !isArrayExpr(n.Else) {
			return false
		}
		return true
	}
	return false
}

// rewriteFoldContext walks the V1 expression tree and replaces every
// `<paramName>.tally` / `<paramName>.value` field access with bare
// `tally` / `value` identifiers. The walk is in-place but the caller
// owns the V1 AST by this point (it's being discarded after translation).
// Returns (rewritten, unsafeRef) where unsafeRef is true when we found a
// reference to `<paramName>` outside the .tally/.value pattern — the
// caller should bail on the rewrite in that case.
func rewriteFoldContext(e v1ast.Expr, paramName string) (v1ast.Expr, bool) {
	unsafe := false
	var walk func(v1ast.Expr) v1ast.Expr
	walk = func(e v1ast.Expr) v1ast.Expr {
		if e == nil {
			return nil
		}
		switch n := e.(type) {
		case *v1ast.Ident:
			// Bare reference to the context param — cannot safely rewrite.
			if n.Name == paramName {
				unsafe = true
			}
			return n
		case *v1ast.FieldAccess:
			if id, ok := n.Recv.(*v1ast.Ident); ok && id.Name == paramName {
				switch n.Seg.Name {
				case "tally":
					return &v1ast.Ident{Name: "tally", TokPos: id.TokPos}
				case "value":
					return &v1ast.Ident{Name: "value", TokPos: id.TokPos}
				default:
					// <paramName>.something_else — unexpected, bail.
					unsafe = true
					return n
				}
			}
			n.Recv = walk(n.Recv)
			return n
		case *v1ast.MethodCall:
			n.Recv = walk(n.Recv)
			for i := range n.Args {
				n.Args[i].Value = walk(n.Args[i].Value)
			}
			return n
		case *v1ast.FunctionCall:
			for i := range n.Args {
				n.Args[i].Value = walk(n.Args[i].Value)
			}
			return n
		case *v1ast.MapExpr:
			n.Recv = walk(n.Recv)
			n.Body = walk(n.Body)
			return n
		case *v1ast.Lambda:
			// A nested lambda shadowing paramName binds a fresh value; don't
			// descend into it (the param inside is a different variable).
			if n.Param == paramName {
				return n
			}
			n.Body = walk(n.Body)
			return n
		case *v1ast.BinaryExpr:
			n.Left = walk(n.Left)
			n.Right = walk(n.Right)
			return n
		case *v1ast.UnaryExpr:
			n.Operand = walk(n.Operand)
			return n
		case *v1ast.ParenExpr:
			n.Inner = walk(n.Inner)
			return n
		case *v1ast.ArrayLit:
			for i := range n.Elems {
				n.Elems[i] = walk(n.Elems[i])
			}
			return n
		case *v1ast.ObjectLit:
			for i := range n.Entries {
				n.Entries[i].Key = walk(n.Entries[i].Key)
				n.Entries[i].Value = walk(n.Entries[i].Value)
			}
			return n
		case *v1ast.MetaCall:
			n.Key = walk(n.Key)
			return n
		case *v1ast.IfExpr:
			for i := range n.Branches {
				n.Branches[i].Cond = walk(n.Branches[i].Cond)
				n.Branches[i].Body = walk(n.Branches[i].Body)
			}
			n.Else = walk(n.Else)
			return n
		case *v1ast.MatchExpr:
			n.Subject = walk(n.Subject)
			for i := range n.Cases {
				n.Cases[i].Pattern = walk(n.Cases[i].Pattern)
				n.Cases[i].Body = walk(n.Cases[i].Body)
			}
			return n
		}
		// Literal, ThisExpr, RootExpr, VarRef, MetaRef — no child Expr to rewrite.
		return e
	}
	return walk(e), unsafe
}
