# 11. Common Patterns

## Copy-and-Modify

```bloblang
output = input
output.password = deleted()
output.updated_at = now()

output@ = input@
output@.kafka_topic = "processed"
```

## Null-Safe Access

```bloblang
output.city = input.user?.address?.city
output.email = input.contact?.email.or("no-email@example.com")
output.first = input.users?[0]?.name
output.product = input.order?.items?[0]?.product?.name.or("Unknown")
```

## Error-Safe Parsing

```bloblang
output.parsed = input.date
  .ts_parse("%Y-%m-%d")
  .catch(err -> input.date.ts_parse("%Y/%m/%d"))
  .catch(err -> null)
```

## Array Transformation

```bloblang
# Filter, map, sort
output.results = input.items
  .filter(item -> item.active)
  .map(item -> item.name.uppercase())
  .sort()

# Object transformation
output.uppercased = input.data.iter_kv()
  .map(e -> {"k": e.k, "v": e.v.trim().uppercase()})
  .collect_kv()
```

## Indexing Patterns

```bloblang
# Arrays
output.first = input.items[0].catch(err -> null)
output.last = input.items[-1].catch(err -> null)

# Strings (codepoint position, returns int64)
output.first_codepoint = input.name[0]          # int64 Unicode codepoint

# Dynamic indexing
output.selected = input.options[input.index].catch(err -> "invalid")
```

## Metadata Routing

```bloblang
output@ = input@
output@.kafka_topic = if input.priority == "high" {
  "urgent-topic"
} else {
  "normal-topic"
}

# Enrich from document
output@.kafka_key = input.user_id
output@.content_type = "application/json"
```

## Recursive Tree Walking

```bloblang
map walk(node) {
  match node.type() as t {
    t == "object" => node.iter_kv().map(e -> {"k": e.k, "v": walk(e.v)}).collect_kv(),
    t == "array" => node.map(elem -> walk(elem)),
    t == "string" => node.uppercase(),
    _ => node,
  }
}
output = walk(input)
```

## Complex Conditional Transformations

```bloblang
output.user = if input.user_type == "premium" {
  $discount = 0.20 + (if input.loyalty_years > 5 { 0.05 } else { 0 })
  {
    "id": input.user_id,
    "tier": "premium",
    "discount_rate": $discount
  }
} else {
  {"id": input.user_id, "tier": "basic", "discount_rate": 0}
}

output.timestamp = match input.date_format as f {
  f == "iso8601" => input.date.ts_parse("%Y-%m-%dT%H:%M:%S%z").ts_unix(),
  f == "unix" => input.date.int64(),
  _ => input.date.ts_parse("%Y-%m-%d").ts_unix(),
}
```
