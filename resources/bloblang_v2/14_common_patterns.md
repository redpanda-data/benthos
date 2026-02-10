# 14. Common Patterns

## 14.1 Copy-and-Modify

```bloblang
output = input
output.password = deleted()
output.updated_at = now()
```

## 14.2 Null-Safe Access

```bloblang
output.name = input.user.name.or("anonymous")
output.id = input.primary_id.or(input.secondary_id).or("default")
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

## 14.5 Recursive Tree Walking

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

## 14.6 Message Expansion

```bloblang
$doc_root = input.without("items")
output = input.items.map_each(item -> $doc_root.merge(item))
```

**Semantics**: Converts single message into array; downstream processors expand into multiple messages.

## 14.7 Complex Conditional Transformations

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

## 14.8 Explicit Context Transformation

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
