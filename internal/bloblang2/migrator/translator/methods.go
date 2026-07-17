package translator

import (
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

// flagDotPathArg flags the path-vs-literal-key divergence in get/exists/
// without: V1 splits a string argument on "." and traverses it as a nested
// path, whereas the emitted V2 uses the argument as a single literal key
// (get -> input["k"], exists -> has_key("k"), without -> without(["k"])). A
// dotted string-literal argument therefore diverges in both directions. This
// records a Note (not a coverage-bumping change) so it can layer on top of the
// method's own translation without double-counting the node.
func (t *translator) flagDotPathArg(m *v1ast.MethodCall) {
	for _, a := range m.Args {
		lit, ok := a.Value.(*v1ast.Literal)
		if ok && (lit.Kind == v1ast.LitString || lit.Kind == v1ast.LitRawString) && strings.Contains(lit.Str, ".") {
			t.rec.Note(Change{
				Line: m.NamePos.Line, Column: m.NamePos.Column,
				Severity: SeverityWarning, Category: CategorySemanticChange,
				RuleID: RuleMethodDoesNotExist, SpecRef: "§13",
				Explanation: "V1 ." + m.Name + `("` + lit.Str + `") splits the argument on '.' and traverses it as a nested path; the emitted V2 treats it as a single literal key — results differ. Rewrite the path by hand (e.g. explicit field access).`,
			})
			return
		}
	}
}

// methodRewrite applies V1 → V2 method-shape translations on a V1 MethodCall.
// Returns a non-nil V2 expression on success, or nil to signal "fall through
// to the default 1:1 translation".
//
// Rules are ordered by the V1 method name; each rule may:
//   - rename the method (e.g. map_each -> map),
//   - convert the method call to a different V2 node shape (e.g. index -> []),
//   - leave it alone (default).
func (t *translator) methodRewrite(m *v1ast.MethodCall, recv syntax.Expr) syntax.Expr {
	// Custom rules win on name collision (design P2). When a registered
	// rule signals handled=true, its result short-circuits the
	// built-in switch entirely. Returning a nil expression with
	// handled=true is the rule's way of signalling Unsupported — the
	// translator still falls back to the default 1:1 translation but
	// the rule will already have recorded an Error-severity Change.
	if rule, ok := t.customMethodRules[m.Name]; ok {
		if out, handled := rule(t, m, recv); handled {
			if out == nil {
				return nil
			}
			return out
		}
	}
	switch m.Name {

	// ----- Simple renames (V2 name differs, same shape) -----
	case "map_each":
		// V1 .map_each accepts arrays and objects; V2 has no single
		// polymorphic equivalent. On an ARRAY it passes each element (→ V2
		// .map); on an OBJECT it passes each entry as {key, value} and
		// replaces the value (→ V2 .map_values / .map_entries, which bind
		// differently). When the receiver is a statically-known object
		// literal we can translate faithfully; otherwise the runtime type is
		// unknown so we emit .map (the array case) and flag the object case.
		if _, isObj := m.Recv.(*v1ast.ObjectLit); isObj {
			return t.translateObjectMapEach(m, recv)
		}
		return t.queryFormRename(m, recv, "map", &Change{
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID: RuleMethodDoesNotExist,
			Explanation: "V1 .map_each() accepts arrays and objects; the receiver type is not statically known. " +
				"V2 .map() (emitted) is array-only — if the receiver is an object, V1 binds each entry as " +
				"{key, value} and replaces the value: use .map_values(v -> ...) when only the value is needed, " +
				"or .map_entries((k, v) -> {\"key\": k, \"value\": ...}) when the key is needed",
		}, true)
	case "map":
		// V1 .map(fn) applies fn to the WHOLE receiver value (fn's `this` /
		// param is the whole value), for any type — it is NOT element-wise.
		// V2 .map is element-wise and array-only, so a verbatim passthrough
		// silently changes meaning. V2's apply-once-to-the-whole-value method
		// is .into, so translate V1 .map -> V2 .into.
		return t.queryFormRename(m, recv, "into", &Change{
			Severity: SeverityInfo, Category: CategoryIdiomRewrite,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§13",
			Explanation: "V1 .map(fn) applies fn to the whole value; rewritten to V2 .into(fn) (V2 .map is element-wise/array-only)",
		}, false)
	case "trim":
		// V1 .trim() removes whitespace (matches V2). V1 .trim(cutset) removes
		// the given characters — V2 .trim() takes no argument and has no cutset
		// form, so drop the argument and flag; the character-set trim must be
		// migrated by hand (e.g. a regex replace).
		if len(m.Args) == 0 {
			return nil // whitespace trim — identical, use the default 1:1
		}
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID: RuleMethodDoesNotExist, SpecRef: "§13",
			Explanation: "V1 .trim(cutset) removes the given characters; V2 .trim() removes whitespace only and has no cutset form — the cutset argument was dropped, migrate manually",
		})
		return &syntax.MethodCallExpr{Receiver: recv, Method: "trim", MethodPos: pos(m.NamePos)}
	case "enumerated":
		return t.simpleRename(m, recv, "enumerate")
	case "key_values":
		return t.simpleRename(m, recv, "iter")
	case "map_each_key":
		// V1 .map_each_key == V2 .map_keys (exact match — both take lambda).
		return t.queryFormRename(m, recv, "map_keys", nil, false)
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
		t.flagDotPathArg(m)
		return t.indexToBracket(m, recv)

	// ----- Apply: recv.apply("name") -> name(recv) -----
	case "apply":
		return t.applyToCall(m, recv)

	// ----- Numeric coercion: V1 .number() -> V2 .float64() -----
	case "number":
		ch := Change{
			RuleID:      RuleMethodDoesNotExist,
			Severity:    SeverityWarning,
			Category:    CategorySemanticChange,
			Explanation: "V1 .number() is float64; V2 .float64() preserves that, but downstream code expecting int64 results may break",
		}
		if len(m.Args) > 0 {
			// V1 .number(default) uses the arg as a fallback on parse
			// failure/null; V2 .float64() does not — the arg is inert.
			ch.Explanation = "V1 .number(default) returns the default on parse failure/null; V2 .float64() does NOT use the argument as a fallback (it is inert) — wrap as .float64().catch(_ -> default) to preserve the fallback"
		}
		return t.rewrittenRename(m, recv, "float64", ch)
	case "bool":
		// V1 .bool() coerces permissively ("TRUE"/"t"/"1"/"0"/1/0 → bool); V2
		// .bool() is strict (only real booleans and exact "true"/"false").
		// V1 .bool(default) also falls back on failure/null; V2 does not. 1:1
		// shape (bool exists in V2) — flag the semantic change.
		expl := `V1 .bool() coerces strings like "TRUE"/"t"/"1"/"0" and numbers to bool; V2 .bool() errors on those — only real booleans (and exact "true"/"false") convert`
		if len(m.Args) > 0 {
			expl = "V1 .bool(default) coerces permissively and returns the default on failure/null; V2 .bool() is strict and does not use the argument as a fallback — wrap as .bool().catch(_ -> default) and pre-normalise non-boolean forms"
		}
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID: RuleMethodDoesNotExist, SpecRef: "§13",
			Explanation: expl,
		})
		return nil
	case "format_json":
		// V1 .format_json() defaults to 4-space indentation; V2 defaults to
		// compact (no indent). When no argument was given, emit an explicit
		// 4-space indent so the migrated output matches V1. Explicit args pass
		// through unchanged.
		if len(m.Args) == 0 {
			t.rec.Rewritten(Change{
				Line: m.NamePos.Line, Column: m.NamePos.Column,
				Severity: SeverityInfo, Category: CategoryIdiomRewrite,
				RuleID: RuleMethodDoesNotExist, SpecRef: "§13",
				Explanation: `V1 .format_json() defaults to 4-space indentation; V2 defaults to compact — emitted .format_json("    ") to preserve V1's indented output`,
			})
			return &syntax.MethodCallExpr{
				Receiver: recv, Method: "format_json", MethodPos: pos(m.NamePos),
				Args: []syntax.CallArg{{Value: &syntax.LiteralExpr{
					TokenPos: pos(m.NamePos), TokenType: syntax.STRING, Value: "    ",
				}}},
			}
		}
		return nil

	// ----- Variadic .without("a","b","c") -> .without(["a","b","c"]) -----
	case "without":
		t.flagDotPathArg(m)
		return t.variadicArgsToArray(m, recv, "without")
	// ----- Variadic .with(...) and .zip(...) follow the same pattern.
	case "with", "zip":
		return t.variadicArgsToArray(m, recv, m.Name)
	// ----- V1 `.format(a, b, ...)` (variadic) -> V2 `.format([a, b, ...])`.
	case "format":
		return t.variadicArgsToArray(m, recv, "format")

	// ----- V1 timestamp method renames.
	// V2 ts_format / ts_parse use strftime/strptime exclusively, so V1
	// callsites that already use the strftime/strptime variants rename
	// directly. The V1 Go-layout variants (`format_timestamp`,
	// `parse_timestamp`) cannot be auto-rewritten because V2 has no
	// Go-layout method — flag with a Note instead.
	case "ts_strftime", "format_timestamp_strftime":
		return t.simpleRename(m, recv, "ts_format")
	case "ts_strptime", "parse_timestamp_strptime":
		return t.simpleRename(m, recv, "ts_parse")
	case "format_timestamp", "parse_timestamp":
		t.rec.Note(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 ." + m.Name + "() uses Go's reference-time layout; V2 ts_format / ts_parse use strftime/strptime — convert the format string and rename to ." + map[string]string{"format_timestamp": "ts_format", "parse_timestamp": "ts_parse"}[m.Name] + "() manually",
		})
		return nil
	case "format_timestamp_unix":
		return t.simpleRename(m, recv, "ts_unix")
	case "format_timestamp_unix_milli":
		return t.simpleRename(m, recv, "ts_unix_milli")
	case "format_timestamp_unix_micro":
		return t.simpleRename(m, recv, "ts_unix_micro")
	case "format_timestamp_unix_nano":
		return t.simpleRename(m, recv, "ts_unix_nano")

	// ----- .find(value) -> .index_of(value) -----
	case "find":
		return t.findValueToIndexOf(m, recv)

	// ----- V1 .fold single-param ctx-object lambda -> V2 two-param (tally, value) lambda
	case "fold":
		return t.foldContextToTwoParam(m, recv)

	// ----- .exists(path) -> (path != null).catch(false) -----
	case "exists":
		t.flagDotPathArg(m)
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
		if m.Name == "all" {
			// V1 .all([]) on an EMPTY array returns false; V2 returns true
			// (vacuous truth). A Note (not a coverage-bumping change) so it
			// layers on top of queryFormRename's tally without double-counting.
			t.rec.Note(Change{
				Line: m.NamePos.Line, Column: m.NamePos.Column,
				Severity: SeverityWarning, Category: CategorySemanticChange,
				RuleID: RuleMethodDoesNotExist, SpecRef: "§13",
				Explanation: "V1 .all() on an empty array returns false; V2 returns true (vacuous truth) — results differ on empty input",
			})
		}
		// Pass the divergence as queryFormRename's note so the method node is
		// tallied exactly once (Rewritten). Recording a separate Rewritten
		// here and then calling queryFormRename — which tallies again — would
		// double-count the node and inflate the coverage ratio.
		return t.queryFormRename(m, recv, m.Name, &Change{
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 ." + m.Name + "() accepts arrays and objects; V2 is strict about receiver type",
		}, false)
	case "find_by", "find_all_by":
		// V1 .find_by / .find_all_by take a ParamQuery predicate where
		// `this` and bare idents resolve as fields of the current
		// element. V2 has the same methods but requires an explicit lambda,
		// so wrap the query form unconditionally.
		return t.queryFormRename(m, recv, m.Name, nil, false)
	case "sort_by":
		return t.queryFormRename(m, recv, m.Name, nil, false)
	case "unique":
		// V1 .unique() with no args = identity comparison; with one arg
		// it's a ParamQuery key extractor that needs wrapping.
		if len(m.Args) == 1 {
			return t.queryFormRename(m, recv, "unique", nil, false)
		}
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
		if len(m.Args) > 0 {
			// V1 .sort(<query>) uses implicit `left`/`right` identifiers as a
			// boolean comparator. V2 has no comparator-sort form: .sort() is
			// ascending-only and .sort_by(fn) takes a one-argument key
			// function. Translating the comparator through the normal path
			// rewrites the bare `left`/`right` identifiers into
			// `input.left`/`input.right`, emitting nonsense like
			// `output.sort(input.left > input.right)` (and counting it as an
			// Exact translation). Instead, drop the comparator and emit a bare
			// ascending `.sort()` as a best-effort starting point, flagged
			// Unsupported (Error) so the user MUST migrate the ordering by hand
			// (e.g. `.sort()` ascending, `.sort().reverse()` descending, or
			// `.sort_by(fn)` for a key). Note: the bare `.sort()` only executes
			// for a scalar array — for object/key comparators it errors at
			// runtime (objects aren't sortable), which is why it's Unsupported.
			t.rec.Unsupported(Change{
				Line: m.NamePos.Line, Column: m.NamePos.Column,
				RuleID:      RuleUnsupportedConstruct,
				Explanation: "V1 .sort(<comparator>) uses implicit left/right comparator identifiers; V2 has no comparator sort (use .sort() for ascending, .sort().reverse() for descending, or .sort_by(fn) for a key). The comparator was dropped and a bare ascending .sort() emitted — migrate the ordering manually.",
			})
			return &syntax.MethodCallExpr{
				Receiver:  recv,
				Method:    "sort",
				MethodPos: pos(m.NamePos),
			}
		}
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 .sort() accepts any element type but produces lexicographic ordering; V2 rejects non-scalar or non-numeric elements outright",
		})
		return nil
	case "slice":
		// V1 .slice() indexes STRINGS by byte; V2 by Unicode codepoint (spec
		// §13 ".slice"), so slicing a multi-byte string differs. Arrays (by
		// element) and bytes (by byte) are UNAFFECTED. Skip the flag when the
		// receiver is statically a non-string (array/object); otherwise flag
		// conservatively (a dynamic receiver may be a string). 1:1 shape
		// (return nil), counted once via the Rewritten flag so
		// translateMethodCall skips the Exact tally.
		if sliceReceiverIsNonString(m.Recv) {
			return nil
		}
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleSliceByteVsCodepoint,
			Explanation: "on a string receiver, V1 .slice() indexes by byte while V2 indexes by Unicode codepoint — results differ on multi-byte strings (array/bytes receivers are unaffected)",
		})
		return nil
	case "contains":
		// Intentionally NOT flagged. .contains() is 1:1 and has no V1/V2
		// divergence: V2 numeric equality uses promotion (spec §2 "5 == 5.0 is
		// true"), so array .contains() matches 1 against 1.0 exactly as V1
		// does, and cross-type comparisons are false in both. String substring
		// search is identical too. Verified against both engines. Falls through
		// to the default 1:1 translation (counted Exact once). Do not re-add a
		// "type-strict" flag — it was factually wrong.
		//
		// EXCEPTION: on an OBJECT receiver, V1 .contains(v) checks membership
		// over the object's values, whereas V2 .contains() errors on objects.
		// Only object-literal receivers are statically detectable; dynamic
		// receivers can't be classified here.
		if _, isObj := m.Recv.(*v1ast.ObjectLit); isObj {
			t.rec.Note(Change{
				Line: m.NamePos.Line, Column: m.NamePos.Column,
				Severity: SeverityWarning, Category: CategorySemanticChange,
				RuleID: RuleMethodDoesNotExist, SpecRef: "§13",
				Explanation: "V1 .contains(v) on an object checks membership over its values; V2 .contains() errors on object receivers — use .values().contains(v)",
			})
		}
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
	case "abs", "floor", "ceil":
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
	case "round":
		// Two divergences: (1) the numeric-type change (as abs/floor/ceil
		// above), and — more importantly — (2) the ROUNDING MODE. V1 rounds
		// half away from zero (0.5 -> 1, 2.5 -> 3, -2.5 -> -3); V2 rounds half
		// to even / banker's (0.5 -> 0, 2.5 -> 2). Values at .5 boundaries
		// differ. 1:1 shape, flagged once via the Rewritten tally.
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			SpecRef:     "§14#5",
			Explanation: "V1 .round() rounds half AWAY FROM ZERO (2.5 -> 3); V2 .round() rounds half TO EVEN (banker's: 2.5 -> 2) — results differ at every .5 boundary. (V2 also preserves int64/float64 rather than an untyped number.)",
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

