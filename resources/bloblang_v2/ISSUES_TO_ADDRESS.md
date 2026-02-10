# Bloblang Language Issues and Improvement Opportunities

**Date:** 2026-02-10
**Status:** Draft for review and prioritization

This document outlines potential weaknesses, limitations, and improvement opportunities for the Bloblang language based on analysis of the specification and documentation.

---

## 1. Variable Scoping Limitations

### Issue
Variables can only be declared at top-level scope using `let`. They cannot be declared inside conditional blocks (`if`, `match`) or lambda expressions.

### Impact
- Forces developers to declare variables at the top even when only needed in specific branches
- Requires additional null-safety measures when variables might not be initialized in all code paths
- Reduces code locality and readability

### Example
```bloblang
# INVALID - Parse error
root.age = if this.birthdate != null {
  let parsed = this.birthdate.ts_parse("2006-01-02")  # Not allowed here
  $parsed.ts_unix()
}

# Must use workaround with top-level declaration
let parsed = this.birthdate.ts_parse("2006-01-02").catch(null)
root.age = if $parsed != null {
  $parsed.ts_unix()
}
```

### Severity
**Medium** - Has workarounds but impacts code quality

---

## 2. Context Shifting Complexity

### Issue
The `this` keyword changes meaning in different contexts:
- Top-level: input document
- Inside `match this.field { }`: refers to the matched field value
- Inside lambda `x -> x.foo`: refers to lambda parameter
- Inside parentheses `this.foo.(this.bar)`: refers to `this.foo`

Additionally, the documentation warns that "context references in catch chains can be unreliable."

### Impact
- Confusing for developers learning the language
- Easy to make mistakes when refactoring code
- Requires storing values in variables to ensure reliable access in error handlers
- Different mental models needed for different contexts

### Example
```bloblang
# Context shifts inside match
match this.pet {
  "cat" => this.lives   # 'this' now refers to this.pet, not root document
  _ => 0
}

# Unreliable context in catch chains (from documentation)
root.parsed = this.date.ts_parse("2006-01-02").catch(
  this.date.ts_parse("2006/01/02")  # this.date might not work here
)

# Recommended workaround
let date_str = this.date
root.parsed = $date_str.ts_parse("2006-01-02").catch(
  $date_str.ts_parse("2006/01/02")
)
```

### Severity
**Medium-High** - Creates reliability issues and cognitive overhead

---

## 3. Dual Execution Models (Mapping vs Mutation)

### Issue
The language has two different execution semantics depending on processor type:
- **Mapping processor**: Input document is immutable, creates new output
- **Mutation processor**: Input document is mutable, modified in-place

The same Bloblang code produces different results depending on which processor executes it.

### Impact
- Order-dependent bugs in mutation mode that don't appear in mapping mode
- Confusion when moving code between processor types
- Difficult to reason about code behavior without knowing execution context
- Testing becomes more complex as same code needs different test strategies

### Example
```bloblang
# Same code, different behavior:

# In MUTATION mode - order matters
root.invitees = this.invitees.filter(i -> i.mood >= 0.5)  # Mutates original
root.rejected = this.invitees.filter(i -> i.mood < 0.5)   # Now sees mutated version - empty!

# In MAPPING mode - order doesn't matter
root.invitees = this.invitees.filter(i -> i.mood >= 0.5)  # Reads original
root.rejected = this.invitees.filter(i -> i.mood < 0.5)   # Still reads original
```

### Severity
**High** - Can cause silent correctness issues when refactoring

---

## 4. Verbose Error Handling Requirements

### Issue
The language requires explicit `.catch()` or `.or()` on nearly every operation to write production-safe code. Many operations that commonly fail in practice don't have sensible default behaviors.

### Impact
- Code becomes verbose and cluttered with defensive programming
- Burden shifted to developers rather than having safe defaults
- Easy to forget error handling in new code
- Reduces signal-to-noise ratio in mappings

