# 4. Control Flow

## 4.1 If Expressions vs Statements

**If Expression** (returns value, used in assignment):
```bloblang
output.result = if condition { value } else { other_value }

# Without else: field remains unset if condition false
output.category = if score > 80 { "high" }  # category unset if score <= 80
```

**If Statement** (standalone, contains output assignments):
```bloblang
if input.type == "user" {
  output.role = "member"
  output.permissions = ["read"]
}
```

**Distinction:**
- **Expression:** Used in assignment context, contains pure expressions (no `output`/`output@` assignments)
- **Statement:** Standalone, contains `output`/`output@` assignments, **cannot end with expression** (parse error)

If expressions without `else` leave the assignment target **unset** (not null) when condition is false.

## 4.2 Match Expressions vs Statements

**Match Expression** (returns value):
```bloblang
output.sound = match input.animal as a {
  a == "cat" => "meow"
  a == "dog" => "woof"
  _ => "unknown"
}
```

**Match Statement** (multiple assignments):
```bloblang
match input.type() as t {
  t == "object" => {
    output = input.map_each(item -> transform(item.value))
  }
  t == "array" => {
    output = input.map_each(elem -> transform(elem))
  }
  _ => {
    output = input
  }
}
```

**Context binding with `as`** is optional. When omitted, case expressions reference the original matched expression directly:

```bloblang
# Without 'as' (repeat expression)
output.tier = match input.score {
  input.score >= 100 => "gold"
  input.score >= 50 => "silver"
  _ => "bronze"
}

# With 'as' (bind to variable)
output.tier = match input.score as s {
  s >= 100 => "gold"
  s >= 50 => "silver"
  _ => "bronze"
}
```

Use `as` when the matched expression is complex or used multiple times in cases.

## 4.3 Block-Scoped Variables

```bloblang
output.processed = if input.has_discount {
  $rate = input.discount_rate.or(0.10)
  $base = input.price
  $base * (1 - $rate)
} else {
  input.price
}

output.formatted = match input.currency as c {
  c == "USD" => {
    $symbol = "$"
    $amount = input.amount.round(2)
    $symbol + $amount.string()
  }
  _ => {
    $amount = input.amount.round(2)
    c + " " + $amount.string()
  }
}
```
