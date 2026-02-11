# 5. Statements

Statements are top-level constructs that perform actions or cause side effects. They differ from expressions, which compute and return values.

## Statements vs Expressions

**Statements** (cause side effects):
- Assignment statements: `output.field = value`
- Metadata assignments: `@key = value`
- Variable declarations: `$var = value`
- If statements (Section 6.2)
- Match statements (Section 6.4)

**Expressions** (return values):
- If expressions (Section 6.1)
- Match expressions (Section 6.3)
- Lambda expressions (Section 4.7)
- Path expressions, function calls, method chains, etc.

**Key Distinction**: Expressions can be used anywhere a value is expected and **cannot contain assignments to `output` or metadata**. Statements execute at the top level and can modify the output document and metadata.

## 5.1 Assignment Statement

Assigns expression result to output document path:
```
output.field = expression
output = input                   # Copy entire input
output.user.id = input.id        # Nested field creation
output."special.field" = value   # Quoted field name
```

**Semantics**: Creates intermediate objects as needed. Assignments to `output` build new document.

## 5.2 Metadata Assignment

Assigns to message metadata using `@` prefix:
```
@output_key = input.id
@content_type = "application/json"
@kafka_topic = "new-topic"
```

**Semantics**: Metadata assignments are separate from output document assignments.

## 5.3 Variable Declaration

Declares **immutable** values using `$` prefix:
```
$user_id = input.user.id
$processed = $user_id.string().uppercase()
```

**Syntax**: Variables are declared and referenced using the same `$` prefix.

**Immutability**: Once declared, variables **cannot be reassigned** in the same scope:
```bloblang
$value = 10
$value = 20      # ERROR: cannot reassign variable in same scope
```

**Shadowing**: Inner scopes can declare new variables with the same name, which **shadow** outer variables:
```bloblang
$value = 10
output.outer = $value  # 10

output.inner = if input.flag {
  $value = 20          # NEW variable in inner scope (shadows outer)
  $value               # Returns 20
}

output.still_outer = $value  # Still 10 (outer variable unchanged)
```

**Scope**: Variables can be declared at top-level or within blocks (`if`, `match`, lambda bodies). Block-scoped variables are only accessible within their declaring block and nested blocks.

**Block-Scoped Variables**:
```bloblang
# Variables scoped to conditional blocks
output.age = if input.birthdate != null {
  $parsed = input.birthdate.ts_parse("2006-01-02")
  $parsed.ts_unix()
} else {
  null
}

# Variables scoped to match branches
output.formatted = match input.type as t {
  t == "timestamp" => {
    $fmt = input.format.or("2006-01-02")
    $parsed = input.value.ts_parse($fmt)
    $parsed.ts_format("Mon Jan 2")
  }
  t == "number" => {
    $val = input.value.number()
    $val.round(2)
  }
  _ => input.value
}

# Variables in nested scopes
output.result = if input.enabled {
  $outer = input.value
  if $outer > 100 {
    $inner = $outer / 2  # Scoped to inner if
    $inner.floor()
  } else {
    $outer  # Can still access outer scope
  }
}
```

**Shadowing**: Block-scoped variables may shadow variables from outer scopes. The innermost declaration takes precedence.

## 5.4 Deletion

Removes fields using `deleted()` function:
```
output.password = deleted()           # Remove field
output = if input.spam { deleted() }  # Filter message
```

**Semantics**: Assigning `deleted()` to a field excludes it from output. Assigning to `output` filters entire message.
