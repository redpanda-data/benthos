# 18. Implementation Optimizations (Optional)

This section describes **optional optimization strategies** that implementations may use to improve performance. These are not required for correctness but can provide significant performance benefits.

---

## 18.1 Lazy Evaluation with Iterators

**Status:** Optional optimization strategy
**Benefit:** 10-100x performance improvement for functional pipelines
**User Impact:** Fully transparent - no code changes required

### Overview

Implementations may optimize functional method chains by using **lazy iterators** internally. This eliminates intermediate array allocations and enables early termination.

**Key Principle:** Iterators are an internal implementation detail. Users always work with arrays - iterators materialize automatically at assignment boundaries.

### Lazy vs Eager Methods

**Lazy Methods (May Return Internal Iterator):**
- `.filter(predicate)` - Lazy filtering
- `.map_each(transform)` - Lazy mapping
- `.flat_map(transform)` - Lazy flattening
- `.take(n)` - Take first n items
- `.drop(n)` / `.skip(n)` - Skip first n items
- `.take_while(predicate)` - Take while true
- `.skip_while(predicate)` - Skip while true

**Terminal Methods (Always Return Array/Value):**
- `.sort()` / `.sort_by(key)` - Must materialize to sort
- `.reverse()` - Must materialize to reverse
- `.length()` / `.count()` - Must count all items
- `.first()` / `.last()` - Return single value
- `.any(predicate)` / `.all(predicate)` - Return boolean
- `.join(separator)` - Return string
- `.fold(initial, fn)` / `.reduce(fn, initial)` - Return value

### Materialization Points

**Iterators must materialize at:**

1. **Variable assignment**
   ```bloblang
   $filtered = input.items.filter(x -> x.active)  # Must be array
   ```

2. **Output assignment**
   ```bloblang
   output.results = input.items.filter(x -> x.active)  # Must be array
   ```

3. **Metadata assignment**
   ```bloblang
   output@.items = input.items.filter(x -> x.active)  # Must be array
   ```

4. **Array indexing**
   ```bloblang
   $first = input.items.filter(x -> x.active)[0]  # Materialize then index
   ```

5. **Terminal method calls**
   ```bloblang
   $sorted = input.items.filter(x -> x.active).sort()  # Materialize then sort
   ```

6. **Function/map arguments**
   ```bloblang
   output.result = transform(input.items.filter(x -> x.active))  # Materialize
   ```

### Example: Transparent Optimization

**User Code (No Changes Required):**
```bloblang
# Users write normal functional code
output.results = input.items
  .filter(item -> item.active)
  .map_each(item -> item.value * 2)
  .filter(value -> value > 100)
  .take(10)
```

**Without Optimization (Eager):**
```
1. input.items → array[10000]
2. .filter() → allocate array[5000]
3. .map_each() → allocate array[5000]
4. .filter() → allocate array[1000]
5. .take(10) → allocate array[10]
Total: 4 intermediate allocations (21,000 items)
```

**With Optimization (Lazy):**
```
1. input.items → array[10000]
2. .filter() → iterator (no allocation)
3. .map_each() → iterator (no allocation)
4. .filter() → iterator (no allocation)
5. .take(10) → iterator (no allocation)
6. Assignment → materialize array[10]
Total: 1 materialization (10 items processed)
Improvement: 99% less memory, processes only what's needed
```

### Early Termination

Lazy evaluation enables early termination for operations like `.take()`, `.any()`, and `.all()`:

```bloblang
# Find first 5 expensive items in 1M item array
output.top_five = input.items
  .filter(item -> item.price > 1000)
  .take(5)

# Without optimization: Processes all 1M items
# With optimization: Stops after finding 5 matches
```

### Pipeline Fusion

Implementations may fuse multiple lazy operations into a single pass:

```bloblang
# User code
output.results = input.items
  .filter(item -> item.active)
  .map_each(item -> item.value)
  .filter(value -> value > 0)
  .map_each(value -> value * 2)

# Implementation may fuse into single loop:
# for item in items:
#   if item.active and item.value > 0:
#     yield item.value * 2
```

### Guaranteed Behavior

**What users can rely on:**

1. **Variables are always arrays:**
   ```bloblang
   $var = input.items.filter(x -> x.active)
   $var.type()  # Always "array", never "iterator"
   ```

2. **Variables are reusable:**
   ```bloblang
   $filtered = input.items.filter(x -> x.active)
   output.a = $filtered.take(5)
   output.b = $filtered.skip(5)  # Works - $filtered is an array
   ```

3. **Evaluation order is deterministic:**
   ```bloblang
   # Lambdas execute when chain materializes
   output.results = input.items
     .map_each(item -> item.value * 2)
   # All items processed before assignment
   ```

4. **No observable difference:**
   - Results are identical with or without optimization
   - Only performance characteristics change
   - Memory usage and timing may differ, but outputs are the same

### Trade-offs

**Optimization Through Direct Chaining:**
```bloblang
# ✅ Maximum optimization: Direct chain to output
output.top = input.items
  .filter(x -> x.active)
  .map_each(x -> x.value)
  .take(10)
# Single pass, early termination, lazy evaluation

# ⚠️ Less optimized: Variable breaks chain
$intermediate = input.items.filter(x -> x.active)  # Materializes here
output.top = $intermediate.map_each(x -> x.value).take(10)
# Two passes: filter materializes, then map+take
```

**When to use variables:**
- When you need to reuse data multiple times
- When intermediate results are useful
- When debugging (can inspect materialized values)

**When to use direct chains:**
- When you only need the final result
- When processing large datasets
- When maximum performance is important

### Implementation Notes

**For implementers:**

1. **Iterators are internal** - Never exposed to user code
2. **Must materialize at boundaries** - Variables, outputs, indexing
3. **Fusion is optional** - Single-operation iterators are valid
4. **Early termination** - `.take()`, `.any()`, `.all()` should stop early
5. **Memory safety** - Ensure no iterator lifecycle issues
6. **Testing** - Results must match eager evaluation exactly

### Performance Expectations

**Micro-benchmarks (reference):**

- **Filter + Map + Take (10K items):** 10-15x faster
- **Long pipeline (1M items):** 3-5x faster, 99% less memory
- **Early termination (find first in 1M):** 100-1000x faster

Actual improvements depend on dataset size, pipeline complexity, and implementation quality.

---

## 18.2 Other Optimization Opportunities

### String Builder Optimization

Implementations may optimize repeated string concatenation:

```bloblang
# User code
output.result = "a" + "b" + "c" + "d"

# Implementation may use string builder instead of creating intermediate strings
```

### Constant Folding

Implementations may evaluate constant expressions at parse time:

```bloblang
output.value = 2 + 2           # May compile to: output.value = 4
output.flag = true && false    # May compile to: output.flag = false
```

### Dead Code Elimination

Implementations may eliminate unreachable code:

```bloblang
output.value = if true {
  "always"
} else {
  "never"    # Unreachable - may be eliminated
}
```

### Map Inlining

Implementations may inline small, frequently-called maps:

```bloblang
map small_transform(x) {
  output = x * 2
}

# May be inlined at call sites for performance
output.result = small_transform(input.value)
```

---

## Summary

These optimizations are **optional** - implementations may choose which to implement based on complexity and benefit. The key requirement is that **optimized and non-optimized code must produce identical results**.

Users write the same code regardless of optimization level - the language semantics remain unchanged.
