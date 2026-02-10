# 9. Error Handling

## 9.1 Catch Method

Provides fallback value on operation failure:
```bloblang
output.count = input.items.length().catch(0)
output.parsed = input.data.parse_json().catch({})
output.value = (input.price * input.quantity).catch(null)
```

**Semantics**: On error anywhere in method chain, returns fallback value and suppresses error propagation.

## 9.2 Or Method

Provides fallback for `null` values:
```bloblang
output.name = input.user.name.or("anonymous")
output.id = input.primary_id.or(input.secondary_id)
```

**Semantics**: Returns fallback if target is `null`; distinct from `.catch()` which handles errors.

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
