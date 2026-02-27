# Bloblang V2 Spec Review

## High Priority Issues

### 1. `deleted()` vs void — overlapping behavior creates confusion

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

Having two concepts that look similar but differ in assignments is a cognitive burden. (Note: the original `map_array` discrepancy has been resolved — void is now an error in `map_array`/`map_object`, so the overlap is reduced to collection literals and assignments.)

---

## Medium Priority Issues

### 2. `output@` vs `output` deletion asymmetry is surprising

`output = deleted()` removes the document (reads as null, field assignment errors). `output@ = deleted()` clears all keys but the object remains (key assignment still works). The spec explains this but the asymmetry violates the "Consistent Syntax" principle — identical syntax (`X = deleted()`) has fundamentally different semantics based on `X`.

### 3. `uint64 + int64` promotion silently loses data

The promotion rule `signed + unsigned integer → int64` means uint64 values above 2^63-1 silently overflow. The spec notes this but doesn't explain why int64 was chosen over erroring. For a language that claims "Explicit Context Management" and "Fail Loudly," silent overflow of a promotion rule is inconsistent with the design philosophy.

### 4. Integer overflow is implementation-defined

Section 2.3: "Overflow behavior for integer arithmetic is implementation-defined. Implementations may wrap, saturate, or error." This means the same Bloblang program can produce different results across implementations — directly contradicting the "Consistent Syntax" and "Predictable behavior" goals.

### 5. `match` without `as` — boolean case expressions have surprising behavior

In `match expr { cases }` (equality form), case expressions are evaluated and compared by equality. A user might accidentally write:

```bloblang
match input.score {
  input.score >= 100 => "gold",   # Compares input.score == true/false, NOT a range check
}
```

This evaluates `input.score >= 100` to a boolean, then compares `input.score == true/false`. The spec has three match forms to handle this, but there's no guard against using the wrong form — it silently does the wrong thing.

### 6. `else if` without final `else` — produces void silently

```bloblang
output.x = if a { 1 } else if b { 2 }
```

If both `a` and `b` are false, this produces void (assignment skipped). The presence of `else if` may mislead users into thinking cases are exhaustive. Compare with match, where non-exhaustive is an explicit error.

### 7. Methods used in examples but not defined in the spec

`.contains()` is used in Section 8.7 (`["int32", "int64"].contains(input.value.type())`) but is not listed in the required methods of Section 12.1. Similarly, `.flat_map()`, `.take()`, `.drop()`, `.any()`, `.all()`, `.join()`, `.fold()`, and others appear in the optimization section (12.2) as lazy/terminal methods but are never defined. The spec should either define these or mark them as optional with a clear note.

### 8. No mechanism to convert codepoint (int32) back to a string character

String indexing returns int32 codepoint values (`"hello"[0]` → `104`), but the spec provides no function to convert back (e.g., `char(104)` → `"h"`). This makes string indexing a one-way operation, limiting its usefulness.

### 9. `map_object` cannot transform keys

`map_object((key, value) -> expr)` only replaces the value; keys are always preserved. There's no built-in way to rename or transform object keys in a single operation. This forces awkward workarounds for a common transformation need.

---

## Low Priority / Nitpicks

### 10. `match as` binding scope not explicitly stated for result expressions

Section 4.3 shows `c` used in result bodies of match arms, but the prose in Section 4.2 never explicitly states that the `as` binding is available in both case conditions AND result expressions. It's implied by examples but should be stated.

### 11. Grammar's `match_expr` doesn't distinguish equality vs boolean forms

The grammar uses the same `expr_match_case` production for all three match forms. The equality-vs-boolean distinction is purely semantic, not syntactic. This is fine for implementation but means the grammar alone is insufficient to understand the language — it requires the prose from Section 4.2.

### 12. String/byte indexing returns int32, but literals default to int64

`"hello"[0]` returns `104` as int32, while `104` as a literal is int64. So `"hello"[0] == 104` requires implicit int32→int64 promotion. This works correctly per the promotion rules but is a minor surprise.
