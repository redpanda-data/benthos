# 9. Error Handling

## 9.1 Catch Method

Provides fallback value on operation failure:
```bloblang
output.count = input.items.length().catch(0)
output.parsed = input.data.parse_json().catch({})
output.value = (input.price * input.quantity).catch(null)

# Array index out of bounds
output.first = input.items[0].catch(null)         # Null if array is empty
output.tenth = input.items[9].catch("default")    # Fallback if fewer than 10 elements
output.last = input.items[-1].catch(null)         # Null if array is empty
```

**Semantics**: On error anywhere in method chain, returns fallback value and suppresses error propagation.

**Common Error Scenarios**:
- Type mismatches (e.g., calling `.uppercase()` on a number)
- Parsing failures (e.g., `.parse_json()` on invalid JSON)
- Array index out of bounds (positive or negative)
- Null pointer access
- Division by zero
- Method not applicable to type

## 9.1.1 Null-Safe Operators vs Catch

The null-safe operators and `.catch()` handle different failure modes:

```bloblang
# Null-safe operator: handles null values during navigation
output.value = input.data?.field              # null if data is null
output.item = input.items?[0]                 # null if items is null

# Catch: handles errors during operations
output.parsed = input.json.parse_json().catch({})     # {} if parse fails
output.number = input.text.number().catch(0)          # 0 if conversion fails

# Combining both: handle null + errors
output.result = input.data?.parse_json().catch({})    # null if data is null, {} if parse fails
output.value = input.users?[0]?.age.number().catch(0) # null if navigation fails, 0 if conversion fails

# Different failure modes
output.a = input.user?.name                   # null if user is null (no error)
output.b = input.user.name.catch("unknown")   # "unknown" if accessing name fails (e.g., user is number)
output.c = input.user?.name.catch("unknown")  # null if user is null, "unknown" if name access fails
```

**Decision Guide**:
- Use `?.` for optional fields that might not exist
- Use `.catch()` for operations that might fail (parsing, type conversion, etc.)
- Use both when navigating optional fields and performing fallible operations

## 9.2 Or Method

Provides fallback for `null` values:
```bloblang
output.name = input.user.name.or("anonymous")
output.id = input.primary_id.or(input.secondary_id)
```

**Semantics**: Returns fallback if target is `null`; distinct from `.catch()` which handles errors.

## 9.2.1 Null-Safe Operators vs Or Method

The null-safe operators (`?.` and `?[`) and `.or()` method serve different purposes:

```bloblang
# Null-safe operator: prevents errors during navigation
output.city = input.user?.address?.city        # null if user or address is null

# Or method: provides fallback for final null value
output.city = input.user?.address?.city.or("Unknown")

# Without null-safe operator: would error if user is null
output.city = input.user.address.city.or("Unknown")  # Error if user is null!

# Combination: safe navigation + fallback
output.name = input.user?.profile?.name.or("Anonymous")
```

**When to Use Each**:
- Use `?.` when navigating potentially null/missing nested fields
- Use `.or()` when you need a default value for a null result
- Combine both for safe navigation with defaults

## 9.3 Throw Function

Manually raises errors with custom messages:
```bloblang
output.value = if input.required_field == null {
  throw("Missing required field")
} else {
  input.required_field
}
```

## 9.4 Validation Methods

Type validation methods throw errors on failure:
```bloblang
output.count = input.count.number()      # Error if not number
output.name = input.name.not_null()      # Error if null
output.items = input.items.not_empty()   # Error if empty
```
