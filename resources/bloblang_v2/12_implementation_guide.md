# 12. Implementation Guide

## 12.1 Standard Library

See **[Section 13: Standard Library](13_standard_library.md)** for the complete reference of all required functions and methods. Implementations may provide additional functions and methods beyond the standard library.

## 12.2 Optional Optimizations

Implementations may optimize without changing observable behavior. Results must be identical with or without optimization.

### Lazy Evaluation (Iterators)

**Strategy:** Methods may return internal iterators instead of materializing arrays immediately.

**Lazy methods (from standard library):** `.filter()`, `.map_array()`. Additional extension methods like `.flat_map()`, `.take()`, `.drop()`, `.take_while()`, `.skip_while()` may also be made lazy if offered.

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
  .map_array(x -> x.value)
# Single pass, no intermediate array

# Variable breaks chain (materializes)
$filtered = input.items.filter(x -> x.active)  # Materializes
output.values = $filtered.map_array(x -> x.value)  # Second pass
```

**Benefit:** 10-100x faster for large datasets, no intermediate allocations.

**Guarantee:** Variables always hold arrays (never iterators), fully reusable.

### Pipeline Fusion

Combine multiple operations into single loop:
```bloblang
# User code
output.results = input.items
  .filter(x -> x.active)
  .map_array(x -> x.value * 2)
  .filter(x -> x > 0)

# Implementation may fuse into:
# for item in items:
#   if item.active and item.value * 2 > 0:
#     yield item.value * 2
```

### Early Termination

`.any()`, `.all()` should stop processing when result is determined:
```bloblang
input.items.any(x -> x > 100) # Stop at first match
input.items.all(x -> x > 0)   # Stop at first non-match
```

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

## 12.3 Error Messages

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

## 12.4 Performance Expectations

**Lazy evaluation benefits:**
- Filter + Map + Take (10K items): 10-15x faster
- Long pipeline (1M items): 3-5x faster, 99% less memory
- Early termination (find first in 1M): 100-1000x faster

## 12.5 Testing Requirements

- Results must match eager evaluation exactly
- Variable materialization must be transparent
- Iterator consumption must not leak to user code
- All examples in spec must execute correctly
