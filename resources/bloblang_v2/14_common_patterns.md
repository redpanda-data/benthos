# 14. Common Patterns

## 14.1 Copy-and-Modify

```bloblang
output = input
output.password = deleted()
output.updated_at = now()
```

## 14.2 Null-Safe Access

```bloblang
# Using .or() method for final value
output.name = input.user.name.or("anonymous")
output.id = input.primary_id.or(input.secondary_id).or("default")

# Using ?. operator for nested navigation (V2)
output.city = input.user?.address?.city
output.email = input.contact?.primary?.email.or("no-email@example.com")

# Null-safe array access
output.first_name = input.users?[0]?.name
output.last_item = input.data?[-1]?.value

# Complex null-safe chains
output.product = input.order?.items?[0]?.product?.name.or("Unknown")
output.nested = input.a?.b?.c?.d?.e.or("default")

# Mixed safe and unsafe (be explicit about optionality)
output.user_city = input.user?.address.city     # user is optional, address is required if user exists
output.full_safe = input.user?.address?.city    # both user and address are optional
```

## 14.3 Error-Safe Parsing

```bloblang
output.parsed = if input.date != null {
  $date_str = input.date
  $date_str.ts_parse("2006-01-02").catch(
    $date_str.ts_parse("2006/01/02")
  ).catch(null)
} else {
  null
}
```

## 14.4 Array Transformation Pipeline

```bloblang
output.results = input.items
  .filter(item -> item.active)
  .map_each(item -> item.name.uppercase())
  .sort()
  .join(", ")
```

## 14.5 Array Indexing and Access

```bloblang
# Safe array access with fallbacks
output.first_item = input.items[0].catch(null)
output.last_item = input.items[-1].catch(null)

# Extract specific elements
output.top_three = {
  "first": input.results[0],
  "second": input.results[1],
  "third": input.results[2]
}

# Negative indexing for tail elements
output.recent = {
  "latest": input.events[-1],
  "previous": input.events[-2],
  "before_that": input.events[-3]
}

# Dynamic indexing
$position = input.selected_index
output.selected = input.options[$position].catch("invalid selection")

# Nested array access
output.matrix_value = input.grid[2][3]
output.deep = input.data[0].items[5].values[-1]

# Combine with method chaining
output.first_name = input.users[0].name.uppercase()
output.last_email = input.contacts[-1].email.lowercase()

# Safe access pattern for optional arrays
output.primary_tag = if input.tags.type() == "array" && input.tags.length() > 0 {
  input.tags[0]
} else {
  "untagged"
}

# Process head and tail
$first = input.items[0]
$last = input.items[-1]
output.summary = {
  "first_id": $first.id,
  "last_id": $last.id,
  "total": input.items.length()
}
```

## 14.6 Recursive Tree Walking

```bloblang
map walk(node) {
  output = match node.type() as type {
    type == "object" => node.map_each(item -> item.value.apply("walk"))
    type == "array" => node.map_each(elem -> elem.apply("walk"))
    type == "string" => node.uppercase()
    _ => node
  }
}
output = input.apply("walk")
```

## 14.7 Message Expansion

```bloblang
$doc_root = input.without("items")
output = input.items.map_each(item -> $doc_root.merge(item))
```

**Semantics**: Converts single message into array; downstream processors expand into multiple messages.

## 14.8 Complex Conditional Transformations

Explicit context binding enables clear, predictable transformations:

```bloblang
# Processing user records with conditional enrichment
output.user = if input.user_type == "premium" {
  $base_discount = 0.20
  $loyalty_bonus = if input.loyalty_years > 5 { 0.05 } else { 0 }
  $total_discount = $base_discount + $loyalty_bonus

  {
    "id": input.user_id,
    "name": input.name,
    "tier": "premium",
    "discount_rate": $total_discount,
    "benefits": ["priority_support", "free_shipping"]
  }
} else if input.user_type == "standard" {
  $base_discount = 0.10

  {
    "id": input.user_id,
    "name": input.name,
    "tier": "standard",
    "discount_rate": $base_discount,
    "benefits": ["standard_support"]
  }
} else {
  {
    "id": input.user_id,
    "name": input.name,
    "tier": "basic",
    "discount_rate": 0,
    "benefits": []
  }
}

# Processing timestamps with explicit match binding
output.timestamp = match input.date_format as format {
  format == "iso8601" => {
    $parsed = input.date.ts_parse("2006-01-02T15:04:05Z07:00")
    $parsed.ts_unix()
  }
  format == "unix" => {
    $ts = input.date.number()
    $ts
  }
  format == "custom" => {
    $fmt = input.format_string
    $parsed = input.date.ts_parse($fmt)
    $parsed.ts_unix()
  }
  _ => {
    $parsed = input.date.ts_parse("2006-01-02")
    $parsed.ts_unix()
  }
}
```

## 14.9 Explicit Context Transformation

```bloblang
# Transform nested objects with explicit naming
output.formatted_users = input.users.map_each(user -> {
  "id": user.id,
  "display_name": user.first_name + " " + user.last_name,
  "orders": user.orders.map_each(order -> {
    "order_id": order.id,
    "total": order.total,
    "items": order.items.length()
  })
})

# Use parenthesized context with explicit naming
output.result = input.data.(data -> {
  "sum": data.a + data.b,
  "product": data.a * data.b,
  "nested": data.inner.(inner -> inner.value)
})
```
