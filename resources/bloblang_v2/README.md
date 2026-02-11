# Bloblang Language Technical Specification

**Version:** 2.0
**Date:** 2026-02-10

## About This Specification

This directory contains the complete technical specification for Bloblang (blobl), a domain-specific mapping language for transforming structured and unstructured data within stream processing pipelines. The specification has been organized into focused sections for easy navigation and reference.

## Key Principle

Bloblang emphasizes **explicit context management**: all data contexts must be explicitly named, eliminating implicit context shifting.

## Development Context

This V2 specification evolved from analysis of V1 language weaknesses:

- **[DEVELOPMENT_GUIDE.md](DEVELOPMENT_GUIDE.md)** - **START HERE** for continuation: design principles, key changes, how to proceed
- **[V1_BASELINE.md](V1_BASELINE.md)** - Original V1 specification (baseline for comparison)
- **[ISSUES_TO_ADDRESS.md](ISSUES_TO_ADDRESS.md)** - Detailed analysis of 13 identified V1 weaknesses
- **[PROPOSED_SOLUTIONS.md](PROPOSED_SOLUTIONS.md)** - Solutions addressing each issue with implementation phases
- **[BLOBLANG_V2_PROGRESS.md](BLOBLANG_V2_PROGRESS.md)** - Development progress tracker showing completed and pending solutions

## Table of Contents

1. **[Overview](01_overview.md)** - Introduction to Bloblang and its core design philosophy

2. **[Lexical Structure](02_lexical_structure.md)** - Tokens, identifiers, literals, and comments

3. **[Type System](03_type_system.md)** - Runtime types and type introspection

4. **[Expressions](04_expressions.md)** - Path expressions, operators, functions, methods, and lambda expressions

5. **[Statements](05_statements.md)** - Assignment, metadata assignment, variable declaration, and deletion

6. **[Control Flow](06_control_flow.md)** - If expressions/statements and match expressions/statements

7. **[Maps (Named Mappings)](07_maps.md)** - Reusable transformation definitions with explicit parameters

8. **[Imports](08_imports.md)** - Importing mappings from external files

9. **[Error Handling](09_error_handling.md)** - Catch method, or method, throw function, and validation methods

10. **[Metadata](10_metadata.md)** - Accessing and modifying message metadata

11. **[Execution Model](11_execution_model.md)** - Mapping processor vs mutation processor, evaluation order

12. **[Context and Scoping](12_context_and_scoping.md)** - Input/output contexts, explicit context in lambdas and match, variable scope

13. **[Special Features](13_special_features.md)** - Non-structured data, dynamic field names, conditional literals, message filtering

14. **[Common Patterns](14_common_patterns.md)** - Practical examples including copy-and-modify, null-safe access, error-safe parsing, array transformation, recursive tree walking, and complex conditional transformations

15. **[Grammar Summary](15_grammar.md)** - Formal grammar definition

16. **[Type Coercion Rules](16_type_coercion.md)** - Type coercion behavior for operators

17. **[Built-in Functions and Methods](17_builtin_functions.md)** - Overview of available functions and methods

## Quick Start Examples

### Basic Assignment
```bloblang
output.field = input.field
output.user.id = input.id
```

### Conditional Transformation
```bloblang
output.category = if input.score >= 80 {
  "high"
} else if input.score >= 50 {
  "medium"
} else {
  "low"
}
```

### Pattern Matching with Explicit Context
```bloblang
output.sound = match input.animal as animal {
  animal == "cat" => "meow"
  animal == "dog" => "woof"
  _ => "unknown"
}
```

### Array Transformation
```bloblang
# Simple pipeline
output.results = input.items
  .filter(item -> item.active)
  .map_each(item -> item.name.uppercase())
  .sort()

# Multi-statement lambdas
output.enriched = input.items.map_each(item -> {
  $base_price = item.price * item.quantity
  $tax = $base_price * 0.1
  $base_price + $tax
})

# Multi-parameter lambdas
output.total = input.items.reduce((acc, item) -> acc + item.price, 0)

# Stored lambda functions
$calculate = (price, qty, rate) -> price * qty * (1 + rate)
output.cost = $calculate(input.price, input.qty, 0.1)
```

### Indexing (Arrays, Strings, Bytes)
```bloblang
# Array indexing
output.first = input.items[0]              # First element
output.last = input.items[-1]              # Last element
output.user_name = input.users[2].name     # Chained access

# String indexing (byte position)
output.initial = input.name[0]             # First character
output.last_char = input.text[-1]          # Last character

# Safe access with fallback
output.safe_first = input.items[0].catch(null)
output.safe_char = input.text[5].catch("")
```

### Null-Safe Navigation
```bloblang
# Null-safe field access
output.city = input.user?.address?.city    # null if any field is null

# Null-safe array indexing
output.first_name = input.users?[0]?.name  # null if users is null or empty

# Combine with .or() for defaults
output.email = input.contact?.email.or("no-email@example.com")
```

### Metadata
```bloblang
# Read input metadata (immutable)
output.original_topic = input@.kafka_topic
output.message_key = input@.kafka_key

# Write output metadata (mutable)
output@.kafka_topic = "processed-topic"
output@.content_type = "application/json"
output@.kafka_key = input.id

# Copy and modify
output@ = input@                           # Copy all metadata
output@.kafka_topic = "new-topic"          # Override specific key
```

### Named Map
```bloblang
map extract_user(data) {
  output.id = data.user_id
  output.name = data.full_name
  output.email = data.contact.email
}

output.customer = input.customer_data.apply("extract_user")
```

## Additional Resources

For full function and method reference, use:
```bash
rpk connect blobl --list-functions
rpk connect blobl --list-methods
```

For command-line execution:
```bash
rpk connect blobl
```

## Version History

- **Version 2.0** (2026-02-10) - Complete specification with explicit context management
