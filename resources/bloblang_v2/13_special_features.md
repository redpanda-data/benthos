# 13. Special Features

## 13.1 Non-Structured Data

`content()` function retrieves raw message bytes for unstructured data processing:
```bloblang
output = content().string().uppercase()
```

Assigning primitive values to `output` produces non-structured output:
```bloblang
output = "plain text output"
output = 42
```

## 13.2 Dynamic Field Names

Computed field names in objects:
```bloblang
output = {
  input.key_field: input.value_field
}
```

## 13.3 Conditional Literals

If expressions and `deleted()` within array and object literals:
```bloblang
output = {
  "id": input.id,
  "name": if input.name != null { input.name },
  "age": if input.age > 0 { input.age } else { deleted() }
}
```

**Semantics**: Omitted branches skip field creation; `deleted()` removes field from literal.

## 13.4 Message Filtering

Assigning `deleted()` to `output` filters entire message from pipeline:
```bloblang
output = if input.spam { deleted() }
```

## 13.5 Command-Line Execution

`rpk connect blobl` subcommand executes Bloblang scripts directly, treating each input line as separate JSON document.