// variadicArgsToArray rewrites V1 variadic-style method calls
// `.NAME(a, b, c)` into V2 `.NAME([a, b, c])`. Used for V1 methods whose
// V2 counterpart was redefined to take a single array argument now that
// V2 rejects variadic plugins at compile time (without, with, zip).
//
// If the V1 call already passes a single array literal the rewrite is a
// no-op rename.
func (t *translator) variadicArgsToArray(m *v1ast.MethodCall, recv syntax.Expr, name string) syntax.Expr {
	if len(m.Args) == 1 {
		if _, ok := m.Args[0].Value.(*v1ast.ArrayLit); ok {
			return t.simpleRename(m, recv, name)
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
		Explanation: "V1 variadic ." + name + "(...) rewritten as V2 ." + name + "([...])",
	})
	return &syntax.MethodCallExpr{
		Receiver:  recv,
		Method:    name,
		MethodPos: pos(m.NamePos),
		Args: []syntax.CallArg{{
			Value: &syntax.ArrayLiteral{LBracketPos: pos(m.NamePos), Elements: elems},
		}},
	}
}

// queryFormRename translates a V1 method call whose final argument is a
// ParamQuery (V1 rebinds `this` and bare idents to the per-element
// context). When the V1 argument is already an explicit lambda we
// translate through 1:1; otherwise we synthesize a V2 lambda that
// rebinds `this` to a fresh parameter so the V2 surface (which requires
// an explicit lambda) sees the same effective predicate.
//
// `newName` selects the V2 method name (often the same as V1). If
// `note` is non-nil it is recorded as a Rewritten change describing the
// rename.
func (t *translator) queryFormRename(m *v1ast.MethodCall, recv syntax.Expr, newName string, note *Change, keepElement bool) syntax.Expr {
	args := make([]syntax.CallArg, 0, len(m.Args))
	wrapped := false
	for i, a := range m.Args {
		if i == len(m.Args)-1 {
			lam, didWrap := t.translateQueryFormPredicate(a.Value, m.NamePos)
			if lam == nil {
				return nil
			}
			// For element-transform methods (V1 .map_each), a lambda whose
			// body is an else-less `if` returns V1's nothing sentinel when the
			// condition is false, which V1 treats as "keep the original
			// element". V2 has no such fallback (void errors), so synthesize
			// an explicit `else { <element> }` to preserve the semantics.
			if keepElement {
				t.keepElementOnBareIf(lam, m.NamePos)
			}
			args = append(args, syntax.CallArg{Name: a.Name, Value: lam})
			wrapped = didWrap
			continue
		}
		v := t.translateExpr(a.Value)
		if v == nil {
			return nil
		}
		args = append(args, syntax.CallArg{Name: a.Name, Value: v})
	}
	switch {
	case note != nil:
		ch := *note
		ch.Line = m.NamePos.Line
		ch.Column = m.NamePos.Column
		if wrapped {
			ch.Explanation += "; V1 query-form wrapped as V2 (__v -> ...)"
		}
		t.rec.Rewritten(ch)
	case wrapped:
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityInfo, Category: CategoryIdiomRewrite,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 ." + m.Name + "(query-form) wrapped as V2 ." + newName + "(__v -> ...) — V2 requires an explicit lambda",
		})
	default:
		t.rec.Exact()
	}
	return &syntax.MethodCallExpr{
		Receiver:  recv,
		Method:    newName,
		MethodPos: pos(m.NamePos),
		Args:      args,
		Named:     m.Named,
	}
}

