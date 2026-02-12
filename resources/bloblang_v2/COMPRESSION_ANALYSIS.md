# Bloblang V2 Spec Compression Analysis

## Current Structure: 18 Sections

1. Overview (1 page)
2. Lexical Structure (1 page)
3. Type System (1 page)
4. Expressions (10 pages) ⚠️ LARGE
5. Statements (2 pages)
6. Control Flow (3 pages)
7. Maps (1 page)
8. Imports (3 pages)
9. Error Handling (2 pages)
10. Metadata (1 page)
11. Execution Model (1 page)
12. Context and Scoping (2 pages)
13. Special Features (2 pages)
14. Common Patterns (6 pages) ⚠️ LARGE
15. Grammar Summary (2 pages)
16. Type Coercion (1 page)
17. Built-in Functions (1 page)
18. Implementation Optimizations (6 pages) ⚠️ LARGE

**Total: ~45 pages**

---

## Proposed Compressed Structure: 12 Sections

### 1. Overview & Lexical Structure (1.5 pages)
**Merge:** Sections 1 + 2
**Rationale:** Lexical structure is short, overview can include it
**Savings:** 1 file

### 2. Type System & Coercion (1.5 pages)
**Merge:** Sections 3 + 16
**Rationale:** Type coercion is just 1 page, natural extension of type system
**Savings:** 1 file

### 3. Expressions & Statements (8 pages)
**Merge:** Sections 4 + 5
**Rationale:** Closely related, statements are just special expressions
**Compression:** Remove redundant examples, tighten prose
**Savings:** ~4 pages through compression

### 4. Control Flow (2 pages)
**Keep:** Section 6
**Compression:** Remove some verbose examples
**Savings:** ~1 page

### 5. Functions & Maps (2 pages)
**Merge:** Section 7 (Maps) concepts into this
**New focus:** User-defined functions (maps) and how they work
**Rationale:** Maps are really just named functions
**Savings:** Clearer organization

### 6. Imports & Modules (2 pages)
**Keep:** Section 8
**Compression:** Remove some redundant examples
**Savings:** ~1 page

### 7. Execution Model (2 pages)
**Merge:** Sections 11 + 12 (Context and Scoping) + 10 (Metadata)
**Rationale:** All about how code executes - input/output, scoping, metadata
**Savings:** 2 files, ~1 page through compression

### 8. Error Handling (1.5 pages)
**Keep:** Section 9
**Compression:** Tighten prose
**Savings:** ~0.5 pages

### 9. Special Features (1.5 pages)
**Keep:** Section 13
**Compression:** Remove less critical examples
**Savings:** ~0.5 pages

### 10. Grammar Reference (2 pages)
**Keep:** Section 15
**Compression:** Remove verbose notes (move to relevant sections)
**Savings:** Keep concise reference

### 11. Common Patterns (3 pages)
**Keep:** Section 14
**Compression:** Keep only most illustrative examples, remove verbose explanations
**Savings:** ~3 pages through aggressive compression

### 12. Implementation Guide (4 pages)
**Merge:** Sections 17 + 18
**New focus:** Guidelines for implementers (built-ins + optimizations)
**Compression:** Remove verbose explanations, keep technical details
**Savings:** ~3 pages

---

## Compression Summary

**From:** 18 sections, ~45 pages
**To:** 12 sections, ~28 pages
**Reduction:** 33% file count, 38% page count
**Information preserved:** 100%

---

## Specific Compression Techniques

### 1. Consolidate Examples
**Before:**
```bloblang
# Example 1: Basic case
output.result = input.value

# Example 2: With null check
output.result = if input.value != null {
  input.value
}

# Example 3: With default
output.result = input.value.or("default")
```

**After:**
```bloblang
# Basic, null-checked, and default handling
output.result = input.value
output.result = input.value.or("default")
```

### 2. Reduce Repetition
**Before:**
- Explain immutability in Section 11 (Execution Model)
- Explain immutability again in Section 12 (Context and Scoping)
- Mention immutability in Section 5 (Statements)

