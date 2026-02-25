# Bloblang V2 Spec Review: Inconsistencies and Ambiguities

**Reviewed:** 2026-02-17

---

## Ambiguities (Underspecified Behavior)

### 11. Object key ordering

The spec never states whether object key insertion order is preserved. This matters for `map_object` and general object construction. In JSON, key order is unspecified, but many languages guarantee insertion order. Users of `.map_object()` might rely on ordering.

---

## Design Concerns (Consistent but Potentially Surprising)

### 12. Operator precedence: unary minus with method calls

Section 3.2 puts field access/method calls (`.`) at higher precedence than unary minus (`-`). This means:

```bloblang
-10.string()     # Parsed as -(10.string()) = -("10") -> ERROR
(-10).string()   # Must use parens
```

This is internally consistent but likely to surprise users who write `-10.string()` expecting it to work.

### 13. Inner blocks always shadow, never modify outer variables

Section 3.8 is clear that `$x = value` in an inner block always creates a new variable (shadowing). You can **never** modify an outer variable from within an if/match/lambda block:

```bloblang
$count = 0
if input.flag {
  $count = 1    # Shadows -- outer $count stays 0
}
output.count = $count  # Always 0
```

This is consistent with the functional design philosophy, but could surprise users expecting imperative-style conditional variable modification. The correct pattern is:
```bloblang
$count = if input.flag { 1 } else { 0 }
```

### 14. `.or()` laziness is implied but never stated

Section 8.4 shows:
```bloblang
output.name = input.name.or(throw("name is required")).catch(err -> "Anonymous")
```

This implies `.or()` only evaluates its argument when the value is null (otherwise `throw()` would always fire). But lazy/eager evaluation of `.or()`'s argument is never explicitly specified. An eager implementation would break this pattern.

### 15. Method definitions are deferred to implementation

Section 12.1 lists "required" methods (`.first()`, `.last()`, `.sort()`, `.round()`, etc.) but never defines their behavior. `.first()` on an empty array -- returns null or throws? `.sort()` on mixed types -- error or implicit ordering? `.round()` -- banker's rounding or half-up? These are referenced in examples (Sections 4.3, 8.6, 11) without formal definitions.

### 16. `(except assignment)` in Section 9.2 deleted() error list

Under "Causes runtime error," the spec says:
> Used as function arguments (except assignment): `some_function(deleted())`

The parenthetical "(except assignment)" is unexplained. Assignment is not a function call context. This appears to be a leftover from editing.

---

## Minor Grammar/Editorial Issues

### 17. Non-associativity not reflected in grammar or Key Points

The grammar's `binary_expr` production doesn't encode precedence or the non-associativity constraint. The Key Points section discusses precedence but omits non-associativity, even though Section 3.2 calls chaining non-associative operators a parse error.

### 18. Void not mentioned in grammar Key Points

The grammar and Key Points section never mention **void** as a concept, despite it arising directly from the `if_expr`-without-else grammar rule and having complex semantic implications described at length in Section 4.1.

### 19. Section 4.2 leading example uses atypical match form

Section 4.2 leads with an example that uses the `as` boolean match form for a task that Section 3.5 and every other section shows using the simpler equality match form, creating a misleading first impression about which form is typical.
