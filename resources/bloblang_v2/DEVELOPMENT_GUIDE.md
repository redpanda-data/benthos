# Bloblang V2 Development Guide

**Purpose:** This document provides everything needed to continue Bloblang V2 development in a new conversation.

---

## Project Goal

Design and specify **Bloblang V2** - a cleaned-up version of the Bloblang mapping language that eliminates confusion, improves ergonomics, and enforces explicit context management.

**Important:** This is a **clean slate design** with NO backward compatibility requirements. V2 will run alongside V1.

---

## Core Design Principles

1. **Radical Explicitness** - No implicit context shifting, all data references must be explicit
2. **One Clear Way** - Single obvious way to do each operation (Zen of Python)
3. **Consistent Syntax Patterns** - Symmetrical keywords (`input`/`output`), consistent prefixes (`$` for variables, `@` for metadata)
4. **Block-Scoped Variables** - Modern scoping with shadowing support
5. **Fail Loudly** - Errors are explicit, not silent

---

## Key V2 Changes (Already Implemented)

### Syntax Changes
| V1 Syntax | V2 Syntax | Rationale |
|-----------|-----------|-----------|
| `this.field` | `input.field` | Explicit input reference, no context confusion |
| `root.field` | `output.field` | Symmetry with `input`, more explicit |
| `let foo = value` | `$foo = value` | Consistent prefix for declaration and reference |
| `meta foo = value` | `@foo = value` | Only one way to handle metadata |
| `metadata("foo")` | `@foo` | Consistent with assignment syntax |
| `a \| b \| c` | `a.or(b).or(c)` | Removed pipe operator (reserved for future) |

### Semantic Changes
1. **`this` keyword removed entirely** - Must use `input` or explicit lambda parameters
2. **Match requires explicit `as` binding** - `match input.value as v { v == 1 => ... }`
3. **Maps require explicit parameters** - `map foo(data) { output.x = data.x }`
4. **Block-scoped variables** - Can declare `$var` inside `if`, `match`, lambda bodies
5. **Variable shadowing allowed** - Inner scopes can shadow outer scope variables

### Keywords Removed
- `this` (use `input`)
- `root` (use `output`)
- `let` (use `$variable =`)
- `meta` (use `@metadata =`)

### Keywords Added
- `input` (explicit input document reference)
- `as` (explicit context binding in match)

---

## File Organization

```
./resources/bloblang_v2/
├── DEVELOPMENT_GUIDE.md          ← YOU ARE HERE (continuation guide)
├── README.md                      ← Entry point, quick start, navigation
├── V1_BASELINE.md                 ← Original V1 spec (17K)
├── ISSUES_TO_ADDRESS.md           ← 13 identified V1 problems (13K)
├── PROPOSED_SOLUTIONS.md          ← Solutions for all 13 issues (29K)
├── BLOBLANG_V2_PROGRESS.md        ← Completed/pending tracker (9K)
└── 01-17_*.md                     ← Modular V2 specification (17 files)
```

---

## Completed Solutions (6 of 13)

✅ **Solution 1:** Block-Scoped Variables
✅ **Solution 2:** Explicit Context Management (enhanced - removed `this`)
✅ **Additional:** Pipe Operator Removal
✅ **Additional:** Keyword Consistency (`root` → `output`)
✅ **Additional:** Simplified Metadata Syntax (only `@` notation)
✅ **Additional:** Simplified Variable Syntax (only `$` notation)

---

## Pending Solutions (7 of 13)

### High Priority (Phase 1)
- **Solution 3:** Unify Execution Models (mapping vs mutation semantic split)
- **Solution 4:** Null-Safe Operators (`?.` and `?:`)
- **Solution 6:** String Interpolation

### Medium Priority (Phase 2)
- **Solution 5:** Enhanced Lambda Expressions (multi-statement lambdas)
- **Solution 7:** Static Analysis and Type Hints
- **Solution 8:** Iteration Syntax Sugar
- **Solution 10:** Destructuring Assignment

### Lower Priority (Phase 3+)
- **Solution 9:** Enhanced Module System
- **Solution 11:** Improved Documentation and Discovery (tooling)
- **Solution 12:** Enhanced Debugging Support (tooling)
- **Solution 13:** Strict Mode Option

---

## How to Continue Development