**After:**
- Explain once in merged Execution Model section
- Reference briefly elsewhere

### 3. Inline Short Sections
**Before:**
- Section 16: Type Coercion (1 page, standalone)
- Section 3: Type System (1 page)

**After:**
- Section 2: Type System & Coercion (1.5 pages, merged)

### 4. Compress Verbose Prose
**Before:**
```
Variables in Bloblang V2 use the `$` prefix for both declaration and reference.
This provides a consistent syntax pattern where the same symbol is used regardless
of whether you are declaring a new variable or referencing an existing one.
The variable declarations use block-scoping semantics, which means that variables
are scoped to the block in which they are declared and are not accessible outside
of that block. This is similar to modern programming languages like JavaScript,
Rust, and Go.
```

**After:**
```
Variables use `$` prefix for declaration and reference. Block-scoped with shadowing support.
```

### 5. Remove Redundant Sections
**Before:**
- Section 12: Context and Scoping (explains `input`/`output`, variable scope)
- Section 11: Execution Model (explains immutable input, mutable output)
- These overlap significantly

**After:**
- Section 7: Execution Model (covers all: immutability, scoping, variables, contexts)

### 6. Streamline Common Patterns
**Before:**
- 6 pages of examples with verbose explanations

**After:**
- 3 pages with concise examples and minimal explanation
- Focus on unique patterns, remove obvious ones

---

## Detailed Merge Plan

### Merge 1: Overview + Lexical Structure

**Current (2 pages):**
- Section 1: Philosophy, design goals, quick intro
- Section 2: Tokens, identifiers, literals

**Compressed (1.5 pages):**
```markdown
# 1. Overview

## Design Philosophy
- Explicit context management
- One clear way to do things
- Fail loudly

## Quick Start
[Brief examples]

## Lexical Structure
- Keywords: input, output, if, else, match, as, map, deleted, import
- Operators: `.`, `?.`, `@.`, `=`, `+`, `-`, `*`, `/`, `%`, `!`, `>`, `>=`, `==`, `<`, `<=`, `&&`, `||`, `=>`, `->`
- Variables: `$name`, Metadata: `input@.key` / `output@.key`
- Literals: numbers, strings, booleans, null, arrays, objects
- Comments: `#` to EOL
```

### Merge 2: Type System + Coercion

**Current (2 pages):**
- Section 3: Runtime types
- Section 16: Type coercion rules for `+`

**Compressed (1.5 pages):**
```markdown
# 2. Type System

## Runtime Types
- Primitives: string, number, bool, null, bytes
- Collections: array, object
- Functions: lambda

## Type Coercion
- `+` requires same types: `5 + 3` (number), `"a" + "b"` (string)
- No implicit conversion: `5 + "3"` is ERROR
- Explicit: `5.string() + "3"` → `"53"`
```

### Merge 3: Expressions + Statements

**Current (12 pages):**
- Section 4: Path expressions, operators, functions, lambdas (10 pages)
- Section 5: Assignments, variable declarations (2 pages)

**Compressed (8 pages):**
- Combine into one cohesive section
- Remove "Statements vs Expressions" redundancy (implied by structure)
- Consolidate examples
- Single operator precedence table (not repeated)

### Merge 4: Execution Model + Context + Metadata

**Current (4 pages):**
- Section 10: Metadata (1 page)
- Section 11: Execution model (1 page)
- Section 12: Context and scoping (2 pages)

**Compressed (2 pages):**
```markdown
# 7. Execution Model

## Immutable Input, Mutable Output
- `input` (document + metadata) always immutable
- `output` (document + metadata) built incrementally
- Order-independent correctness

## Contexts
- `input.field` / `input@.key` - read-only
- `output.field` / `output@.key` - write
- `$variable` - block-scoped, immutable, shadowing

## Metadata
- Access: `input@.key` (immutable), `output@.key` (mutable)
- Copy: `output@ = input@`

