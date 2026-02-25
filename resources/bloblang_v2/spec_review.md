# Bloblang V2 Spec Review: Inconsistencies and Ambiguities

**Reviewed:** 2026-02-25

---

## High Severity — Inconsistencies & Underspecification That Would Cause Implementation Disagreements

### 1. `stmt_body` grammar forbids empty blocks, making no-op match catch-alls impossible

The grammar defines `stmt_body := statement+` (one or more statements). Since non-exhaustive match statements **error at runtime** (Section 4.2), users need catch-all arms. But a "do nothing" catch-all is grammatically impossible:

```bloblang
match input.type {
  "special" => { output.handled = true },
  _ => { }  # PARSE ERROR: stmt_body requires at least one statement
}
```

Users must write a dummy statement like `_ => { $noop = null }`. The same problem affects `if_stmt` — you can't write `if condition { }` as a guard. The grammar should either allow empty statement bodies or provide an explicit no-op statement.

**Status:** Resolved — changed `stmt_body := statement+` to `stmt_body := statement*` in Section 10.

### 2. Wildcard `_` semantics undefined in boolean match forms

In **equality match** (`match expr { ... }`), `_` is a catch-all that matches any value. In **boolean match** (`match expr as x { ... }` or `match { ... }`), each case must be a **boolean expression**, and non-boolean cases throw errors (Sections 3.5, 4.2). But `_` is not a boolean — it's a wildcard.

Is `_` exempt from the boolean requirement? Every example uses `_` as a catch-all in boolean match forms, but the spec never states that `_` has special semantics in that context. An implementation that treats `_` as a non-boolean case value (and errors) would be spec-compliant per the literal text.

**Status:** Open

### 3. `.or()` evaluation strategy unspecified (lazy vs eager)

Section 8.4 shows this pattern:

```bloblang
output.name = input.name.or(throw("name is required")).catch(err -> "Anonymous")
```

