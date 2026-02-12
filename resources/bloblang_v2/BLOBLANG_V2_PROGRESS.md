# Bloblang V2 Development Progress

**Last Updated:** 2026-02-11
**Status:** ✅ **COMPLETE**

## Overview

This document tracks the incremental development of Bloblang V2 enhancements. Each solution from PROPOSED_SOLUTIONS.md is being incorporated into the modular specification files in this directory one at a time.

**Language Features:** 11 completed ✅
**Optimization Strategies:** 1 documented (optional for implementers)
**Deferred Features:** 2 (can be added in future versions if needed)
**Out of Scope:** 4 solutions removed (tooling concerns or unnecessary complexity)

---

## Completed Solutions

### ✅ Solution 1: Block-Scoped Variables

**Implementation Date:** 2026-02-10
**Approach:** Option A - Block-scoped let (most intuitive, aligns with common programming languages)

### ✅ Solution 2: Explicit Context Management (ENHANCED)

**Implementation Date:** 2026-02-10
**Approach:** Complete removal of `this` keyword - all contexts must be explicitly named

**What Changed:**
- Removed `this` keyword entirely from the language
- Replaced with `input` keyword for input document reference
- Match expressions now require explicit `as name` binding
- Maps now require explicit parameter declarations
- Added `as` keyword to language

**Benefits:**
- Zero implicit context shifting
- Complete elimination of context confusion
- Code is always explicit about what data is being referenced
- Easier to read and reason about

### ✅ Pipe Operator Removal

**Implementation Date:** 2026-02-10

**What Changed:**
- Removed pipe operator `|` for coalescing
- Reserved `|` symbol for possible future features
- Updated examples to use `.or()` method chaining instead

**Migration:**
```bloblang
# Old: input.(primary_id | secondary_id | "default")
# New: input.primary_id.or(input.secondary_id).or("default")
```

### ✅ Keyword Consistency: `root` → `output`

**Implementation Date:** 2026-02-10

**What Changed:**
- Renamed `root` keyword to `output`
- Updated all documentation and examples
- Section "Root Context" renamed to "Output Context"

**Rationale:**
- Consistent with `input` keyword
- More explicit and clear
- `input` and `output` are symmetrical and intuitive

### ✅ Unified Metadata Syntax with Input/Output Model

**Implementation Date:** 2026-02-11

**What Changed:**
- Removed `meta foo =` syntax → use `output@.foo =` instead
- Removed `metadata("foo")` function → use `input@.foo` instead
- Removed standalone `@key` syntax
- Unified metadata with input/output model: `input@.key` (immutable) and `output@.key` (mutable)
- Metadata follows same immutability semantics as documents

**Examples:**
```bloblang
# Reading input metadata (immutable)
output.topic = input@.kafka_topic
output.key = input@.kafka_key

# Writing output metadata (mutable)
output@.kafka_topic = "processed"
output@.kafka_key = input.id

# Copy and modify
output@ = input@
output@.kafka_topic = "new-topic"

# Deleting metadata
output@.kafka_key = deleted()
```

**Rationale:**
- Explicit distinction between input (immutable) and output (mutable) metadata
- Consistent with document model (`input.field` / `output.field`)
- Makes immutability semantics clear
- No ambiguity about input vs output

### ✅ Simplified Variable Syntax

**Implementation Date:** 2026-02-10

**What Changed:**
- Removed `let variable =` syntax → use `$variable =` instead
- Removed `let` keyword entirely
- Same prefix for declaration and reference: `$`

**Examples:**
```bloblang
# Declaring variables
$user_id = input.user.id
$name = input.name

# Using variables
output.user_id = $user_id
output.display = $name.uppercase()

# Block-scoped variables
output.result = if input.enabled {
  $value = input.amount * 2
  $value.floor()
}
```

**Rationale:**
- Consistent with metadata syntax (both use prefix for declaration and reference)
- One clear way to declare variables
- Symmetry: `@` for metadata, `$` for variables
- Less keywords to learn

