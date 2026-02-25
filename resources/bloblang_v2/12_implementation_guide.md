# 12. Implementation Guide

## 12.1 Built-in Functions & Methods

**Reference:** Full list available via `rpk connect blobl --list-functions` and `--list-methods`

**Common functions:** `uuid_v4()`, `now()`, `random()`, `deleted()`

**Common methods:**
- String: `.uppercase()`, `.lowercase()`, `.trim()`, `.split()`, `.replace_all()`
- Array: `.filter()`, `.map_array()`, `.sort()`, `.length()`, `.first()`, `.last()`
- Object: `.map_object()`
- Type: `.type()`, `.string()`, `.int32()`, `.int64()`, `.uint32()`, `.uint64()`, `.float32()`, `.float64()`, `.bool()`, `.bytes()`
- Time: `.ts_parse()`, `.ts_format()`, `.ts_unix()`
- Error: `.catch()`, `.or()`

### map_array and map_object Semantics

**`.map_array(elem -> expr)`** — Transforms each element of an array. Returns a new array.
- Lambda receives each element as a single parameter
- Lambda return value replaces the element
- If the lambda returns `deleted()`, the element is omitted from the result

```bloblang
[1, 2, 3].map_array(x -> x * 2)                              # [2, 4, 6]
[1, -2, 3].map_array(x -> if x > 0 { x } else { deleted() }) # [1, 3]
```

**`.map_object((key, value) -> expr)`** — Transforms each value of an object. Returns a new object with the same keys.
- Lambda receives the key (string) and value as two parameters
- Lambda return value replaces the value for that key (key is preserved)
- If the lambda returns `deleted()`, the key-value pair is removed from the result
- Result is always an object (may be empty if all pairs are deleted)

```bloblang
{"a": 1, "b": 2}.map_object((k, v) -> v * 10)                          # {"a": 10, "b": 20}
{"a": 1, "b": -2, "c": 3}.map_object((k, v) -> if v > 0 { v } else { deleted() })  # {"a": 1, "c": 3}
{"x": "hello"}.map_object((k, v) -> v.uppercase())                     # {"x": "HELLO"}
```

**All methods listed above are required.** The type conversion methods (`.int32()`, `.uint32()`, etc.) are the only way to create non-default numeric types since literals are always int64 or float64. Implementations may provide additional methods (e.g., `.get()`, `.without()`, `.merge()`, `.append()`, `.parse_json()`) that are useful but not required by this specification. Consult implementation documentation for complete method listing.

## 12.2 Optional Optimizations

Implementations may optimize without changing observable behavior. Results must be identical with or without optimization.

### Lazy Evaluation (Iterators)

**Strategy:** Methods may return internal iterators instead of materializing arrays immediately.

**Lazy methods:** `.filter()`, `.map_array()`, `.flat_map()`, `.take()`, `.drop()`, `.take_while()`, `.skip_while()`

**Terminal methods:** `.sort()`, `.reverse()`, `.length()`, `.first()`, `.last()`, `.any()`, `.all()`, `.join()`, `.fold()`

**Materialization points:**
- Variable assignment: `$var = iterator` → array
- Output assignment: `output.x = iterator` → array
- Indexing: `iterator[0]` → array
- Terminal methods

**Example:**
```bloblang
# Direct chain (stays lazy)
output.top = input.items
  .filter(x -> x.active)
  .map_array(x -> x.value)
  .take(10)
# Single pass, processes only ~10 items

# Variable breaks chain (materializes)
$filtered = input.items.filter(x -> x.active)  # Materializes
output.top = $filtered.take(10)                 # Two passes
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

`.take()`, `.any()`, `.all()` should stop processing when result is determined:
```bloblang
input.items.take(10)          # Stop after 10 items
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
