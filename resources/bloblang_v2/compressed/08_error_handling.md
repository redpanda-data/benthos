# 8. Error Handling

## 8.1 Error Propagation

Errors propagate through expressions:
```bloblang
output.parsed = input.date.ts_parse("2006-01-02")
# Throws error if parsing fails
```

## 8.2 Catch Method

Handle errors with `.catch()`:
```bloblang
# Provide fallback value
output.parsed = input.date.ts_parse("2006-01-02").catch(null)

# Chain multiple attempts
output.parsed = input.date
  .ts_parse("2006-01-02")
  .catch(input.date.ts_parse("2006/01/02"))
  .catch(null)
```

## 8.3 Or Method

Provide default for null values:
```bloblang
output.name = input.user.name.or("Anonymous")
output.count = input.items.length().or(0)
```

## 8.4 Throw Function

Throw custom errors that **stop execution immediately**:
```bloblang
output.value = if input.value != null {
  input.value
} else {
  throw("Value is required")  # Halts mapping with error
}

# Assignment never happens if throw executes
output.result = throw("error")  # Stops execution, result unset
```

`throw()` halts the entire mapping with an error. No subsequent statements execute.

## 8.5 Null-Safe vs Error-Safe

**Null-safe operators** (`?.`, `?[]`): Handle `null`, not errors
```bloblang
input.user?.name    # null if user is null, error if name access fails
```

**`.catch()`**: Handles errors, not `null`
```bloblang
input.date.ts_parse("format").catch(null)  # null if parse fails
```

**`.or()`**: Handles `null`, not errors
```bloblang
input.name.or("default")  # "default" if name is null
```

**Combine for both:**
```bloblang
input.user?.age.or(0).catch(-1)
# null-safe → default for null → fallback for errors
```

## 8.6 Validation Methods

```bloblang
# exists() - check if field exists
output.has_name = input.user.name.exists()

# type() - check type
output.valid = if input.value.type() == "number" {
  input.value
} else {
  throw("Value must be number")
}
```
