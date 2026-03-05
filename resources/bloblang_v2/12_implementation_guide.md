# 12. Implementation Guide

## 12.1 Standard Library

See **[Section 13: Standard Library](13_standard_library.md)** for the complete reference of all required functions and methods. Implementations may provide additional functions and methods beyond the standard library.

## 12.2 Optional Optimizations

Implementations may optimize without changing observable behavior. Results must be identical with or without optimization.

### Lazy Evaluation (Iterators)

**Strategy:** Methods may return internal iterators instead of materializing arrays immediately.

**Lazy methods (from standard library):** `.filter()`, `.map()`. Additional extension methods like `.flat_map()`, `.take()`, `.drop()`, `.take_while()`, `.skip_while()` may also be made lazy if offered.

**Terminal methods (from standard library):** `.sort()`, `.reverse()`, `.length()`, `.any()`, `.all()`, `.join()`, `.fold()`

**Materialization points:**
- Variable assignment: `$var = iterator` → array
- Output assignment: `output.x = iterator` → array
- Indexing: `iterator[0]` → array
- Terminal methods

**Example:**
```bloblang
# Direct chain (stays lazy)
output.active_values = input.items
  .filter(x -> x.active)
  .map(x -> x.value)
# Single pass, no intermediate array

# Variable breaks chain (materializes)
$filtered = input.items.filter(x -> x.active)  # Materializes
output.values = $filtered.map(x -> x.value)  # Second pass
```

**Benefit:** 10-100x faster for large datasets, no intermediate allocations.

**Guarantee:** Variables always hold arrays (never iterators), fully reusable.

### Pipeline Fusion

Combine multiple operations into single loop:
```bloblang
# User code
output.results = input.items
  .filter(x -> x.active)
  .map(x -> x.value * 2)
  .filter(x -> x > 0)

# Implementation may fuse into:
# for item in items:
#   if item.active and item.value * 2 > 0:
#     yield item.value * 2
```

### Early Termination

**Note:** `.any()` and `.all()` short-circuiting is a **required semantic**, not an optional optimization — see Section 13.6. Implementations must not evaluate elements beyond the determined result.

### Constant Folding

Evaluate constant expressions at parse time:
```bloblang
output.value = 2 + 2  # May compile to: output.value = 4
```

### Dead Code Elimination

Remove unreachable code:
```bloblang
if true {
  output.a = "always"
} else {
  output.b = "never"  # May be eliminated
}
```

### String Builder

Optimize repeated concatenation:
```bloblang
"a" + "b" + "c" + "d"  # May use string builder instead of intermediate strings
```

## 12.3 Intrinsic Methods

`.catch()` and `.or()` are parsed as regular method calls (via the `method_call` postfix operation in `postfix_expr`) but require special handling by the runtime. They **cannot** be implemented as ordinary methods:

- **`.catch(err -> expr)`** — Must intercept errors from the left-hand expression chain. Normal methods are skipped when the receiver is an error; `.catch()` is the opposite — it activates only on errors and passes through successful values unchanged. See Section 8.2.
- **`.or(default)`** — Must use short-circuit evaluation. Normal methods eagerly evaluate all arguments; `.or()` must *not* evaluate its argument unless the receiver is null, void, or `deleted()`. Additionally, `.or()` and `.catch()` are the only methods that can be called on void or `deleted()` — all other methods error on void and `deleted()` receivers. `.catch()` passes void and `deleted()` through unchanged (they are not errors); `.or()` actively rescues them. This matters when the argument has side effects or throws (e.g., `.or(throw("required"))`). See Section 8.3.

Implementations should recognize these during compilation/interpretation and emit specialized instructions rather than routing them through the general method dispatch path.

## 12.4 Error Messages

Provide clear error messages with context:
```
mapping.blobl:15:22: Type mismatch: cannot add int64 and string
  output.result = 5 + "3"
                      ^^^
```

Include:
- File name and location
- Clear description
- Suggested fix when possible

## 12.5 Performance Expectations

**Lazy evaluation benefits:**
- Filter + Map + Take (10K items): 10-15x faster
- Long pipeline (1M items): 3-5x faster, 99% less memory
- Early termination (find first in 1M): 100-1000x faster

## 12.6 Testing Requirements

- Results must match eager evaluation exactly
- Variable materialization must be transparent
- Iterator consumption must not leak to user code
- All examples in spec must execute correctly