// keepElementOnBareIf preserves V1 .map_each's "nothing keeps the original
// element" semantics for a lambda whose body is an else-less `if`. V1 returns
// its nothing sentinel when no branch matches and .map_each keeps the element;
// V2 would produce void and error. If the lambda's element parameter is
// referenceable, synthesize `else { <element> }` so the element is kept.
// A discarded (_) parameter can't be referenced, so it's left as-is (the
// §14#44 divergence note recorded during body translation still flags it).
func (t *translator) keepElementOnBareIf(lam syntax.Expr, namePos v1ast.Pos) {
	le, ok := lam.(*syntax.LambdaExpr)
	if !ok || le.Body == nil {
		return
	}
	ifExpr, ok := le.Body.Result.(*syntax.IfExpr)
	if !ok || ifExpr.Else != nil {
		return
	}
	if len(le.Params) == 0 {
		return
	}
	param := le.Params[0].Name
	if le.Params[0].Discard || param == "" {
		// The element param was discarded (`_`), so we can't reference it —
		// but V1 keeps the element, so give the param a synthetic name and
		// un-discard it. Safe: a discarded param is by definition unused in
		// the body, so introducing the name cannot collide with anything.
		param = "__elem"
		le.Params[0].Name = param
		le.Params[0].Discard = false
	}
	ifExpr.Else = &syntax.ExprBody{Result: &syntax.IdentExpr{
		TokenPos: pos(namePos), Name: param, SlotIndex: -1,
	}}
	t.rec.Note(Change{
		Line: namePos.Line, Column: namePos.Column,
		Severity: SeverityInfo, Category: CategorySemanticChange,
		RuleID: RuleIfNoElseNothing, SpecRef: "§14#34",
		Explanation: "V1 .map_each with an else-less `if` keeps the original element when the condition is false; synthesized `else { " + param + " }` to preserve this in V2",
	})
}