### Example
```bloblang
# Documentation shows many "Bad" vs "Good" patterns:

# Array access
root.first = this.items[0]                        # Bad: fails on empty array
root.first = this.items[0].catch(null)            # Good: requires explicit catch

# Parsing
root.data = this.payload.parse_json()             # Bad: fails on invalid JSON
root.data = this.payload.parse_json().catch({})   # Good: explicit fallback

# Type coercion
root.id = this.user_id.uppercase()                # Bad: fails if not string
root.id = this.user_id.string().uppercase()       # Good: explicit conversion

# Null checks
root.name = this.user.name                        # Bad: fails if null
root.name = this.user.name.or("anonymous")        # Good: explicit fallback
```

### Severity
**Medium** - Verbose but makes code resilient; trade-off between safety and ergonomics

---

## 5. Limited Lambda Expressions

### Issue
Lambda expressions are constrained:
- Single expression only (no multi-statement bodies)
- Single parameter only (no multi-parameter functions)
- No closures over mutable variables
- Cannot define named functions (only named maps)

### Impact
- Complex transformations require multiple chained operations or recursive maps
- Cannot easily pass multiple values to transformation functions
- Less expressive than modern functional programming constructs
- Workarounds using object literals reduce clarity

### Example
```bloblang
# Can't do multi-parameter lambdas
this.items.filter((item, index) -> index < 10 && item.score > 50)  # Not supported

# Must compose multiple operations
this.items.enumerate()
  .filter(x -> x.index < 10)
  .filter(x -> x.value.score > 50)
  .map_each(x -> x.value)

# Can't define reusable functions, only maps
# No equivalent to: let transform = (x, y) -> x + y * 2
```

### Severity
**Low-Medium** - Limits expressiveness but has workarounds

---

## 6. No String Interpolation

### Issue
The language lacks string interpolation/template syntax. All string construction requires explicit concatenation with `+` operator.

### Impact
- Verbose string construction
- Type conversion must be explicit (`.string()` method)
- Difficult to read complex formatted strings
- More opportunities for errors (missing conversions, operator precedence)

### Example
```bloblang
# Current approach
root.message = "User " + this.name + " has " + this.count.string() + " items"

# Hypothetical interpolation syntax
root.message = "User ${this.name} has ${this.count} items"
```

### Severity
**Low** - Annoying but functional

---

## 7. Runtime-Only Type Checking

### Issue
No static analysis or type checking at parse/compile time. All type errors surface only at runtime with actual data.

### Impact
- Type errors discovered late in development or in production
- Difficult to ensure all code paths are type-safe without comprehensive test data
- No IDE support for type-aware autocomplete or validation
- Refactoring is riskier without type guarantees

### Severity
**Medium** - Common in dynamic languages but limits tooling and safety

---

## 8. No Iteration Constructs

### Issue
No `for`, `while`, or explicit loop constructs. Iteration beyond built-in array methods (`.map_each()`, `.filter()`) requires recursive maps.

### Impact
- Stateful iteration requires recursion
- Complex iteration patterns are verbose
- Performance characteristics less obvious
- Harder to understand for developers not familiar with functional programming

### Example
```bloblang
# Must use recursive map for custom iteration
map countdown {
  root = if this.count > 0 {
    {"count": this.count - 1}.apply("countdown")
  } else {
    "done"
  }
}
```

### Severity
**Low-Medium** - Functional approach works but less familiar to many developers

---

## 9. Limited Module System

