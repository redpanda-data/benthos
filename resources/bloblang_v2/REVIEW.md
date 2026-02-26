# Bloblang V2 Spec Review

## Critical Issues

### 1. Void has contradictory semantics across contexts

Void means opposite things depending on where it appears:

- **In collection literals:** void **skips** the element — `[1, if false { 2 }, 3]` → `[1, 3]`
- **In `map_array` return:** void **keeps the element unchanged** — `[1, -2, 3].map_array(x -> if x > 0 { x * 10 })` → `[10, -2, 30]`

The same concept ("no value was produced") results in *removal* in one context and *identity/preservation* in another. This is the single most confusing aspect of the spec and will be a persistent source of bugs. The rationale ("no value to include" vs "no transformation to apply") is defensible but violates the spec's own "One Clear Way" principle.

### 2. Variable path assignment (`$obj.field = value`) — undefined behavior

The grammar explicitly allows this:
```
top_level_path      := top_level_context_root path_component*
top_level_context_root := 'output' metadata_accessor? | var_ref
```

So `$obj.field = value` parses as a valid assignment. But the spec **never discusses** whether variables support field/index assignment or only whole-variable reassignment. All examples show only `$var = expr`. This leaves a fundamental question unanswered: are variable values deeply mutable or only rebindable? If deep mutation is allowed, what are the COW semantics? If not allowed, the grammar is overly permissive.

### 3. Grammar ambiguity: `foo.bar(args)` — qualified function call vs method chain

The grammar has two valid derivations for `foo.bar(args)`:
- `function_call` via `qualified_name` (`foo.bar` as namespace.map)
- `method_chain` on expression `foo` (bare identifier) calling method `.bar(args)`

The parser cannot disambiguate without semantic knowledge (is `foo` a namespace or a parameter/variable?). The spec never addresses this. While implementable via semantic resolution, a formal grammar ambiguity is a spec defect — especially since the grammar section is labeled "Grammar Reference" and appears intended as a formal definition.

### 4. `output = input` copy semantics unspecified

For metadata, the spec explicitly states: "`output@ = input@` performs a **deep copy**." But for the document itself, `output = input` has no equivalent statement. Since input is immutable and output is mutable, modifications to output must not affect input — but whether this is achieved via deep copy, shallow copy, or COW is never specified. The asymmetry with the metadata section (where it IS specified) makes this feel like an oversight rather than intentional omission.

---

## High Priority Issues

### 5. Bytes not defined as a `+` operand

Section 2.3 defines `+` for strings (concatenation) and numerics (addition), and says all other cross-family mixes are errors. Bytes are never mentioned. Is `bytes + bytes` concatenation (by analogy with strings) or an error? Given that bytes are a distinct type from strings, the spec should state this explicitly.

### 6. Unary minus + method call precedence trap

Since field access/method calls (precedence 1) bind tighter than unary minus (precedence 2):

```bloblang
-10.string()    # Parses as -(10.string()) = -("10") = ERROR
```

This is consistent with the precedence rules, but the spec uses `-10` as a natural way to write negative numbers (Section 1.3: "negative numbers use unary minus: `-10`") without noting this interaction. Users will write `-10.string()` expecting `"-10"` and get a type error.

### 7. Statement-context variables may not exist at point of use

Section 3.8 shows that in statement `if`/`match`, new variables leak into the outer scope — but only if the branch executes:

```bloblang
if input.flag {
  $temp = "found"
}
output.temp = $temp    # ERROR if flag was false ($temp never created)
```

The spec acknowledges this but provides no mechanism to handle it (e.g., no way to check if a variable exists). This creates fragile code that works for some inputs and crashes for others — at odds with the "Fail Loudly" principle which implies *predictable* failure, not *conditional* failure.

### 8. Map "purity" is undermined by closure arguments

Section 5.3 states maps are "pure functions" where "the result is entirely determined by the parameter values." But the closure caveat immediately contradicts this:

```bloblang
$multiplier = 2
$fn = x -> x * $multiplier
map apply(data, callback) { callback(data) }
output.a = apply(5, $fn)  # 10
$multiplier = 3
output.b = apply(5, $fn)  # 15 — same inputs, different result
```

The spec acknowledges the issue but still calls maps "pure," which is misleading. They are pure only if you exclude lambdas-with-captures from the definition of "value."