// keepElementOnBareIfValue is the object-receiver variant of
// keepElementOnBareIf. In the .map_entries/.into rewrite the lambda parameter
// is bound to the whole {key, value} entry, but V1 .map_each over an object
// keeps the original VALUE (not the entry) when an else-less `if` matches
// nothing — so the synthesized else is `<param>.value`, not `<param>`.
func (t *translator) keepElementOnBareIfValue(lam syntax.Expr, namePos v1ast.Pos) {
	le, ok := lam.(*syntax.LambdaExpr)
	if !ok || le.Body == nil {
		return
	}
	ifExpr, ok := le.Body.Result.(*syntax.IfExpr)
	if !ok || ifExpr.Else != nil || len(le.Params) == 0 {
		return
	}
	param := le.Params[0].Name
	if le.Params[0].Discard || param == "" {
		param = "__entry"
		le.Params[0].Name = param
		le.Params[0].Discard = false
	}
	ifExpr.Else = &syntax.ExprBody{Result: &syntax.FieldAccessExpr{
		Receiver: &syntax.IdentExpr{TokenPos: pos(namePos), Name: param, SlotIndex: -1},
		Field:    "value",
		FieldPos: pos(namePos),
	}}
	t.rec.Note(Change{
		Line: namePos.Line, Column: namePos.Column,
		Severity: SeverityInfo, Category: CategorySemanticChange,
		RuleID: RuleIfNoElseNothing, SpecRef: "§14#34",
		Explanation: "V1 .map_each over an object with an else-less `if` keeps the original value; synthesized `else { " + param + ".value }` to preserve this in V2",
	})
}

