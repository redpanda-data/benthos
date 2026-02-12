# 4. Control Flow

## 4.1 If Expressions vs Statements

**If Expression** (returns value, used in assignment):
```bloblang
output.result = if condition { value } else { other_value }

# Without else: assignment doesn't execute if condition false
output.category = if input.score > 80 { "high" }
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

**Parsing disambiguation:** The syntactic context determines which form:
- **If statement:** Top-level in mapping, or inside another statement body (where `output` assignments are allowed)
- **If expression:** Inside assignment RHS, variable declarations, lambda bodies, map bodies, or expression contexts

```bloblang
# Statement context (top-level)
if input.type == "user" {
  output.role = "member"     # Statement: assigns to output
}

# Expression context (assignment RHS)
output.value = if input.flag {
  input.value                # Expression: returns value
}

# Expression context (variable declaration)
$result = if input.score > 80 { "high" } else { "low" }

# Expression context (lambda body)
input.items.map_each(x -> if x > 0 { x * 2 } else { 0 })
```

**If expressions without `else`:** When the condition is false, the assignment **does not execute**. The target field is neither created nor modified.

```bloblang
# Field not created if condition false
output.category = if input.score > 80 { "high" }
# If score <= 80: output.category not present in output object
# Reading output.category returns null, but the field doesn't exist structurally

# Preserve existing values
output.status = "pending"
output.status = if false { "override" }  # Assignment skipped, status unchanged
# output.status remains "pending"

# Contrast with explicit null
output.field1 = null                    # Field exists in object, value is null
output.field2 = if false { "value" }    # Field doesn't exist in object
```

**Key distinction:** Reading a non-existent field returns `null`, but this differs from a field that exists with `null` as its value. For JSON serialization, non-existent fields are simply omitted.

## 4.2 Match Expressions vs Statements

**Match Expression** (returns value):
```bloblang
output.sound = match input.animal as a {
  a == "cat" => "meow"
  a == "dog" => "woof"
  _ => "unknown"
}
```

**Exhaustiveness:** Match expressions are **not required** to be exhaustive. If no case matches at runtime, the mapping **throws an error**. Use `_` as a catch-all to handle unexpected values:

```bloblang
# Not exhaustive - will error if animal is "bird"
output.sound = match input.animal {
  "cat" => "meow"
  "dog" => "woof"
}

# Exhaustive - always matches
output.sound = match input.animal {
  "cat" => "meow"
  "dog" => "woof"
  _ => "unknown"  # Catch all other values
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

**Parsing disambiguation:** Like `if`, the syntactic context determines statement vs expression form. Match statements are only valid at top-level or inside other statement bodies.

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
