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
input.items.map_array(x -> if x > 0 { x * 2 } else { 0 })

# ERROR: Statement body cannot end with expression
if input.flag {
  $x = 10
  $x + 5    # Parse error: trailing expression in statement context
}
```

**If expressions without `else`:** When the condition is false, the expression produces **void** — the absence of a value. No value is produced at all, and the surrounding context determines what happens:

**Void in assignments:** The assignment does not execute. The target field is neither created nor modified.

**Case 1: No prior assignment**
```bloblang
output.category = if input.score > 80 { "high" }
# If score <= 80: void, assignment skipped, field doesn't exist
# Reading output.category returns null (field is absent)
# JSON output: field omitted entirely
```

**Case 2: Has prior assignment**
```bloblang
output.status = "pending"
output.status = if false { "override" }  # Void: assignment skipped
# output.status keeps its existing value: "pending"
# Reading output.status returns "pending" (not null!)
# JSON output: {"status": "pending"}
```

**Case 3: Explicit null vs non-existent**
```bloblang
output.field1 = null                    # Field exists with null value
output.field2 = if false { "value" }    # Void: field doesn't exist (no prior assignment)
# field1 reads as null, field2 reads as null - but differ structurally
# JSON output: {"field1": null} (field2 omitted)
```

**Void in array literals:** No value was produced, so there is nothing to include — the element position is skipped.
```bloblang
output.items = [1, if false { 2 }, 3]   # Result: [1, 3]
```

**Void in object literals:** No value was produced for the field, so the field is omitted.
```bloblang
output.user = {
  "id": input.id,
  "email": if input.verified { input.email }  # Void if not verified: field omitted
}
# If not verified: {"id": ...} (no email field)
```

**Void vs `deleted()`:** Both cause elements to be absent from collections and fields to be omitted from objects, but they are different concepts. Void means "no value was produced" — nothing happens. `deleted()` is an active deletion marker that removes existing fields (see Section 9.2). The distinction matters in assignments:
```bloblang
output.status = "pending"
output.status = if false { "override" }  # Void: keeps "pending" (no-op)
output.status = deleted()                # Deleted: removes the field entirely
```

**Void in variable declarations:** The variable is not created. Subsequent references to it error as if the variable were never defined.
```bloblang
$x = if false { 42 }    # Void: $x is not created
output.y = $x            # ERROR: variable $x does not exist
```

**Void as a function/map argument:** Passing void as an argument is invalid and causes a mapping error (similar to `deleted()`).
```bloblang
map double(val) { val * 2 }
output.result = double(if false { 42 })  # ERROR: void argument
```

**Void in expression context:** If an operator encounters void as an operand, it causes an error.
```bloblang
output.result = (if false { 42 }) + 1    # ERROR: void in expression
output.flag = !(if false { true })       # ERROR: void in expression
```

**Summary of void behavior by context:**

| Context | Behavior |
|---------|----------|
| Output field assignment (`output.x = void`) | Assignment skipped; prior value (if any) preserved |
| Variable declaration (`$x = void`) | Variable not created; references error |
| Collection literal (`[1, void, 3]`) | Element skipped (`[1, 3]`) |
| Object literal (`{"a": void}`) | Field omitted (`{}`) |
| Function/map argument (`f(void)`) | Error |
| Expression operand (`void + 1`) | Error |

## 4.2 Match Expressions vs Statements

**Match Expression** (returns value):
```bloblang
output.sound = match input.animal as a {
  a == "cat" => "meow",
  a == "dog" => "woof",
  _ => "unknown",
}
```

**Exhaustiveness:** Match expressions and statements are **not required** to be exhaustive. If no case matches at runtime, the mapping **throws an error**. Use `_` as a catch-all to handle unexpected values:

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
    output = input.map_object((key, value) -> transform(value))
  },
  t == "array" => {
    output = input.map_array(elem -> transform(elem))
  },
  _ => {
    output = input
  },
}
```

**Parsing disambiguation:** Like `if`, the syntactic context determines statement vs expression form. Match statements are only valid at top-level or inside other statement bodies.

### Three Match Forms

**1. Equality match (`match expr { value => ... }`):** The matched expression is evaluated **once**, then each case value is compared against it using equality (`==`). The first case that matches is selected.

```bloblang
output.sound = match input.animal {
  "cat" => "meow",
  "dog" => "woof",
  _ => "unknown",
}

# Equivalent to:
output.sound = match input.animal as a {
  a == "cat" => "meow",
  a == "dog" => "woof",
  _ => "unknown",
}
```

**2. Boolean match with `as` (`match expr as x { bool => ... }`):** The matched expression is evaluated **once** and bound to the variable. Each case must be a **boolean expression** (evaluated in order, first `true` wins). If a case evaluates to a non-boolean value, an error is thrown.

```bloblang
output.tier = match input.score as s {
  s >= 100 => "gold",
  s >= 50 => "silver",
  _ => "bronze",
}
```

Use `as` when you need range checks or complex conditions against the matched value.

**3. Boolean match (`match { bool => ... }`):** No matched expression. Each case must be a **boolean expression**. Cases are evaluated in order, and the first one that yields `true` is selected. If a case evaluates to a non-boolean value, an error is thrown.

```bloblang
output.category = match {
  input.score >= 90 => "A",
  input.score >= 80 => "B",
  input.score >= 70 => "C",
  _ => "F",
}
```

**Key distinction:** Without `as`, case values are compared by equality against the matched expression. With `as`, case expressions must be booleans.

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