### Step 1: Choose Next Solution
Review **PROPOSED_SOLUTIONS.md** and select which solution to implement next based on:
- User priorities and pain points
- Dependencies between solutions
- Breaking vs non-breaking changes
- Implementation complexity

**Recommended starting points:**
- **Solution 4** (Null-Safe Operators) - High impact, non-breaking, independent
- **Solution 6** (String Interpolation) - High demand, non-breaking, independent
- **Solution 3** (Unify Execution Models) - Major design issue, influences other solutions

### Step 2: Update V2 Specification Files
Modify the relevant `0X_*.md` files in this directory:
- Add new syntax to `02_lexical_structure.md` (if new operators/keywords)
- Update `04_expressions.md` or `05_statements.md` (for new expression/statement forms)
- Update `06_control_flow.md` (for control flow changes)
- Update `15_grammar.md` (formal grammar)
- Add examples to `14_common_patterns.md`

### Step 3: Update Progress Tracker
Mark the solution as completed in **BLOBLANG_V2_PROGRESS.md**:
- Move from "Pending Solutions" to "Completed Solutions"
- Document implementation date, approach, and rationale
- List all specification sections updated

### Step 4: Maintain Consistency
Ensure all examples across all specification files reflect the change:
- Use explicit `input`/`output` references
- Use `$` for variables, `@` for metadata
- No `this`, `root`, `let`, `meta` keywords
- Match expressions have `as` binding
- Maps have explicit parameters

---

## Design Decision Log

### Why remove `this`?
Context confusion: `this` changed meaning in different contexts (top-level, lambda, map, match). Explicit naming eliminates all ambiguity.

### Why remove pipe operator `|`?
Reserved for potential future feature (e.g., Unix-style pipelining). `.or()` method chaining is explicit and consistent.

### Why `$` and `@` prefixes?
- Consistent: same symbol for declaration and reference
- Symmetrical: `$` for variables, `@` for metadata
- Clear visual distinction from regular identifiers
- One obvious way to declare/reference

### Why rename `root` to `output`?
Symmetry with `input`. The pair `input`/`output` is immediately intuitive.

### Why require `as` in match?
Explicit context binding eliminates confusion about what `this` refers to in match arms. Forces explicit naming.

### Why require explicit map parameters?
Eliminates implicit `this` context. Makes data flow explicit and easier to understand.

---

## Testing Strategy (Future)

When implementing solutions:
1. **Parser Tests** - For new syntax
2. **Runtime Tests** - For new semantics
3. **Example Tests** - Verify all documentation examples work
4. **Migration Tests** - If breaking changes, document migration path
5. **Performance Tests** - Ensure no regressions

---

## Questions to Resolve

1. **Versioning Strategy** - How will V2 be deployed alongside V1?
2. **Timeline** - Target release date for V2 Phase 1?
3. **Community RFC** - Should V2 spec be RFC'd before implementation?
4. **Implementation Team** - Who owns parser, runtime, tooling, docs?
5. **Migration Support** - Will there be automated V1→V2 migration tools?

---

## Quick Reference: V2 Syntax

```bloblang
# Variable declaration and reference
$user_id = input.user.id
output.id = $user_id

# Metadata read and write
output.topic = @kafka_topic
@output_key = input.id

# Block-scoped variables
output.result = if input.enabled {
  $doubled = input.amount * 2
  $doubled.floor()
}

# Match with explicit binding
output.category = match input.score as score {
  score >= 90 => "A"
  score >= 80 => "B"
  _ => "F"
}

# Map with explicit parameter
map extract_user(data) {
  output.id = data.user_id
  output.name = data.full_name
}

# Array transformation with explicit lambda parameter
output.names = input.users
  .filter(user -> user.active)
  .map_each(user -> user.name.uppercase())

# Error handling with .or()
output.value = input.primary.or(input.secondary).or("default")
```

---

## For AI Assistants Continuing This Work

- Read **BLOBLANG_V2_PROGRESS.md** first to see current status
- Review **PROPOSED_SOLUTIONS.md** to understand remaining work
- Check **ISSUES_TO_ADDRESS.md** to understand the "why" behind changes
- Reference **V1_BASELINE.md** when comparing V1 vs V2 behavior
- Keep **all 17 specification files** in sync when making changes
- Update **BLOBLANG_V2_PROGRESS.md** after completing each solution
- Maintain the design principles: explicit, consistent, one clear way

**Last Updated:** 2026-02-10
**Status:** 6 of 13+ solutions completed, ready for next solution
