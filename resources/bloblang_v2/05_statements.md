# 5. Statements

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

Declares reusable values using `$` prefix:
```
$user_id = input.user.id
$processed = $user_id.string().uppercase()
```

**Syntax**: Variables are declared and referenced using the same `$` prefix.

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
