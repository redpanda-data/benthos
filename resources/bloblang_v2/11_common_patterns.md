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
output.uppercased = input.data.map_values(v -> v.trim().uppercase())
```

## Indexing Patterns

```bloblang
# Arrays — null-safe handles missing field, catch handles out-of-bounds
output.first = input.items?[0].catch(err -> null)
output.last = input.items?[-1].catch(err -> null)

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
  match node.type() {
    "object" => node.map_values(v -> walk(v)),
    "array" => node.map(elem -> walk(elem)),
    "string" => node.uppercase(),
    _ => node,
  }
}
output = walk(input)
```

## Unary Minus with Methods

```bloblang
# Method calls bind tighter than unary minus
# -10.string()      # ERROR: parses as -(10.string()) = -("10")
(-10).string()      # OK: "-10"
(-3.14).abs()       # OK: 3.14
```

## Boolean Dispatch

```bloblang
# match equality form cannot match boolean values — use if/else instead
output.label = if input.flag { "yes" } else { "no" }

# Else-if chains for multi-way branching
output.tier = if input.score >= 90 {
  "gold"
} else if input.score >= 50 {
  "silver"
} else {
  "bronze"
}

# For multi-way boolean dispatch, use match-without-expression or match-with-as
output.status = match {
  input.enabled && input.verified => "active",
  input.enabled => "pending",
  _ => "disabled",
}
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

output.timestamp = match input.date_format {
  "iso8601" => input.date.ts_parse("%Y-%m-%dT%H:%M:%S%z").ts_unix(),
  "unix" => input.date.int64(),
  _ => input.date.ts_parse("%Y-%m-%d").ts_unix(),
}
```

## Pitfall: Lambda Capture in Loops

Lambdas capture variables by reference (late binding) — they see the variable's value at invocation time, not at creation time. This can surprise you when building lambdas in a loop-like pattern:

```bloblang
# BUG: all three lambdas see the final value of $multiplier (3)
$multiplier = 1
$fn1 = x -> x * $multiplier
$multiplier = 2
$fn2 = x -> x * $multiplier
$multiplier = 3
$fn3 = x -> x * $multiplier

output.a = $fn1(10)  # 30 (not 10!) — $multiplier is 3 when $fn1 runs
output.b = $fn2(10)  # 30 (not 20!)
output.c = $fn3(10)  # 30
```

Also remember that assignments inside a lambda shadow the outer variable rather than modifying it (Section 3.4). This means reads and writes are asymmetric: reads see the outer scope's current value, but writes create a local copy.