### ✅ Explicit Indexing with Negative Index Support (Arrays, Strings, Bytes)

**Implementation Date:** 2026-02-11

**What Changed:**
- Documented explicit indexing syntax for arrays, strings, and bytes
- Added support for negative indices (Python-style) for all indexable types
- Clarified error behavior and safe access patterns
- String indexing by byte position returns single-character string
- Bytes indexing returns numeric byte value (0-255)

**Examples:**
```bloblang
# Array indexing (0-based)
output.first = input.items[0]
output.second = input.items[1]
output.last = input.items[-1]           # Last element

# String indexing (byte position, returns single-char string)
output.first_char = input.text[0]       # First character
output.last_char = input.text[-1]       # Last character
output.initial = input.name[0]          # First letter

# Bytes indexing (returns number 0-255)
output.first_byte = input.data[0]       # First byte as number
output.last_byte = input.data[-1]       # Last byte as number

# Dynamic indexing
output.element = input.items[input.position]
output.nested = input.users[$index].name

# Safe access
output.safe = input.items[0].catch(null)
output.char = input.text[5].catch("")
output.byte = input.data[100].catch(0)

# Chained access
output.value = input.data[2].items[5].name
output.initial = input.users[0].name[0]  # First char of first user's name
```

**Negative Index Semantics:**
- `-1` accesses the last element/character/byte
- `-n` accesses the nth element/character/byte from the end
- For collection of length N, index `-i` is equivalent to `N-i`

**Return Types:**
- **Arrays**: Returns element at position (any type)
- **Strings**: Returns single-character string at byte position
- **Bytes**: Returns byte value as number (0-255)

**String Indexing Note:**
- Indexing is by byte position, not character/rune position
- Multi-byte UTF-8 characters may be split if indexed in the middle
- Returns single-byte string (may be invalid UTF-8 for multi-byte chars)

**Error Behavior:**
- Out-of-bounds access (positive or negative) throws mapping error for all types
- Use `.catch()` for safe access with fallback values
- Non-integer index throws error
- Indexing unsupported types (number, boolean, object) throws error

**Specification Updates:**
- Section 3 - Enhanced type descriptions with indexing details for arrays, strings, and bytes
- Section 4.1.1 (UPDATED) - Comprehensive indexing documentation for all indexable types
- Section 9.1 - Added indexing error examples for arrays, strings, and bytes
- Section 14.5 (UPDATED) - Added indexing patterns for all indexable types
- Section 15 - Fixed grammar to support `path[index]` without dot, documented multi-type support
- README - Added indexing quick start examples for arrays and strings

**Grammar Correction:**
- Changed `path := base ('.' field_access)*` where `field_access` includes `'[' expr ']'`
- To `path := base path_component*` where `path_component := '.' field_name | '[' expr ']'`
- This correctly allows `input.foo[0]` instead of requiring `input.foo.[0]`

**Rationale:**
- Common user request for explicit element/character/byte access
- Negative indexing provides ergonomic access to tail of collections
- String indexing enables character extraction without methods
- Bytes indexing provides direct numeric byte value access
- Consistent with Python, Ruby, and other modern languages
- Grammar was close but needed correction for proper bracket syntax

**What Changed:**
- Variables can now be declared inside `if`, `else if`, `else`, and `match` expression branches
- Variables can be declared inside lambda bodies (multi-statement lambdas not yet implemented)
- Variables are lexically scoped to their declaring block
- Inner scopes can shadow outer scope variables
- Variables are automatically deallocated when scope exits

**Specification Updates:**
- Section 5.3 (Variable Declaration) - Added block-scoped variable syntax and examples
- Section 6.1 (If Expression) - Added V2 examples with scoped variables
- Section 6.2 (If Statement) - Added V2 examples with scoped variables
- Section 6.3 (Match Expression) - Added V2 examples with scoped variables
- Section 12.3 (Variable Scope) - Complete rewrite explaining block scoping, shadowing, lifetime
- Section 14.3 (Error-Safe Parsing) - Updated pattern to show V2 improvement
- Section 14.7 (NEW) - Added comprehensive V2 examples
- Section 15 (Grammar Summary) - Added note about var_decl in blocks
- V2 Implementation Notes section added