// exprYieldsNothing reports whether e contains a V1 `deleted()`/`nothing()`
// sentinel call. Used to detect object .map_each bodies that drop entries: the
// .map_entries/.into rewrite can't place a dropped value in an entry's "value"
// position (it errors), so those are flagged Unsupported instead.
func exprYieldsNothing(e v1ast.Expr) bool {
	switch x := e.(type) {
	case *v1ast.FunctionCall:
		if (x.Name == "deleted" || x.Name == "nothing") && len(x.Args) == 0 {
			return true
		}
		return anyArgYieldsNothing(x.Args)
	case *v1ast.MethodCall:
		return exprYieldsNothing(x.Recv) || anyArgYieldsNothing(x.Args)
	case *v1ast.BinaryExpr:
		return exprYieldsNothing(x.Left) || exprYieldsNothing(x.Right)
	case *v1ast.UnaryExpr:
		return exprYieldsNothing(x.Operand)
	case *v1ast.ParenExpr:
		return exprYieldsNothing(x.Inner)
	case *v1ast.FieldAccess:
		return exprYieldsNothing(x.Recv)
	case *v1ast.Lambda:
		return exprYieldsNothing(x.Body)
	case *v1ast.ArrayLit:
		for _, el := range x.Elems {
			if exprYieldsNothing(el) {
				return true
			}
		}
	case *v1ast.ObjectLit:
		for _, en := range x.Entries {
			if exprYieldsNothing(en.Key) || exprYieldsNothing(en.Value) {
				return true
			}
		}
	case *v1ast.IfExpr:
		for _, br := range x.Branches {
			if exprYieldsNothing(br.Cond) || exprYieldsNothing(br.Body) {
				return true
			}
		}
		return x.Else != nil && exprYieldsNothing(x.Else)
	case *v1ast.MatchExpr:
		if x.Subject != nil && exprYieldsNothing(x.Subject) {
			return true
		}
		for _, c := range x.Cases {
			if exprYieldsNothing(c.Body) {
				return true
			}
		}
	case *v1ast.MapExpr:
		return exprYieldsNothing(x.Recv) || exprYieldsNothing(x.Body)
	}
	return false
}