### 9. `deleted()` vs void — overlapping behavior creates confusion

In collection literals, both produce identical results:
```bloblang
[1, deleted(), 3]          # [1, 3]
[1, if false { 2 }, 3]     # [1, 3]
```

But in assignments, they differ:
```bloblang
output.x = "prior"
output.x = if false { "new" }   # void: keeps "prior"
output.x = deleted()             # deleted: removes field
```

And in `map_array`, they differ yet again (void = keep unchanged, deleted = remove). Having three behaviors across three contexts for two concepts that look similar is a significant cognitive burden.

---

## Medium Priority Issues

### 10. `output@` vs `output` deletion asymmetry is surprising

`output = deleted()` removes the document (reads as null, field assignment errors). `output@ = deleted()` clears all keys but the object remains (key assignment still works). The spec explains this but the asymmetry violates the "Consistent Syntax" principle — identical syntax (`X = deleted()`) has fundamentally different semantics based on `X`.

### 11. `uint64 + int64` promotion silently loses data

The promotion rule `signed + unsigned integer → int64` means uint64 values above 2^63-1 silently overflow. The spec notes this but doesn't explain why int64 was chosen over erroring. For a language that claims "Explicit Context Management" and "Fail Loudly," silent overflow of a promotion rule is inconsistent with the design philosophy.

### 12. Integer overflow is implementation-defined

Section 2.3: "Overflow behavior for integer arithmetic is implementation-defined. Implementations may wrap, saturate, or error." This means the same Bloblang program can produce different results across implementations — directly contradicting the "Consistent Syntax" and "Predictable behavior" goals.

### 13. `match` without `as` — boolean case expressions have surprising behavior

In `match expr { cases }` (equality form), case expressions are evaluated and compared by equality. A user might accidentally write:

```bloblang
match input.score {
  input.score >= 100 => "gold",   # Compares input.score == true/false, NOT a range check
}
```

This evaluates `input.score >= 100` to a boolean, then compares `input.score == true/false`. The spec has three match forms to handle this, but there's no guard against using the wrong form — it silently does the wrong thing.

### 14. `else if` without final `else` — produces void silently

```bloblang
output.x = if a { 1 } else if b { 2 }
```

If both `a` and `b` are false, this produces void (assignment skipped). The presence of `else if` may mislead users into thinking cases are exhaustive. Compare with match, where non-exhaustive is an explicit error.

### 15. Methods used in examples but not defined in the spec

`.contains()` is used in Section 8.7 (`["int32", "int64"].contains(input.value.type())`) but is not listed in the required methods of Section 12.1. Similarly, `.flat_map()`, `.take()`, `.drop()`, `.any()`, `.all()`, `.join()`, `.fold()`, and others appear in the optimization section (12.2) as lazy/terminal methods but are never defined. The spec should either define these or mark them as optional with a clear note.

### 16. No mechanism to convert codepoint (int32) back to a string character

String indexing returns int32 codepoint values (`"hello"[0]` → `104`), but the spec provides no function to convert back (e.g., `char(104)` → `"h"`). This makes string indexing a one-way operation, limiting its usefulness.

### 17. `map_object` cannot transform keys

`map_object((key, value) -> expr)` only replaces the value; keys are always preserved. There's no built-in way to rename or transform object keys in a single operation. This forces awkward workarounds for a common transformation need.

---

## Low Priority / Nitpicks

### 18. `match as` binding scope not explicitly stated for result expressions

Section 4.3 shows `c` used in result bodies of match arms, but the prose in Section 4.2 never explicitly states that the `as` binding is available in both case conditions AND result expressions. It's implied by examples but should be stated.

### 19. Grammar's `match_expr` doesn't distinguish equality vs boolean forms

The grammar uses the same `expr_match_case` production for all three match forms. The equality-vs-boolean distinction is purely semantic, not syntactic. This is fine for implementation but means the grammar alone is insufficient to understand the language — it requires the prose from Section 4.2.

### 20. String/byte indexing returns int32, but literals default to int64

`"hello"[0]` returns `104` as int32, while `104` as a literal is int64. So `"hello"[0] == 104` requires implicit int32→int64 promotion. This works correctly per the promotion rules but is a minor surprise.