**Backward Compatibility:** ✅ Fully backward compatible
- All V1 mappings continue to work unchanged
- New feature is opt-in

**Next Steps for Implementation:**
1. Parser changes to allow `let` in block contexts
2. Scope stack implementation in interpreter
3. Variable resolution with scope chain lookup
4. Shadow warning/detection in linter
5. Comprehensive test suite
6. Documentation and examples

### ✅ Null-Safe Navigation Operators

**Implementation Date:** 2026-02-11

**What Changed:**
- Added `?.` operator for null-safe field access
- Added `?[` operator for null-safe array indexing
- Both operators short-circuit to `null` if the left operand is `null`

**Examples:**
```bloblang
# Null-safe field access
output.city = input.user?.address?.city           # null if any field is null
output.email = input.contact?.primary?.email      # chains safely

# Null-safe array indexing
output.first = input.items?[0]                    # null if items is null
output.last = input.data?[-1]                     # null if data is null

# Combined operations
output.name = input.users?[0]?.profile?.name      # null if any step is null
output.value = input.orders?[5]?.items?[0]?.price # complex null-safe chain

# Combine with .or() for defaults
output.city = input.user?.address?.city.or("Unknown")

# Combine with .catch() for error handling
output.parsed = input.data?.parse_json().catch({}) # null if data is null, {} if parse fails
```

**Semantics:**
- `?.` returns `null` if left operand is `null` or field doesn't exist
- `?[` returns `null` if left operand is `null`
- Short-circuits immediately on first `null` value
- Only handles `null` values, not errors (use `.catch()` for errors)
- Type errors still throw (e.g., `input.number?.uppercase()` throws error)

**Specification Updates:**
- Section 2.1 - Added `?.` operator and `?[` delimiter to tokens
- Section 4.1.2 (NEW) - Comprehensive null-safe navigation documentation
- Section 9.1.1 (NEW) - Comparison with `.catch()` for error handling
- Section 9.2.1 (NEW) - Comparison with `.or()` for null handling
- Section 14.2 - Enhanced null-safe navigation patterns and examples
- Section 15 - Updated grammar to support `?.` and `?[`, added detailed notes
- README - Added null-safe navigation quick start example

**Rationale:**
- Addresses Solution 4 (Null-Safe Operators) from proposed solutions
- Dramatically reduces verbosity for optional field navigation
- Consistent with TypeScript, Swift, C#, Kotlin, and other modern languages
- Complements existing `.or()` and `.catch()` methods
- Fully backward compatible - adds new syntax without breaking existing code

### ✅ Enhanced Lambda Expressions (Multi-Parameter, Multi-Statement, First-Class)

**Implementation Date:** 2026-02-11

**What Changed:**
- Added support for **multi-parameter lambdas**: `(a, b) -> a + b`
- Added support for **multi-statement lambda bodies** using block syntax `{ }`
- **Lambdas are now first-class values**: can be stored in variables and passed around
- Added **lambda as a runtime type**: `.type()` returns `"lambda"`
- Last expression in block is the return value
- Variables can be declared inside lambda bodies with `$variable = value`
- Enforced purity constraints: lambdas cannot assign to `output` or metadata