func anyArgYieldsNothing(args []v1ast.CallArg) bool {
	for _, a := range args {
		if exprYieldsNothing(a.Value) {
			return true
		}
	}
	return false
}

// mapEntryObject builds the V2 object literal {"key": <k>, "value": <v>}.
func mapEntryObject(k, v syntax.Expr, namePos v1ast.Pos) *syntax.ObjectLiteral {
	strKey := func(s string) syntax.Expr {
		return &syntax.LiteralExpr{TokenPos: pos(namePos), TokenType: syntax.STRING, Value: s}
	}
	return &syntax.ObjectLiteral{
		LBracePos: pos(namePos),
		Entries: []syntax.ObjectEntry{
			{Key: strKey("key"), Value: k},
			{Key: strKey("value"), Value: v},
		},
	}
}

// translateObjectMapEach translates V1 `.map_each` over a statically-known
// object literal. V1 binds each entry as {key, value} and replaces the value
// with the lambda result; V2 has no drop-in equivalent (.map_values binds the
// bare value; .map_entries binds (key, value) and returns a {key, value}
// object). We rebuild the entry and bind it via .into so the body sees
// {key, value} exactly as V1 did:
//
//	recv.map_entries((__ek, __ev) -> {
//	    "key":   __ek,
//	    "value": {"key": __ek, "value": __ev}.into(<param> -> <body>),
//	})
//
// Keep-element (else-less if) synthesizes `else { <param>.value }` (V1 keeps
// the original value). A body that can yield deleted()/nothing() can't be
// represented in the "value" position (it would error), so that case is
// flagged Unsupported with the best-effort form still emitted.
func (t *translator) translateObjectMapEach(m *v1ast.MethodCall, recv syntax.Expr) syntax.Expr {
	if len(m.Args) != 1 {
		return nil
	}
	lamExpr, _ := t.translateQueryFormPredicate(m.Args[0].Value, m.NamePos)
	lam, ok := lamExpr.(*syntax.LambdaExpr)
	if !ok {
		return nil
	}
	t.keepElementOnBareIfValue(lam, m.NamePos)

	entryRef := func() *syntax.ObjectLiteral {
		return mapEntryObject(identRef("__ek", m.NamePos), identRef("__ev", m.NamePos), m.NamePos)
	}
	inner := &syntax.MethodCallExpr{
		Receiver:  entryRef(),
		Method:    "into",
		MethodPos: pos(m.NamePos),
		Args:      []syntax.CallArg{{Value: lam}},
	}
	outerLam := &syntax.LambdaExpr{
		TokenPos: pos(m.NamePos),
		Params: []syntax.Param{
			{Name: "__ek", Pos: pos(m.NamePos), SlotIndex: -1},
			{Name: "__ev", Pos: pos(m.NamePos), SlotIndex: -1},
		},
		Body: &syntax.ExprBody{Result: mapEntryObject(identRef("__ek", m.NamePos), inner, m.NamePos)},
	}
	if exprYieldsNothing(m.Args[0].Value) {
		t.rec.Unsupported(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			RuleID:      RuleUnsupportedConstruct,
			Explanation: "V1 .map_each over an object whose body can drop entries (deleted()/nothing()) has no faithful V2 .map_entries form — a dropped value cannot sit in an entry's \"value\"; emitted best-effort, migrate the drop to a .map_entries lambda returning deleted() manually",
		})
	} else {
		t.rec.Rewritten(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleMethodDoesNotExist,
			Explanation: "V1 .map_each over an object binds each entry as {key, value} and replaces the value; rewritten to V2 .map_entries with the entry rebound via .into so the body sees {key, value} as in V1 — review the (verbose) result",
		})
	}
	return &syntax.MethodCallExpr{
		Receiver:  recv,
		Method:    "map_entries",
		MethodPos: pos(m.NamePos),
		Args:      []syntax.CallArg{{Value: outerLam}},
	}
}

