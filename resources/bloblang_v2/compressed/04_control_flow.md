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

# ERROR: Statement body cannot end with expression
if input.flag {
  $x = 10
  $x + 5    # Parse error: trailing expression in statement context
}
```

**If expressions without `else`:** When the condition is false, the behavior depends on context:

**In assignments:** The assignment does not execute. The target field is neither created nor modified.

**Case 1: No prior assignment**
```bloblang
output.category = if input.score > 80 { "high" }
# If score <= 80: assignment skipped, field doesn't exist
# Reading output.category returns null (field is absent)
# JSON output: field omitted entirely
```

**Case 2: Has prior assignment**
```bloblang
output.status = "pending"
output.status = if false { "override" }  # Assignment skipped
# output.status keeps its existing value: "pending"
# Reading output.status returns "pending" (not null!)
# JSON output: {"status": "pending"}
```

**Case 3: Explicit null vs non-existent**
```bloblang
output.field1 = null                    # Field exists with null value
output.field2 = if false { "value" }    # Field doesn't exist (no prior assignment)
# field1 reads as null, field2 reads as null - but differ structurally
# JSON output: {"field1": null} (field2 omitted)
```

**In array literals:** Elements are skipped (same as `deleted()`).
```bloblang
output.items = [1, if false { 2 }, 3]   # Result: [1, 3]
# Equivalent to: [1, if false { 2 } else { deleted() }, 3]
```

**In object literals:** Fields are omitted (same as `deleted()`).
```bloblang
output.user = {
  "id": input.id,
  "email": if input.verified { input.email }  # Omitted if not verified
}
# If not verified: {"id": ...} (no email field)
```

**Key distinction:** An if-without-else that evaluates to false skips the assignment entirely:
- **No prior value:** Field doesn't exist (reads as `null`, omitted from JSON)
- **Has prior value:** Field keeps its existing value (reads as that value, included in JSON)
- **Explicit null:** Field exists with `null` value (reads as `null`, included in JSON as `"field": null`)

## 4.2 Match Expressions vs Statements

**Match Expression** (returns value):
```bloblang
output.sound = match input.animal as a {
  a == "cat" => "meow",
  a == "dog" => "woof",
  _ => "unknown",
}
```

**Exhaustiveness:** Match expressions are **not required** to be exhaustive. If no case matches at runtime, the mapping **throws an error**. Use `_` as a catch-all to handle unexpected values:

```bloblang
# Not exhaustive - will error if animal is "bird"
output.sound = match input.animal {
  "cat" => "meow",
  "dog" => "woof",
}

# Exhaustive - always matches
output.sound = match input.animal {
  "cat" => "meow",
  "dog" => "woof",
  _ => "unknown",  # Catch all other values
}
```

**Match Statement** (multiple assignments):
```bloblang
match input.type() as t {
  t == "object" => {
    output = input.map_each(item -> transform(item.value))
  },
  t == "array" => {
    output = input.map_each(elem -> transform(elem))
  },
  _ => {
    output = input
  },
}
```

**Parsing disambiguation:** Like `if`, the syntactic context determines statement vs expression form. Match statements are only valid at top-level or inside other statement bodies.

**Context binding with `as`** is optional. When omitted, case expressions reference the original matched expression directly:

```bloblang
# Without 'as' (repeat expression)
output.tier = match input.score {
  input.score >= 100 => "gold",
  input.score >= 50 => "silver",
  _ => "bronze",
}

# With 'as' (bind to variable)
output.tier = match input.score as s {
  s >= 100 => "gold",
  s >= 50 => "silver",
  _ => "bronze",
}
```

Use `as` when the matched expression is complex or used multiple times in cases.

**Evaluation semantics:** The matched expression is evaluated **once** before testing any cases, regardless of whether `as` is used. The value is then compared against each case condition in order. Using `as` simply binds the evaluated value to a variable for cleaner syntax, but does not change evaluation behavior.

**Boolean match (no expression):** When `match` is used without an expression, each case must be a boolean expression. Cases are evaluated in order, and the first one that yields `true` is selected. If a case evaluates to a non-boolean value, an error is thrown.

```bloblang
# Boolean match
output.category = match {
  input.score >= 90 => "A",
  input.score >= 80 => "B",
  input.score >= 70 => "C",
  _ => "F",
}

# ERROR: non-boolean case
output.bad = match {
  "hello" => "result",  # ERROR: "hello" is string, not boolean
  _ => "default",
}
```

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
  },
  _ => {
    $amount = input.amount.round(2)
    c + " " + $amount.string()
  },
}
```