**Examples:**
```bloblang
# Multi-parameter lambdas
output.total = input.items.reduce((acc, item) -> acc + item.price, 0)
output.sum = input.pairs.map_each((key, value) -> key + value)

# Stored lambda functions
$add = (a, b) -> a + b
$multiply = (a, b) -> a * b
output.sum = $add(input.x, input.y)
output.product = $multiply(input.x, input.y)

# Multi-statement lambda
output.processed = input.items.map_each(item -> {
  $base = item.price * item.quantity
  $tax = $base * 0.1
  $total = $base + $tax
  $total.round(2)
})

# Complex multi-parameter lambda with block
$calculate_total = (price, quantity, tax_rate) -> {
  $subtotal = price * quantity
  $tax = $subtotal * tax_rate
  $subtotal + $tax
}
output.order_total = $calculate_total(input.price, input.qty, 0.1)

# Lambda type
$fn = (a, b) -> a + b
output.type = $fn.type()  # "lambda"

# Nested multi-parameter lambdas
output.summary = input.orders.map_each(order -> {
  $subtotal = order.items.reduce((acc, item) -> acc + item.price, 0)
  $tax = $subtotal * 0.1
  {
    "order_id": order.id,
    "subtotal": $subtotal,
    "tax": $tax,
    "total": $subtotal + $tax
  }
})
```

**Purity Constraints:**
```bloblang
# ❌ FORBIDDEN: Cannot assign to output inside lambda
input.items.map_each(item -> {
  output.log = item.id  # ERROR: output assignments not allowed
  item.value
})

# ❌ FORBIDDEN: Cannot assign to output inside lambda
input.items.filter(item -> {
  output@.counter = output@.counter + 1  # ERROR: output assignments not allowed
  item.active
})

# ✅ ALLOWED: Pure computations with local variables only
input.items.map_each(item -> {
  $doubled = item.value * 2
  $squared = $doubled * $doubled
  $squared
})
```

**Also Applied To:**
- **If Expressions** (Section 6.1) - Cannot contain output/metadata assignments
- **Match Expressions** (Section 6.3) - Cannot contain output/metadata assignments
- **Rationale**: These are expressions that return values, not statements with side effects

**Specification Updates:**
- Section 3 - Added `lambda` as a runtime type with examples
- Section 4.7 - Complete rewrite with single/multi-parameter and single/multi-statement lambda syntax
- Section 5 - Added "Statements vs Expressions" section clarifying the distinction
- Section 6.1 - Added purity constraints for if expressions
- Section 6.3 - Added purity constraints for match expressions
- Section 14.4 - Added multi-parameter and reusable lambda examples
- Section 15 - Updated grammar: `lambda_params := identifier | '(' identifier (',' identifier)* ')'`
- README - Added multi-parameter and stored lambda quick start examples

**Design Decisions:**
- **Multi-parameter syntax**: `(a, b) -> a + b` (parentheses required for multiple params)
- **First-class values**: Lambdas can be stored in variables and passed around
- **Dynamic typing**: No generics, runtime type checking only
- **Lambda type**: `.type()` returns `"lambda"`
- **Keep it simple**: Generics deferred to future version if needed

**Rationale:**
- Addresses Solution 5 (Enhanced Lambda Expressions) from proposed solutions
- Enables complex transformations within functional pipelines
- Multi-parameter support enables reduce, fold, and other higher-order patterns
- First-class lambdas enable reusable transformation functions
- Maintains language purity by preventing side effects in expressions
- Dynamic typing keeps implementation simple and consistent with rest of language
- Block-scoped variables (already implemented) enable multi-statement bodies
- Consistent with modern functional languages (JavaScript, Python, Ruby)
- Clear distinction between expressions (pure) and statements (side effects)
- Fully backward compatible - single-expression lambdas continue to work

### ✅ Iterator Optimization Strategy (Optional for Implementers)

**Implementation Date:** 2026-02-11

**What Changed:**
- Added Section 18: Implementation Optimizations
- Documented optional lazy evaluation strategy using iterators
- Enables 10-100x performance improvements for functional pipelines
- Fully transparent to users - no code changes required
- Variables always materialize to arrays (no "consumed iterator" errors)
- Direct chains stay lazy for maximum optimization

**Key Design:**
```bloblang
# Variable assignment materializes
$filtered = input.items.filter(x -> x.active)  # Array, reusable

# Direct chain stays lazy (optimized)
output.top = input.items
  .filter(x -> x.active)
  .map_each(x -> x.value)
  .take(10)
# Single pass, early termination
```