// identRef builds a by-name V2 identifier reference (slot resolved later).
func identRef(name string, namePos v1ast.Pos) *syntax.IdentExpr {
	return &syntax.IdentExpr{TokenPos: pos(namePos), Name: name, SlotIndex: -1}
}

// translateQueryFormPredicate translates a single V1 ParamQuery argument.
// Returns the V2 lambda expression and a `wrapped` flag indicating whether
// a lambda had to be synthesized (true when the V1 source used the
// query form rather than an explicit lambda).
func (t *translator) translateQueryFormPredicate(arg v1ast.Expr, namePos v1ast.Pos) (syntax.Expr, bool) {
	if _, ok := arg.(*v1ast.Lambda); ok {
		return t.translateExpr(arg), false
	}
	const paramName = "__v"
	t.pushScope(paramName)
	t.pushThisRebind(paramName)
	t.pushCtx(ctxLambdaBody)
	body := t.translateExpr(arg)
	t.popCtx()
	t.popThisRebind()
	t.popScope()
	if body == nil {
		return nil, false
	}
	return &syntax.LambdaExpr{
		TokenPos: pos(namePos),
		Params:   []syntax.Param{{Name: paramName, Pos: pos(namePos), SlotIndex: -1}},
		Body:     &syntax.ExprBody{Result: body},
	}, true
}

