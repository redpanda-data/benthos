# 8. Error Handling

## 8.1 Error Propagation

Errors propagate through expressions:
```bloblang
output.parsed = input.date.ts_parse("%Y-%m-%d")
# Throws error if parsing fails
```

Common error sources:
- Type mismatches (e.g., `5 + "text"`)
- Failed method calls (e.g., parsing, out-of-bounds access)
- Explicit `throw()` calls

## 8.2 Catch Method

Handle errors with `.catch()`. The method takes a lambda with a single parameter — the error object — and is called only when the expression to its left produces an error. If the expression succeeds, `.catch()` returns its value unchanged. If the lambda itself errors, that error propagates and can be caught by a subsequent `.catch()`.

**Scope:** `.catch()` catches any error produced by its receiver expression — the entire expression that the grammar parses as the left-hand side of the `.catch()` method call. Errors propagate through method chains: if any method errors, subsequent methods are skipped and the error flows to the next `.catch()`.

```bloblang
# Catches errors from ts_parse or uppercase (either one)
input.date.ts_parse("%Y-%m-%d").uppercase().catch(err -> null)

# Parentheses define the boundary — catches errors from the addition and .string()
(input.a + input.b).string().catch(err -> "0")

# Catches errors from .map() (e.g., lambda errors), not from inside individual elements
input.items.map(x -> x.value / x.count).catch(err -> [])
```

All runtime errors are catchable with `.catch()` — the sole exception is exceeding the recursion limit (Section 5.2), which halts execution immediately.

**Void passes through `.catch()` unchanged.** Void is not an error — it is the absence of a value. `.catch()` only activates on errors, so void flows through transparently. If void then encounters a method that requires a value, *that* produces an error which can be caught by a subsequent `.catch()`:
```bloblang
(if false { 1 }).catch(err -> 0)                    # void (catch not triggered, no error occurred)
(if false { 1 }).string().catch(err -> "boo!")       # "boo!" (.string() errors on void, catch triggers)
```

**The error object** is a plain object (`{"what": "..."}`) with a single field:
- `.what` — a string containing the error message

The error is structured as an object (rather than a plain string) to allow future extension with additional fields (e.g., error codes, source locations) without breaking existing handlers.

```bloblang
# Inspect the error
output.parsed = input.date.ts_parse("%Y-%m-%d").catch(err -> {
  $msg = "parse failed: " + err.what
  throw($msg)
})

# Ignore the error, provide fallback value
output.parsed = input.date.ts_parse("%Y-%m-%d").catch(err -> null)

# Chain multiple attempts
output.parsed = input.date
  .ts_parse("%Y-%m-%d")                                    # Try format 1
  .catch(err -> input.date.ts_parse("%Y/%m/%d"))           # If format 1 fails, try format 2
  .catch(err -> null)                                        # If format 2 also fails, use null
```

## 8.3 Or Method

Provide default for null, void, or deleted values. `.or()` uses **short-circuit evaluation**: the argument expression is only evaluated if the receiver is null, void, or `deleted()`. If the receiver has a value, the argument is never evaluated and the receiver value is returned directly.

`.or()` and `.catch()` are the only methods that can be called on void or `deleted()` — all other method calls on void or `deleted()` are errors. `.catch()` passes void and `deleted()` through unchanged (they are not errors), while `.or()` actively rescues them by returning its argument. This makes `.or()` useful for providing defaults in deeply nested expressions involving if-without-else, non-exhaustive match, or expressions that may yield `deleted()`:

```bloblang
output.name = input.user.name.or("Anonymous")
output.count = input.items?.length().or(0)

# Short-circuit: throw() is only evaluated if name is null
output.name = input.name.or(throw("name is required"))

# Rescues void from if-without-else
output.label = (if input.premium { "VIP" }).or("standard")

# Rescues void from non-exhaustive match
output.sound = (match input.animal { "cat" => "meow", "dog" => "woof" }).or("unknown")

# Rescues deleted() — useful when calling maps that may return deleted()
output.field = some_map(input.value).or("placeholder")
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

**Null-safe operators** (`?.`, `?[]`, `?.method()`): Handle `null`, not errors
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

**`.or()`**: Handles `null`, `void`, and `deleted()`, not errors. Short-circuits: argument only evaluated if receiver is null, void, or deleted. If the receiver is an error, the error propagates through `.or()` uncaught.
```bloblang
input.name.or("default")                                   # "default" if name is null
(if false { "hello" }).or("world")                         # "world" (void rescued)
(match input.x { "a" => 1 }).or(0)                        # 0 if no case matched (void rescued)
some_map(input.value).or("fallback")                       # "fallback" if map returned deleted()
(5 / 0).or("default")                                      # ERROR propagates: .or() does not catch errors
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
input.items[0]?.uppercase()      # OK: returns null if first element is null (null-safe skips uppercase)
input.items[0].uppercase()       # ERROR if first element is null (uppercase requires string)

# Out-of-bounds errors — use .catch() for fallback
input.items[0].catch(err -> "").uppercase()    # OK: provides default on empty array
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