**Benefits:**
- **Transparent optimization**: Users write normal functional code, get automatic speedup
- **No confusion**: Variables are always arrays, fully reusable
- **Early termination**: `.take(10)` stops after 10 items
- **Zero intermediate allocations**: No memory waste in chains
- **Optional**: Implementations can choose whether to implement

**Specification Updates:**
- Section 18 - New section documenting optional optimization strategies
- README - Added link to implementation optimizations section

**Rationale:**
- Addresses performance concerns with functional pipelines
- Makes functional style competitive with imperative loops
- Keeps language simple (no iterator type exposed to users)
- Clear trade-off: variables for reuse, chains for performance
- Optional implementation detail, not required for correctness

### ✅ Enhanced Module System with Namespaced Imports

**Implementation Date:** 2026-02-11

**What Changed:**
- Import syntax requires namespace: `import "path" as namespace`
- Maps invoked as functions: `namespace.map_name(argument)`
- All top-level maps automatically exported
- No name collisions - namespaces prevent conflicts
- Clear provenance - always know where maps come from

**Examples:**
```bloblang
# user_transforms.blobl
map extract_user(data) {
  output.id = data.user_id
  output.name = data.full_name
}

# main.blobl
import "./user_transforms.blobl" as users

output.user = users.extract_user(input.user_data)
```

**Benefits:**
- **Clear provenance**: `users.extract_user()` shows where map comes from
- **No collisions**: Multiple files can have same map names
- **Simple implementation**: One import pattern, straightforward namespace lookup
- **Natural encapsulation**: Variables are private, maps are public

**Specification Updates:**
- Section 7 - Maps now use function call syntax instead of `.apply()`
- Section 8 - Complete rewrite with namespace import system
- Section 15 - Grammar updated for `import ... as` and qualified function calls

**Rationale:**
- Addresses Solution 9 (Enhanced Module System) from proposed solutions
- Solves name collision and provenance problems
- Balances power, UX, and implementation simplicity
- Function call syntax is more intuitive than `.apply()` method
- Breaking change acceptable for V2 clean slate

### ✅ Unified Execution Model (Immutable Only)

**Implementation Date:** 2026-02-11

**What Changed:**
- V2 uses **single, immutable execution model** only
- Input document (`input`) is **always immutable** throughout execution
- Removed dual execution model (mapping vs mutation processors)
- Assignment order no longer affects correctness

**Semantics:**
```bloblang
# Input is always immutable - order doesn't matter
output.invitees = input.invitees.filter(i -> i.mood >= 0.5)
output.rejected = input.invitees.filter(i -> i.mood < 0.5)  # Original unchanged

# Both assignments see the same input.invitees regardless of order
output.count1 = input.items.length()
output.filtered = input.items.filter(x -> x.active)
output.count2 = input.items.length()  # Same as count1
```

**Benefits:**
- **Order-independent**: No order-dependent bugs
- **Predictable**: Input never changes during execution
- **Easier to reason about**: Clear input → output transformation
- **Safer**: Can't accidentally mutate input
- **Easier to refactor**: Statements can be reordered without changing behavior

**Specification Updates:**
- Section 11 - Complete rewrite: single immutable execution model
- Removed mutation processor documentation
- Added assignment order independence section
- Clarified that input is immutable, output is built incrementally

**Rationale:**
- Addresses Solution 3 (Unify Execution Models) from proposed solutions
- Eliminates the highest-priority correctness issue
- Simpler mental model - only one way to execute
- Consistent with functional programming principles
- Aligns with V2's "explicit and predictable" philosophy
- Breaking change from V1, but V2 is a clean slate design

---

## Deferred Features (Can Be Added in Future Versions)

### 📋 Solution 6: String Interpolation
**Status:** Deferred - Not essential for V2
**Reason:** Already achievable with `.format()` method
**Current approach:** `"foo: %v".format(input.foo)`
**Possible future syntax:** `#"foo: {input.foo}"` (with custom prefix)
**Decision:** Can be added as syntactic sugar later if user demand justifies it