// findValueToIndexOf rewrites V1 `.find(value)` (returns the index of the
// first matching element, or -1) to V2 `.index_of(value)` (same signature
// and semantics). V2's stdlib `find` exists but takes a lambda and returns
// the matching element — not a semantic match for V1's value-based find.
func (t *translator) findValueToIndexOf(m *v1ast.MethodCall, recv syntax.Expr) syntax.Expr {
	if len(m.Args) != 1 {
		return nil
	}
	needle := t.translateExpr(m.Args[0].Value)
	if needle == nil {
		return nil
	}
	t.rec.Rewritten(Change{
		Line: m.NamePos.Line, Column: m.NamePos.Column,
		Severity: SeverityInfo, Category: CategoryIdiomRewrite,
		RuleID:      RuleMethodDoesNotExist,
		Explanation: "V1 .find(value) rewritten as V2 .index_of(value) (V2 .find takes a lambda and returns an element).",
	})
	return &syntax.MethodCallExpr{
		Receiver:  recv,
		Method:    "index_of",
		MethodPos: pos(m.NamePos),
		Args:      []syntax.CallArg{{Value: needle}},
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
	// V1 .fold() takes the accumulator body in two shapes:
	//   - explicit lambda: .fold(init, ctx -> ...ctx.tally...ctx.value...)
	//   - query form:      .fold(init, this.tally.merge(...this.value...))
	// where in the query form `this` IS the {tally, value} context. Both map
	// to V2's .fold(init, (tally, value) -> ...); rewrite context field
	// accesses to the bare `tally` / `value` names the two-param lambda binds.
	var (
		rewritten v1ast.Expr
		unsafeRef bool
		lamPos    syntax.Pos
	)
	switch arg := m.Args[1].Value.(type) {
	case *v1ast.Lambda:
		if arg.Discard {
			// Discard param: the context is thrown away, so there is no
			// recognisable tally/value to map. Flag for manual conversion.
			t.rec.Rewritten(Change{
				Line: m.NamePos.Line, Column: m.NamePos.Column,
				Severity: SeverityWarning, Category: CategorySemanticChange,
				RuleID:      RuleMethodDoesNotExist,
				SpecRef:     "§13",
				Explanation: "V1 .fold() second argument discards its context param; manually convert to V2 .fold(init, (tally, value) -> ...)",
			})
			return nil
		}
		rewritten, unsafeRef = rewriteFoldContext(arg.Body, arg.Param)
		lamPos = pos(arg.ParamPos)
	default:
		// Query form: `this` is the fold context.
		rewritten, unsafeRef = rewriteFoldContextThis(arg)
		lamPos = pos(m.NamePos)
	}
	if unsafeRef {
		// The body references the fold context as a whole object (bare `this`
		// / bare param) or a field other than .tally/.value. V2's
		// (tally, value) lambda has no single context object, so there is no
		// faithful translation — flag Unsupported (not a soft Warning) so the
		// broken best-effort V2 is not mistaken for a working migration.
		t.rec.Unsupported(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			RuleID:      RuleUnsupportedConstruct,
			SpecRef:     "§13",
			Explanation: "V1 .fold() body references its context as a whole object (or outside .tally/.value); V2's (tally, value) lambda has no single context object — rewrite manually",
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
	if t.exprMayDivergeIfDuplicated(m.Args[0].Value) {
		t.rec.Note(Change{
			Line: m.NamePos.Line, Column: m.NamePos.Column,
			Severity: SeverityWarning, Category: CategorySemanticChange,
			RuleID:      RuleCoalesceDuplicatesFallback,
			Explanation: "V1 .or() fallback is duplicated in the V2 .or(x).catch(_ -> x) rewrite; a nondeterministic or side-effecting fallback runs twice on the error path where V1 ran it once — verify or hoist it into a `let`",
		})
	}
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

// sliceReceiverIsNonString reports whether e is statically a non-string value
// (array/object), so the .slice byte-vs-codepoint divergence — which is
// string-only — cannot apply and the flag can be skipped. It is deliberately
// STRICTER than isArrayExpr: V1 .reverse() is string-only and .slice() returns
// the receiver's own type, so a chain ending in either (e.g.
// `s.reverse().slice(...)` or `s.slice(1,5).slice(0,2)`) may still be a string
// and MUST keep the flag. Everything else isArrayExpr recognises (map_each,
// map, filter, split, values, keys, collapse→object, range(), array literals,
// …) is a non-string collection and is safe to skip. Recurses through parens
// and if-branches so a wrapped slice/reverse is still caught.
func sliceReceiverIsNonString(e v1ast.Expr) bool {
	switch n := e.(type) {
	case *v1ast.MethodCall:
		if n.Name == "slice" || n.Name == "reverse" {
			return false
		}
		return isArrayExpr(e)
	case *v1ast.ParenExpr:
		return sliceReceiverIsNonString(n.Inner)
	case *v1ast.IfExpr:
		for _, b := range n.Branches {
			if !sliceReceiverIsNonString(b.Body) {
				return false
			}
		}
		if n.Else != nil && !sliceReceiverIsNonString(n.Else) {
			return false
		}
		return true
	default:
		return isArrayExpr(e)
	}
}

// rewriteFoldContext walks the V1 expression tree and replaces every
// `<paramName>.tally` / `<paramName>.value` field access with bare
// `tally` / `value` identifiers. The walk is in-place but the caller
// owns the V1 AST by this point (it's being discarded after translation).
// Returns (rewritten, unsafeRef) where unsafeRef is true when we found a
// reference to `<paramName>` outside the .tally/.value pattern — the
// caller should bail on the rewrite in that case.
// foldCtxMatch parameterises the fold-context walk for the two V1 shapes: the
// explicit-lambda form (context is a named param) and the query form (context
// is `this`).
type foldCtxMatch struct {
	// isBare reports whether e is a bare reference to the WHOLE context
	// (`param` / `this`) — which has no single-value V2 equivalent (unsafe).
	isBare func(v1ast.Expr) bool
	// isRecv reports whether e is the context used as a field-access receiver
	// (so `<ctx>.tally` / `<ctx>.value` can be rewritten to bare idents).
	isRecv func(v1ast.Expr) bool
	// skipLambda reports whether a nested lambda rebinds the context and so
	// must not be descended into.
	skipLambda func(*v1ast.Lambda) bool
}

// rewriteFoldContext rewrites a V1 fold body given as an explicit lambda
// `param -> ...`, mapping `param.tally` / `param.value` to bare tally/value.
func rewriteFoldContext(e v1ast.Expr, paramName string) (v1ast.Expr, bool) {
	return rewriteFoldCtx(e, foldCtxMatch{
		isBare: func(x v1ast.Expr) bool { id, ok := x.(*v1ast.Ident); return ok && id.Name == paramName },
		isRecv: func(x v1ast.Expr) bool { id, ok := x.(*v1ast.Ident); return ok && id.Name == paramName },
		// Only a nested lambda that SHADOWS the param rebinds the context.
		skipLambda: func(l *v1ast.Lambda) bool { return l.Param == paramName },
	})
}

// rewriteFoldContextThis rewrites a V1 fold body given in query form, where
// `this` is the {tally, value} context (e.g. `.fold({}, this.tally.merge(...))`),
// mapping `this.tally` / `this.value` to bare tally/value. Any nested lambda
// rebinds `this`, so those are not descended into.
func rewriteFoldContextThis(e v1ast.Expr) (v1ast.Expr, bool) {
	isThis := func(x v1ast.Expr) bool { _, ok := x.(*v1ast.ThisExpr); return ok }
	return rewriteFoldCtx(e, foldCtxMatch{
		isBare:     isThis,
		isRecv:     isThis,
		skipLambda: func(*v1ast.Lambda) bool { return true },
	})
}

func rewriteFoldCtx(e v1ast.Expr, m foldCtxMatch) (v1ast.Expr, bool) {
	unsafe := false
	var walk func(v1ast.Expr) v1ast.Expr
	walk = func(e v1ast.Expr) v1ast.Expr {
		if e == nil {
			return nil
		}
		if m.isBare(e) {
			// Bare reference to the whole context — cannot safely rewrite.
			unsafe = true
			return e
		}
		switch n := e.(type) {
		case *v1ast.FieldAccess:
			if m.isRecv(n.Recv) {
				switch n.Seg.Name {
				case "tally":
					return &v1ast.Ident{Name: "tally", TokPos: n.Recv.NodePos()}
				case "value":
					return &v1ast.Ident{Name: "value", TokPos: n.Recv.NodePos()}
				default:
					// <ctx>.something_else — unexpected, bail.
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
			// A nested lambda that rebinds the context binds a fresh value;
			// don't descend into it (the context inside is different).
			if m.skipLambda(n) {
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
