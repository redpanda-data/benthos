# Bloblang V2 Technical Specification

## Core Principles

- **Explicit Context Management** - No implicit behavior
- **One Clear Way** - Single obvious approach
- **Consistent Syntax** - Predictable patterns
- **Fail Loudly** - Errors are explicit

---

## Specification Sections

1. **[Overview & Lexical Structure](01_overview.md)** - Introduction, design philosophy, tokens, literals
2. **[Type System & Coercion](02_type_system.md)** - Runtime types, type conversion, coercion rules
3. **[Expressions & Statements](03_expressions.md)** - Paths, operators, functions, lambdas, assignments, variables
4. **[Control Flow](04_control_flow.md)** - If expressions/statements, match expressions/statements
5. **[Maps](05_maps.md)** - User-defined reusable transformations
6. **[Imports & Modules](06_imports.md)** - Namespace imports, file resolution
7. **[Execution Model](07_execution_model.md)** - Immutability, contexts, scoping, metadata
8. **[Error Handling](08_error_handling.md)** - `.catch()`, `.or()`, `throw()`, validation
9. **[Special Features](09_special_features.md)** - Dynamic fields, message filtering, non-structured data
10. **[Grammar Reference](10_grammar.md)** - Formal grammar definition
11. **[Common Patterns](11_common_patterns.md)** - Practical examples and idioms
12. **[Implementation Guide](12_implementation_guide.md)** - Optional optimizations, performance
13. **[Standard Library](13_standard_library.md)** - Required functions and methods reference

---

## Quick Reference

### Basic Syntax

```bloblang
# Assignment
output.field = input.field
output@.key = input@.key

# Variables
$user = input.user
$name = $user.name.uppercase()

# Null-safe
output.city = input.user?.address?.city.or("Unknown")

# Functional
output.results = input.items
  .filter(item -> item.active)
  .map(item -> item.value * 2)
  .sort()

# Conditionals
output.tier = match input.score as s {
  s >= 100 => "gold",
  s >= 50 => "silver",
  _ => "bronze",
}

# Maps (isolated functions)
map normalize(data) {
  {
    "id": data.user_id,
    "name": data.full_name
  }
}
output.user = normalize(input.user_data)

# Imports
import "./utils.blobl" as utils
output.result = utils::transform(input.data)
```

### Key Features

- **Immutable input:** `input` never changes
- **Mutable output:** `output` built incrementally
- **Mutable variables:** `$var` can be reassigned, block-scoped with shadowing
- **Null-safe operators:** `?.` and `?[]`
- **Explicit type coercion:** No implicit conversion
- **Function-style maps:** Called as `name(arg)` or `namespace::name(arg)`
- **Namespace imports:** `import "..." as name`
- **Lambda syntax:** Multi-param, multi-statement, for method arguments and map bodies

## For Implementers

See **Section 12: Implementation Guide** for:
- Optional optimization strategies (iterators, fusion)
- Performance expectations
- Testing requirements

See **Section 13: Standard Library** for:
- Complete reference of all required functions and methods

