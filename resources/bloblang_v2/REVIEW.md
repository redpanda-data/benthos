# Bloblang V2 Spec Review

## High Priority Issues

### ~~1. `deleted()` vs void — overlapping behavior creates confusion~~

**Resolved:** Void is now an error in collection literals (arrays and objects). The only remaining valid context for void is assignments, where it causes a no-op (assignment skipped). `deleted()` is the sole mechanism for omitting elements from collections. This eliminates the overlap — void and `deleted()` now have clearly distinct roles.

---

## Medium Priority Issues

### ~~2. `output@` vs `output` deletion asymmetry is surprising~~

**Resolved:** `output@ = deleted()` is now an error — metadata is always an object and cannot be deleted. To clear all metadata keys, use `output@ = {}`. To replace metadata entirely, assign an object literal: `output@ = {"key": "value"}`. This makes `deleted()` consistent: it always means "remove this thing," and since metadata can't be removed, it's an error.

### ~~3. `uint64 + int64` promotion silently loses data~~

**Resolved:** Promotion is now checked at runtime. uint64 → int64 errors if the value exceeds 2^63-1. Integer → float64 errors if the integer magnitude exceeds 2^53 (cannot be represented exactly). All lossless promotions (int32 → int64, float32 → float64, etc.) proceed without checks.

### ~~4. Integer overflow is implementation-defined~~

**Resolved:** Integer arithmetic overflow is now always a runtime error. Implementations must detect overflow and throw an error rather than wrapping or saturating. This applies to all integer types and all arithmetic operators.

### ~~5. `match` without `as` — boolean case expressions have surprising behavior~~

**Resolved:** In equality match (`match expr { cases }`), if a case expression evaluates to a boolean, a runtime error is thrown. This catches the common mistake of writing boolean conditions in equality match. Users must use `as` for boolean conditions, or `if`/`else` to match against boolean values directly.

### ~~6. `else if` without final `else` — produces void silently~~

**Kept as-is:** This is consistent with the general `if`-without-`else` producing void behavior. The void semantics are well-documented (Section 4.1) and the pattern is useful.

### ~~7. Methods used in examples but not defined in the spec~~

**Resolved:** Created Section 13 (Standard Library) as the canonical reference for all required functions and methods. All previously-undefined methods (`.contains()`, `.flat_map()`, `.take()`, `.drop()`, `.any()`, `.all()`, `.join()`, `.fold()`, `.parse_json()`, etc.) are now defined there, along with many additional methods inspired by the original Bloblang language.

### ~~8. No mechanism to convert codepoint (int32) back to a string character~~

**Resolved:** Added `char(codepoint)` function to the standard library (Section 13.1). `char(104)` → `"h"`. This is the inverse of string indexing.

### ~~9. `map_object` cannot transform keys~~

**Resolved:** Added `.map_keys(key -> expr)` to the standard library (Section 13.6). This transforms keys while preserving values, complementing `.map_object()` which transforms values while preserving keys.

---

## Low Priority / Nitpicks

### ~~10. `match as` binding scope not explicitly stated for result expressions~~

**Resolved:** Section 4.2 now explicitly states that the `as` binding is available in both case conditions and result expressions.

### ~~11. Grammar's `match_expr` doesn't distinguish equality vs boolean forms~~

**Resolved:** Added a comment block in the grammar (Section 10) explicitly noting that the three match forms share the same syntactic production, and listing the semantic rules that a post-parse pass must enforce.

### ~~12. String/byte indexing returns int32, but literals default to int64~~

**Resolved:** Added a note in Section 3.1 clarifying that string indexing returns int32 while literals are int64, and that the int32→int64 promotion is always lossless. Also noted that `char()` converts codepoints back to strings.
