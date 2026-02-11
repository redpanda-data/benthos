# Bloblang V2 Development Progress

**Last Updated:** 2026-02-10
**Status:** In Progress

## Overview

This document tracks the incremental development of Bloblang V2 enhancements. Each solution from PROPOSED_SOLUTIONS.md is being incorporated into the modular specification files in this directory one at a time.

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

### ✅ Simplified Metadata Syntax

**Implementation Date:** 2026-02-10

**What Changed:**
- Removed `meta foo =` syntax → use `@foo =` instead
- Removed `metadata("foo")` function → use `@foo` instead
- Removed `meta` keyword entirely
- Only one way to handle metadata: `@` prefix notation

**Examples:**
```bloblang
# Reading metadata
output.topic = @kafka_topic
output.key = @kafka_key

# Writing metadata
@output_topic = "processed"
@kafka_key = input.id

# Deleting metadata
@kafka_key = deleted()
```

**Rationale:**
- One clear way to do things (Zen of Python)
- Consistent syntax for reading and writing
- Less cognitive overhead
- Cleaner grammar

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

### ✅ Explicit Array Indexing with Negative Index Support

**Implementation Date:** 2026-02-11

**What Changed:**
- Documented explicit array indexing syntax (already supported in grammar)
- Added support for negative indices (Python-style)
- Clarified error behavior and safe access patterns

**Examples:**
```bloblang
# Positive indexing (0-based)
output.first = input.items[0]
output.second = input.items[1]

# Negative indexing
output.last = input.items[-1]           # Last element
output.second_last = input.items[-2]    # Second-to-last

# Dynamic indexing
output.element = input.items[input.position]
output.nested = input.users[$index].name

# Safe access
output.safe = input.items[0].catch(null)
output.last_safe = input.items[-1].catch("empty")

# Chained access
output.value = input.data[2].items[5].name
```

**Negative Index Semantics:**
- `-1` accesses the last element
- `-n` accesses the nth element from the end
- For array of length N, index `-i` is equivalent to `N-i`

**Error Behavior:**
- Out-of-bounds access (positive or negative) throws mapping error
- Use `.catch()` for safe access with fallback values
- Non-integer index throws error
- Indexing non-array types throws error

**Specification Updates:**
- Section 3 - Enhanced array type description with indexing details
- Section 4.1.1 (NEW) - Added comprehensive array indexing documentation
- Section 9.1 - Added array indexing error examples
- Section 14.5 (NEW) - Added array indexing patterns and examples
- Section 15 - Fixed grammar to support `path[index]` without dot, added detailed notes
- README - Added array indexing quick start example

**Grammar Correction:**
- Changed `path := base ('.' field_access)*` where `field_access` includes `'[' expr ']'`
- To `path := base path_component*` where `path_component := '.' field_name | '[' expr ']'`
- This correctly allows `input.foo[0]` instead of requiring `input.foo.[0]`

**Rationale:**
- Common user request for explicit element access
- Negative indexing provides ergonomic access to array tail
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

---

## Pending Solutions

### 🔄 Solution 3: Unify Execution Models
**Priority:** High
**Status:** Not Started
**Breaking Change:** Yes (major architectural change)

### 🔄 Solution 4: Null-Safe Operators
**Priority:** High (Phase 1)
**Status:** Not Started
**Breaking Change:** No

### 🔄 Solution 5: Enhanced Lambda Expressions
**Priority:** Medium
**Status:** Not Started
**Breaking Change:** No
**Dependencies:** Solution 1 (enables multi-statement lambdas with scoped variables)

### 🔄 Solution 6: String Interpolation
**Priority:** High (Phase 1)
**Status:** Not Started
**Breaking Change:** No

### 🔄 Solution 7: Static Analysis and Type Hints
**Priority:** Medium (Phase 2)
**Status:** Not Started
**Breaking Change:** No (tooling + optional annotations)

### 🔄 Solution 8: Iteration Syntax Sugar
**Priority:** Medium (Phase 2)
**Status:** Not Started
**Breaking Change:** No

### 🔄 Solution 9: Enhanced Module System
**Priority:** Medium (Phase 3)
**Status:** Not Started
**Breaking Change:** Yes (import syntax changes)

### 🔄 Solution 10: Destructuring Assignment
**Priority:** Low-Medium (Phase 2)
**Status:** Not Started
**Breaking Change:** No

### 🔄 Solution 11: Improved Documentation and Discovery
**Priority:** High (Phase 1 - tooling only)
**Status:** Not Started
**Breaking Change:** No

### 🔄 Solution 12: Enhanced Debugging Support
**Priority:** High (Phase 1 - tooling only)
**Status:** Not Started
**Breaking Change:** No

### 🔄 Solution 13: Strict Mode Option
**Priority:** Low (Phase 4)
**Status:** Not Started
**Breaking Change:** No (opt-in)

---

## Recommended Next Steps

Based on priority, impact, and dependencies:

### Option A: Continue with High-Impact, Non-Breaking Changes
**Next:** Solution 6 (String Interpolation) or Solution 4 (Null-Safe Operators)
- High developer demand
- Significant ergonomics improvement
- Fully backward compatible
- Independent of other solutions

### Option B: Focus on Tooling Improvements
**Next:** Solution 11 (Documentation) or Solution 12 (Debugging)
- Pure tooling improvements
- Immediate benefit to developers
- No language changes required
- Can be done in parallel with language features

### Option C: Address Major Design Issues
**Next:** Solution 3 (Unify Execution Models)
- Highest priority issue from correctness perspective
- Requires careful design and migration planning
- Should be addressed before too much V2 code is written
- May influence other solution designs

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