### Issue
- `import` only loads entire files
- No selective imports (can't import specific maps)
- No namespace management
- No package/library system for sharing common transformations
- Maps are globally scoped within a file (potential name collisions)

### Impact
- Difficult to organize large mapping libraries
- All maps from imported file become available (namespace pollution)
- No way to distribute reusable transformation libraries
- Harder to build tooling around dependencies

### Example
```bloblang
# Can only import entire files
import "./transformations.blobl"  # All maps from file imported

# No equivalent to:
# import { extract_user, format_date } from "./transformations.blobl"
# import * as transforms from "./transformations.blobl"
```

### Severity
**Low-Medium** - Becomes more painful at scale

---

## 10. No Destructuring

### Issue
Cannot destructure objects or arrays in variable assignments or lambda parameters.

### Impact
- Verbose field extraction from nested objects
- Cannot elegantly unpack multiple values
- Reduces code readability for complex transformations

### Example
```bloblang
# Current approach
let id = this.user.id
let name = this.user.name
let email = this.user.email

# Hypothetical destructuring
let {id, name, email} = this.user

# In lambdas
this.items.map_each(item -> {
  "id": item.id,
  "name": item.name
})

# Hypothetical
this.items.map_each({id, name} -> {"id": id, "name": name})
```

### Severity
**Low** - Convenience feature, not critical

---

## 11. Discovery Complexity

### Issue
"Hundreds of functions and methods" spread across many categories make discovery difficult. Requires external tooling (scripts) to browse available operations in structured format.

### Impact
- Steep learning curve for new developers
- Difficult to know what operations are available
- May duplicate functionality due to not finding existing methods
- Documentation maintenance burden

### Potential Solutions
- Better categorization in documentation
- Interactive documentation browser
- IDE plugins with autocomplete
- Curated "common operations" cookbook

### Severity
**Medium** - Documentation/tooling issue rather than language design

---

## 12. Limited Debugging

### Issue
No debugger, breakpoints, or step-through execution. Only way to debug is adding intermediate assignments to inspect values.

### Impact
- Difficult to understand complex mapping failures
- Time-consuming to isolate issues in long method chains
- Cannot inspect intermediate values without modifying code
- Stack traces may not provide sufficient context

### Potential Solutions
- Dry-run mode with verbose output showing intermediate values
- Interactive debugger support
- Better error messages with context
- Trace mode showing evaluation steps

### Severity
**Medium** - Development experience issue

---

## 13. Implicit Field Creation Behavior

### Issue
Assignment to nested paths automatically creates intermediate objects. While convenient, this can mask errors when field names are misspelled.

### Example
```bloblang
# Typo creates unexpected structure instead of failing
root.user.emial = this.email  # Creates root.user.emial, not root.user.email
```

### Impact
- Typos create extra fields rather than failing
- Harder to catch bugs in tests if assertions don't check for extra fields
- Document shape can accidentally diverge from intended structure

### Potential Solutions
- Optional strict mode that requires field declarations
- Linter to detect suspicious field names
- Schema validation as separate step

### Severity
**Low** - Trade-off for convenience

---

## Comparison to Alternative Languages

Several modern transformation languages address some of these issues:

| Feature | Bloblang | JSONata | jq | JMESPath |
|---------|----------|---------|-----|----------|
| String interpolation | ❌ | ✅ | ✅ | ❌ |
| Block-scoped variables | ❌ | ✅ | ✅ | N/A |
| Multi-param lambdas | ❌ | ✅ | Limited | ❌ |
| Static type checking | ❌ | ❌ | ❌ | ❌ |
| Mutable execution mode | ✅ (optional) | ❌ | ❌ | ❌ |
| Module system | Basic | ❌ | ✅ | ❌ |
| Debugging support | ❌ | Limited | ✅ | ❌ |

---

## Prioritization Framework

### High Priority (Correctness/Reliability)
- Issue #3: Dual execution models
- Issue #2: Context shifting reliability

### Medium Priority (Ergonomics/Productivity)
- Issue #1: Variable scoping
- Issue #4: Verbose error handling
- Issue #7: Runtime-only type checking
- Issue #11: Discovery complexity
- Issue #12: Limited debugging

### Low Priority (Convenience)
- Issue #5: Limited lambda expressions
- Issue #6: No string interpolation
- Issue #8: No iteration constructs
- Issue #9: Limited module system
- Issue #10: No destructuring
- Issue #13: Implicit field creation

---

## Next Steps

1. **Validate issues** with core development team and user community
2. **Prioritize** based on user pain points and implementation complexity
3. **Design solutions** that maintain backward compatibility where possible
4. **Consider** whether issues warrant language v2 or incremental improvements
5. **Prototype** high-priority changes in experimental branch
6. **Gather feedback** through RFC process before finalizing changes

---

## Notes

- Many of these trade-offs were likely intentional design decisions prioritizing safety, simplicity, and sandboxability
- Some issues may be inherent constraints of the execution model (embedded in stream processor)
- Solutions should maintain Bloblang's core strengths: safety, predictability, and performance
- Consider versioning strategy if breaking changes are needed