## Scoping
- Top-level: global scope
- Blocks (if, match, lambdas): nested scope
- Variables shadow outer scopes
```

### Merge 5: Built-ins + Optimizations

**Current (7 pages):**
- Section 17: Built-in Functions (1 page)
- Section 18: Implementation Optimizations (6 pages)

**Compressed (4 pages):**
```markdown
# 12. Implementation Guide

## Built-in Functions & Methods
[Concise reference - not exhaustive, point to CLI docs]

## Optional Optimizations

### Lazy Evaluation (Iterators)
- Methods may return internal iterators
- Auto-materialize at: variable assignment, output assignment, indexing
- Benefit: 10-100x faster, no code changes
- Example: filter + map + take → single pass

### Other Optimizations
- Constant folding: `2 + 2` → `4`
- Dead code elimination
- String builder for concatenation
```

---

## Example: Section 4 Compression

**Before (10 pages):**
```markdown
## 4.1 Path Expressions

Navigate nested structures using dot notation:
```
input.user.profile.email
output.result.data.id
```

Paths may reference:
- **Input document**: `input.field`
- **Output document**: `output.field`
- **Variables**: `$variable_name`
- **Input metadata**: `input@.key`
- **Output metadata**: `output@.key`

### 4.1.1 Indexing (Arrays, Strings, Bytes)

Access array elements, string characters, or bytes using square bracket notation with integer indices:

```bloblang
# Array indexing (0-based)
output.first = input.items[0]           # First element
output.second = input.items[1]          # Second element
output.tenth = input.items[9]           # Tenth element

# String indexing (byte position, returns single-character string)
output.first_char = input.text[0]       # First character
output.third_char = input.text[2]       # Third character
[... continues for many more examples ...]
```

**After (6 pages):**
```markdown
## 3.1 Paths

Access nested data: `input.user.email`, `output.result.id`

**Path roots:** `input`, `output`, `$variable`
**Metadata:** `input@.key`, `output@.key`

### Indexing

Access arrays, strings, bytes with `[index]`:
```bloblang
input.items[0]      # First element
input.items[-1]     # Last element
input.name[0]       # First character (string)
input.data[0]       # First byte as number (bytes)
```

Negative indices count from end (Python-style).
Out of bounds throws error (use `.catch()` for safety).
[... concise examples only ...]
```

---

## Compression Results by Section

| Section | Before | After | Savings |
|---------|--------|-------|---------|
| 1-2 Overview + Lexical | 2 pages | 1.5 pages | 25% |
| 3+16 Type System + Coercion | 2 pages | 1.5 pages | 25% |
| 4-5 Expressions + Statements | 12 pages | 8 pages | 33% |
| 6 Control Flow | 3 pages | 2 pages | 33% |
| 7 Maps | 1 page | (merged) | - |
| 8 Imports | 3 pages | 2 pages | 33% |
| 9 Error Handling | 2 pages | 1.5 pages | 25% |
| 10-11-12 Execution Model | 4 pages | 2 pages | 50% |
| 13 Special Features | 2 pages | 1.5 pages | 25% |
| 14 Common Patterns | 6 pages | 3 pages | 50% |
| 15 Grammar | 2 pages | 2 pages | 0% |
| 17-18 Implementation | 7 pages | 4 pages | 43% |
| **TOTAL** | **~45 pages** | **~28 pages** | **38%** |

---

## Benefits of Compression

1. **Easier to read** - Less repetition, more focused
2. **Easier to maintain** - Fewer places to update
3. **Better organization** - Related concepts together
4. **Faster to reference** - Less navigation between sections
5. **Same information** - No loss of key details

## Risks of Compression

1. **Might be too dense** - Less breathing room for readers
2. **Examples might be missed** - Fewer illustrative examples
3. **Harder to skim** - More information per page

## Recommendation

Proceed with compression targeting **30-35 pages** (30-35% reduction):
- Merge related sections (6 fewer files)
- Compress verbose prose (keep technical precision)
- Consolidate redundant examples (keep best ones)
- Maintain all semantic details (no information loss)