This **only works** if `.or()` lazily evaluates its argument (skipping `throw()` when the value isn't null). An eager implementation would always call `throw()`, breaking the pattern. The spec never states whether `.or()` uses lazy or eager evaluation.

**Status:** Open

### 4. Import execution semantics unspecified

Section 6.4 says "All top-level maps are exported automatically. Variables are never exported." But it only discusses **visibility**, not **execution**. If an imported file has top-level statements:

```bloblang
# utils.blobl
$counter = 0
output.side_effect = "hello"
map transform(data) { data * 2 }
```

Are `$counter = 0` and the output assignment executed? In what context? The natural interpretation is that only map declarations are extracted, but this is never stated. Should imported files be forbidden from containing top-level statements?

**Status:** Open

### 5. Modulo (`%`) result type unspecified

Section 2.3 defines a special rule for division: "Division always produces a float." But `%` (modulo) has no result-type rule. Does `7 % 2` produce `int64(1)` or `float64(1.0)`? Does it follow the standard numeric promotion rules or the division-style "always float" rule? For `float64 % float64`, does it use `fmod` semantics?

**Status:** Open

### 6. Name resolution: map parameters vs global map names

Section 5.3 forbids parameter names from conflicting with **imported namespaces**, but says nothing about conflicting with **map names** in the same file:

```bloblang
map callback(data) { data * 2 }

map transform(data, callback) {
  callback(data.value)  # Is this the parameter or the global map?
}
```

Most languages would shadow (parameter wins), but the spec is silent. This is a different class of conflict from namespace conflicts and needs its own rule.

**Status:** Open

### 7. Grammar allows bare identifiers in top-level expression context

The grammar defines `expr_context_root := ('output' | 'input') metadata_accessor? | var_ref | identifier`. The trailing `identifier` alternative is intended for map parameters and match `as` bindings, but the grammar applies it in ALL expression contexts, including top-level RHS:

```bloblang
output.x = foo.bar  # Grammar accepts this even if 'foo' is not bound
```

The prose (Section 3.1) restricts bare identifiers to parameters and match bindings, but the grammar doesn't enforce this. A typo like `inpt.field` would parse successfully rather than failing at parse time.

**Status:** Open

---

## Medium Severity — Ambiguities That Could Cause Subtle Behavioral Differences

### 8. Negative indexing scope: Section 3.1 vs Section 10

**Section 3.1** explicitly states negative indexing works for "arrays, strings, and bytes" and shows examples for all three. **Section 10** (Grammar Key Points) states "Negative indices supported for arrays" — omitting strings and bytes. These contradict each other.

**Status:** Open

### 9. Map "purity" is incomplete with closure arguments

Section 5.4 claims maps are "pure functions: same inputs always produce same output." But lambdas capture variables **by reference** (Section 3.4), so a closure passed to a map carries mutable top-level state:

```bloblang
$multiplier = 2
$fn = x -> x * $multiplier

map apply(data, callback) { callback(data) }

output.a = apply(5, $fn)  # 10
$multiplier = 3
output.b = apply(5, $fn)  # 15 — same map, same lambda, different result
```

The map is deterministic given its exact inputs (including closure state), but the claim of purity becomes misleading since closures are opaque carriers of mutable external state.

**Status:** Open

### 10. Metadata copy depth unspecified

`output@ = input@` copies all metadata (Section 7.4), but the spec never says whether this is a **deep copy** or a **shallow reference**. If metadata values are objects/arrays, this matters:

```bloblang
output@ = input@
output@.tags[0] = "modified"  # Does this also modify input@.tags[0]?
```

Given that `input` is immutable, a deep copy is implied — but it should be explicit.

**Status:** Open

### 11. Object key ordering unspecified

The spec never states whether object key insertion order is preserved. This matters for `map_object` iteration order, JSON serialization order, and equality semantics (`{"a":1,"b":2} == {"b":2,"a":1}` — true or false?). Section 2.3 says object equality is "structural equality (value-based)" but doesn't address key order.

**Status:** Open

### 12. Required method behavior undefined

Section 12.1 lists required methods (`.first()`, `.last()`, `.sort()`, `.round()`, etc.) but never defines their behavior:
- `.first()` / `.last()` on empty array — `null` or error?
- `.sort()` on mixed types — error or implicit ordering?
- `.sort()` sort stability — guaranteed or not?
- `.round(n)` — half-up, half-even (banker's), or implementation-defined?

These are all used in examples (Sections 4.3, 8.6, 11) without formal definitions.

**Status:** Open

### 13. Grammar doesn't encode precedence or non-associativity

The grammar's `binary_expr := expression binary_op expression` is a flat production that doesn't reflect the precedence table (Section 3.2) or the non-associativity constraint. Section 3.2 says chaining non-associative operators is a "parse error," but the grammar accepts `a < b < c`. Implementers must derive precedence and associativity rules entirely from prose.

**Status:** Open

### 14. Void is semantically significant but absent from formal spec

Void has complex context-dependent behavior described at length in Section 4.1 (15+ paragraphs, a behavior table), but:
- Not listed as a type in Section 2.1
- Not mentioned in the grammar (Section 10)
- Not mentioned in the Grammar Key Points
- `.type()` behavior on void is unspecified

This means the formal grammar and type system don't model one of the language's most nuanced semantic concepts.

**Status:** Open

### 15. Reading `output` during construction has implicit ordering dependencies

Section 7.7 shows `output.tax = output.price * 0.1` which reads a previously-assigned field. But reading a not-yet-assigned field silently returns `null`:

```bloblang
output.a = output.b  # null (b not yet assigned) — silent, no error
output.b = 42
```

This is consistent with "non-existent fields return null" (Section 7.1), but creates a class of subtle ordering bugs that could never be caught statically. The spec should acknowledge this or consider making it an error.

**Status:** Open

---

## Low Severity — Design Concerns & Editorial Issues

### 16. Unary minus precedence with method calls

Section 3.2 puts field access/method calls (`.`) at higher precedence than unary minus (`-`). This means:

```bloblang
-10.string()     # Parsed as -(10.string()) = -("10") -> ERROR
(-10).string()   # Must use parens
```

Internally consistent but likely to surprise users who write `-10.string()` expecting it to work.

**Status:** Open

### 17. Inner blocks always shadow, never modify outer variables

Section 3.8 is clear that `$x = value` in an inner block always creates a new variable (shadowing). You can **never** modify an outer variable from within an if/match/lambda block:

```bloblang
$count = 0
if input.flag {
  $count = 1    # Shadows — outer $count stays 0
}
output.count = $count  # Always 0
```

Consistent with the functional design philosophy, but could surprise users expecting imperative-style conditional variable modification. The correct pattern is:
```bloblang
$count = if input.flag { 1 } else { 0 }
```

**Status:** Open

### 18. "(except assignment)" parenthetical in Section 9.2

Under "Causes runtime error," the spec says:
> Used as function arguments (except assignment): `some_function(deleted())`

The parenthetical "(except assignment)" is unexplained. Assignment is not a function call context. This appears to be a leftover from editing.

**Status:** Open

### 19. Section 4.2 leading example uses atypical match form

Section 4.2 leads with an example that uses the `as` boolean match form for a task that Section 3.5 and every other section shows using the simpler equality match form, creating a misleading first impression about which form is typical.

**Status:** Open

### 20. `filter` receiving void from lambda is error, but `map_array` skips — asymmetry

Section 4.1 says `filter` receiving void is an error, while `map_array` receiving void skips the element. The distinction is that `filter` requires a boolean return, but this creates an asymmetry where `if x > 0 { true }` works in `map_array` context but fails in `filter` context — even though both use the same if-without-else pattern.

**Status:** Open