### 📋 Solution 10: Destructuring Assignment
**Status:** Deferred - Nice-to-have, not essential
**Reason:** Keeping V2 spec focused and minimal
**Possible future syntax:** `$x, $y = input.point` or `${id, name} = input.user`
**Current approach:** Individual assignments work fine
**Decision:** Can be added later if user demand justifies the added complexity

---

## Out of Scope

The following solutions are **not part of the V2 language specification**:

### 🔧 Solution 7: Static Analysis and Type Hints
**Reason:** Tooling concern - analyzers can be built on top of V2 without language changes

### 🔧 Solution 11: Improved Documentation and Discovery
**Reason:** Tooling concern - documentation and CLI improvements are implementation details

### 🔧 Solution 12: Enhanced Debugging Support
**Reason:** Tooling concern - debuggers can be built without language specification changes

### 🚫 Solution 13: Strict Mode Option
**Reason:** Design decision - V2 should have one consistent behavior, not multiple modes. Build it right once.

### 🚫 Solution 8: Iteration Syntax Sugar (for loops)
**Reason:** Addressed via optional iterator optimization (Section 18). Functional methods are sufficient; for-loop syntax not needed.

---

## Recommended Next Steps

Based on priority, impact, and dependencies:

### Option A: Continue with High-Impact, Non-Breaking Changes
**Next:** Solution 6 (String Interpolation)
- High developer demand
- Significant ergonomics improvement
- Fully backward compatible
- Independent of other solutions
- Note: Solutions 3, 4, and 5 completed 2026-02-11

### Option B: Focus on Tooling Improvements
**Next:** Solution 11 (Documentation) or Solution 12 (Debugging)
- Pure tooling improvements
- Immediate benefit to developers
- No language changes required
- Can be done in parallel with language features

### Option C: Continue with Medium-Priority Features
**Next:** Solution 7 (Static Analysis) or Solution 8 (Iteration Syntax)
- Medium priority ergonomics improvements
- Non-breaking changes
- Add value for complex use cases

---

## Decision Framework

For each remaining solution, consider:

1. **User Impact**: How much does this improve the developer experience?
2. **Backward Compatibility**: Breaking change or not?
3. **Implementation Complexity**: Parser changes, runtime changes, tooling?
4. **Dependencies**: Does this enable or require other solutions?
5. **Testing Burden**: How much test coverage is needed?
6. **Migration Cost**: If breaking, how hard is migration?

---

## Questions to Answer Before Proceeding

1. **Versioning Strategy**
   - Ship V2 as new processor type alongside V1?
   - Replace existing processors with V2 (with compatibility mode)?
   - Feature flags for gradual rollout?

2. **Timeline**
   - What's the target release for V2 Phase 1?
   - How long should V1 be supported after V2 release?

3. **Community Input**
   - Should we RFC the spec before implementation?
   - Beta program for early testing?
   - Feedback loops during development?

4. **Implementation Team**
   - Who owns parser changes?
   - Who owns runtime/interpreter changes?
   - Who owns documentation updates?
   - Who owns tooling (LSP, linter, debugger)?

5. **Testing Strategy**
   - Compatibility test suite for V1 code on V2 interpreter?
   - Performance benchmarks vs V1?
   - Security review for new features?

---

## Related Documents

- **./V1_BASELINE.md** - Current V1 specification (baseline for comparison)
- **./ISSUES_TO_ADDRESS.md** - Detailed analysis of V1 language weaknesses
- **./PROPOSED_SOLUTIONS.md** - Proposed solutions for each identified issue
- **./README.md through ./17_builtin_functions.md** - Modular V2 specification files (current design)

---

## Next Session

**Recommended Focus:** Decide which solution to tackle next based on strategic priorities.

**Options:**
1. Continue with backward-compatible ergonomics improvements (Solution 4 or 6)
2. Address tooling and documentation (Solution 11 or 12)
3. Tackle major design issues (Solution 3 - Unify Execution Models)

**Your Input Needed:**
- Which area should we focus on next?
- Are there specific user pain points driving priority?
- What's the timeline and release strategy?
