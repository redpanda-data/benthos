# 6. Control Flow

## 6.1 If Expression

Conditional expression returning a value:
```bloblang
output.category = if input.score >= 80 {
  "high"
} else if input.score >= 50 {
  "medium"
} else {
  "low"
}
```

**Syntax**: `if condition { true_branch } [else if condition { branch }]* [else { false_branch }]`

**Semantics**: Returns value of executed branch. Omitting `else` with no match skips assignment.

**With Block-Scoped Variables**:
```bloblang
# Variables scoped to branches
output.processed = if input.has_discount {
  $rate = input.discount_rate.or(0.10)
  $base = input.price
  $base * (1 - $rate)
} else {
  input.price
}

# Complex transformations with local variables
output.age_years = if input.birthdate != null {
  $parsed = input.birthdate.ts_parse("2006-01-02")
  $now_ts = now()
  $age_seconds = $now_ts.ts_unix() - $parsed.ts_unix()
  ($age_seconds / 31536000).floor()
} else {
  null
}
```

### Purity Constraints for If Expressions

**If expressions must be pure** - they cannot modify external state (output or metadata):

```bloblang
# ❌ FORBIDDEN: Cannot assign to output inside if expression
output.result = if input.enabled {
  output.log = "enabled"  # ERROR: output assignments not allowed
  "yes"
}

# ❌ FORBIDDEN: Cannot assign to output inside if expression
output.value = if input.track {
  output@.counter = output@.counter + 1  # ERROR: output assignments not allowed
  input.value
}

# ✅ ALLOWED: Pure computations with local variables
output.result = if input.enabled {
  $value = input.amount * 2
  $processed = $value.floor()
  $processed
}
```

**Rationale**: If expressions are expressions that return values, not statements that cause side effects. Use if statements (see 6.2) when you need to make output/metadata assignments.

## 6.2 If Statement

Conditional execution of multiple assignments without return value:
```bloblang
if input.type == "user" {
  output.role = "member"
  output.permissions = ["read"]
}
```

**With Block-Scoped Variables**:
```bloblang
if input.process_user {
  $user_id = input.user.id
  $user_name = input.user.name

  output.user_id = $user_id
  output.display_name = $user_name
  output.slug = $user_name.lowercase().replace_all(" ", "-")
}
```

## 6.3 Match Expression

Pattern matching with **explicit context binding** using `as`:
```bloblang
output.sound = match input.animal as animal {
  animal == "cat" => "meow"
  animal == "dog" => "woof"
  animal.contains("bird") => "chirp"
  _ => "unknown"
}
```

**Syntax**: `match expression as name { case => result [, case => result]* }`

**Required Context Binding**: The `as name` clause is **required** to explicitly name the matched value.

**Cases**: Boolean expressions referencing the named parameter, or `_` (catch-all).

**Semantics**: Evaluates cases sequentially; returns first matching result.

**With Block-Scoped Variables**:
```bloblang
output.formatted_price = match input.currency as currency {
  currency == "USD" => {
    $symbol = "$"
    $amount = input.amount.round(2)
    $symbol + $amount.string()
  }
  currency == "EUR" => {
    $symbol = "€"
    $amount = input.amount.round(2)
    $amount.string() + $symbol
  }
  currency == "JPY" => {
    $symbol = "¥"
    $amount = input.amount.floor()
    $symbol + $amount.string()
  }
  _ => {
    $amount = input.amount.round(2)
    currency + " " + $amount.string()
  }
}
```

**Boolean Match** (without expression):
```bloblang
output.category = match {
  input.score >= 80 => "high"
  input.score >= 50 => "medium"
  _ => "low"
}
```

### Purity Constraints for Match Expressions

**Match expressions must be pure** - they cannot modify external state (output or metadata):

```bloblang
# ❌ FORBIDDEN: Cannot assign to output inside match expression
output.result = match input.type as type {
  type == "A" => {
    output.log = "type A"  # ERROR: output assignments not allowed
    "result A"
  }
  _ => "unknown"
}

# ❌ FORBIDDEN: Cannot assign to output inside match expression
output.category = match input.level as level {
  level > 10 => {
    output@.high_level_count = output@.high_level_count + 1  # ERROR: output assignments not allowed
    "high"
  }
  _ => "low"
}

# ✅ ALLOWED: Pure computations with local variables
output.result = match input.type as type {
  type == "A" => {
    $base = input.value * 2
    $adjusted = $base + 10
    $adjusted
  }
  type == "B" => {
    $base = input.value * 3
    $base - 5
  }
  _ => input.value
}
```

**Rationale**: Match expressions are expressions that return values, not statements that cause side effects. Use match statements (see 6.4) when you need to make output/metadata assignments.

## 6.4 Match Statement

Pattern matching executing multiple assignments:
```bloblang
match input.type() as type {
  type == "object" => {
    output = input.map_each(item -> item.value.apply("transform"))
  }
  type == "array" => {
    output = input.map_each(elem -> elem.apply("transform"))
  }
  _ => {
    output = input
  }
}
```
