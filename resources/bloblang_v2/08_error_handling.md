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

Handle errors with `.catch()`. The method takes a lambda with a single parameter — the error object — and is called only when the expression to its left produces an error. If the expression succeeds, `.catch()` returns its value unchanged. If the lambda itself errors, that error propagates and can be caught by a subsequent `.catch()`.

**The error object** has a single field:
- `.what` — a string containing the error message

```bloblang
# Inspect the error
output.parsed = input.date.ts_parse("2006-01-02").catch(err -> {
  $msg = "parse failed: " + err.what
  throw($msg)
})

# Ignore the error, provide fallback value
output.parsed = input.date.ts_parse("2006-01-02").catch(err -> null)

# Chain multiple attempts
output.parsed = input.date
  .ts_parse("2006-01-02")                                    # Try format 1
  .catch(err -> input.date.ts_parse("2006/01/02"))           # If format 1 fails, try format 2
  .catch(err -> null)                                        # If format 2 also fails, use null
```

## 8.3 Or Method

Provide default for null values. `.or()` uses **short-circuit evaluation**: the argument expression is only evaluated if the receiver value is null. If the receiver is non-null, the argument is never evaluated and the receiver value is returned directly.

```bloblang
output.name = input.user.name.or("Anonymous")
output.count = input.items?.length().or(0)

# Short-circuit: throw() is only evaluated if name is null
output.name = input.name.or(throw("name is required"))
```

## 8.4 Throw Function

Throw custom errors. `throw()` requires exactly one string argument:
```bloblang
output.value = if input.value != null {
  input.value
} else {
  throw("Value is required")
}
```

Non-string arguments are a compile-time error:
```bloblang
throw(42)     # ERROR: throw() requires a string argument
throw(null)   # ERROR: throw() requires a string argument
throw()       # ERROR: throw() requires exactly one string argument
```

**Error propagation:** `throw()` produces an error that propagates like any other error. It can be caught with `.catch()`:
```bloblang
# Caught: provides fallback value
output.result = throw("bad value").catch(err -> "fallback")  # "fallback"

# Caught with error inspection
output.result = throw("bad value").catch(err -> {
  $default = "fallback"
  $default  # err.what == "bad value"
})

# Caught in expression context
output.name = input.name.or(throw("name is required")).catch(err -> "Anonymous")

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

**`.catch(lambda)`**: Handles errors, not `null`
```bloblang
input.date.ts_parse("format").catch(err -> null)  # null if parse fails
```

**`.or()`**: Handles `null`, not errors. Short-circuits: argument only evaluated if receiver is null.
```bloblang
input.name.or("default")  # "default" if name is null
```

**Combine for both:**
```bloblang
input.user?.age.or(0).catch(err -> -1)
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
input.user?.address?.city.or("Unknown")  # Combine null-safe navigation with defaults
```

**When a method returns null:** The null propagates to the next operation:
```bloblang
[null, "a"].first().uppercase()       # ERROR: first() returns null, uppercase requires string
[null, "a"].first()?.uppercase()      # OK: returns null (null-safe skips uppercase)

# first() errors on empty arrays — use .catch() for fallback
input.items.first().uppercase()                    # ERROR if array is empty
input.items.first().catch(err -> "").uppercase()    # OK: provides default on empty array
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
