# 8. Error Handling

## 8.1 Error Propagation

Errors propagate through expressions:
```bloblang
output.parsed = input.date.ts_parse("2006-01-02")
# Throws error if parsing fails

output.sound = match input.animal {
  "cat" => "meow",
  "dog" => "woof",
}
# Throws error if animal is neither "cat" nor "dog" (non-exhaustive match)
```

Common error sources:
- Type mismatches (e.g., `5 + "text"`)
- Failed method calls (e.g., parsing, out-of-bounds access)
- Non-exhaustive match expressions
- Explicit `throw()` calls

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

Throw custom errors:
```bloblang
output.value = if input.value != null {
  input.value
} else {
  throw("Value is required")
}
```

**Error propagation:** `throw()` produces an error that propagates like any other error. It can be caught with `.catch()`:
```bloblang
# Caught: provides fallback value
output.result = throw("bad value").catch("fallback")  # "fallback"

# Caught in expression context
output.name = input.name.or(throw("name is required")).catch("Anonymous")

# Uncaught: halts the mapping
output.result = throw("fatal error")  # No .catch(), stops execution
```

When a `throw()` error is **not caught** by `.catch()`, it halts the entire mapping and no subsequent statements execute.

## 8.5 Null-Safe vs Error-Safe

**Null-safe operators** (`?.`, `?[]`): Handle `null`, not errors
```bloblang
input.user?.name    # null if user is null, error if user is non-object

# ?.  only short-circuits on null, not type mismatches
null?.name          # OK: returns null
input.user?.name    # OK: returns null if user is null, or user.name if user is object
"string"?.name      # ERROR: cannot access field on string (not null, wrong type)
5?.name             # ERROR: cannot access field on int64 (not null, wrong type)
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

## 8.6 Method Chaining with Null

**Method type requirements:** Methods work on specific types, and calling a method on an incompatible type (including null) results in an error. Some methods like `.type()` accept any type including null, while data transformation methods typically require specific types.

```bloblang
# Method requires specific type (string)
input.value.uppercase()     # ERROR if value is null (or any non-string type)

# Use null-safe operator to skip method call
input.value?.uppercase()    # Returns null if value is null (method not called)

# Method accepts any type including null
input.value.type()          # Returns "null" if value is null (method called)

# Chaining with null-safe operators
input.items.first()?.uppercase()      # Skip uppercase if first() returns null
input.user?.address?.city.or("Unknown")  # Combine null-safe navigation with defaults
```

**When a method returns null:** The null propagates to the next operation:
```bloblang
input.items.first().uppercase()       # ERROR if first() returns null (empty array)
input.items.first()?.uppercase()      # OK: returns null if first() returns null
input.items.first().or("").uppercase() # OK: provides default before uppercase
```

## 8.7 Validation Methods

```bloblang
# type() - check type
# Type checking - check for any signed integer type
output.valid = if [ "int32", "int64" ].contains(input.value.type()) {
  input.value
} else {
  throw("Value must be a signed integer")
}
```
