# 4. Control Flow

## 4.1 If Expressions vs Statements

**If Expression** (returns value, used in assignment):
```bloblang
output.result = if condition { value } else { other_value }

# Without else: assignment doesn't execute if condition false
output.category = if input.score > 80 { "high" }

# Else-if chains
output.tier = if input.score >= 90 {
  "gold"
} else if input.score >= 50 {
  "silver"
} else {
  "bronze"
}

# Else-if without final else: void if no branch matches
output.tier = if input.score >= 90 { "gold" } else if input.score >= 50 { "silver" }
# void if score < 50 — assignment skipped
```

**If Statement** (standalone, contains output assignments):
```bloblang
if input.type == "user" {
  output.role = "member"
  output.permissions = ["read"]
}

# Else-if statement chains
if input.type == "admin" {
  output.role = "admin"
  output.permissions = ["read", "write", "delete"]
} else if input.type == "mod" {
  output.role = "moderator"
  output.permissions = ["read", "write"]
} else {
  output.role = "user"
  output.permissions = ["read"]
}
```

**Distinction:**
- **Expression:** Used in assignment context, contains pure expressions (no `output`/`output@` assignments)
- **Statement:** Standalone, contains `output`/`output@` assignments, **cannot end with expression** (parse error). Empty statement bodies (e.g., `if condition { }`) are valid and are no-ops.

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
input.items.map(x -> if x > 0 { x * 2 } else { 0 })

# ERROR: Statement body cannot end with expression
if input.flag {
  $x = 10
  $x + 5    # Parse error: trailing expression in statement context
}
```

**If expressions without `else`:** When the condition is false, the expression produces **void** — the absence of a value. No value is produced at all. Void is only meaningful in assignments (where it causes a no-op); in most other contexts it is an error (see summary table below).

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

**Void in collection literals (array and object):** Void is an **error** in collection literals. Use `deleted()` to conditionally omit elements/fields, or add an `else` branch to provide a value in all cases.
```bloblang
# Arrays
output.items = [1, if false { 2 }, 3]                        # ERROR: void in array literal
output.items = [1, if false { 2 } else { deleted() }, 3]     # OK: [1, 3]
output.items = [1, if false { 2 } else { 0 }, 3]             # OK: [1, 0, 3]

# Objects
output.user = {
  "id": input.id,
  "email": if input.verified { input.email }                  # ERROR: void in object literal
}
output.user = {
  "id": input.id,
  "email": if input.verified { input.email } else { deleted() }  # OK: field omitted if not verified
}
```

**Void vs `deleted()`:** These are different concepts. Void means "no value was produced" — nothing happens. `deleted()` is an active deletion marker that removes existing fields and elements (see Section 9.2). Void is only meaningful in assignments (where it causes a no-op); in all other contexts it is an error. The distinction in assignments:
```bloblang
output.status = "pending"
output.status = if false { "override" }  # Void: keeps "pending" (no-op)
output.status = deleted()                # Deleted: removes the field entirely
```

**Void in variable declarations:** A variable declaration (the first assignment to a name in a given scope) **cannot** have a void-producing expression as its right-hand side. If void reaches a variable declaration at runtime, it is a **runtime error**. This ensures every declared variable always has a value — there is no "uninitialized variable" state.
```bloblang
$x = if input.flag { 42 }              # RUNTIME ERROR if input.flag is false (void)
$x = match input.x { "a" => 1 }       # RUNTIME ERROR if no case matches (void)
$x = my_map(input)                     # RUNTIME ERROR if the map produces void

$x = (if input.flag { 42 }).or(0)     # OK: .or() rescues void, always produces a value
$x = if input.flag { 42 } else { 0 }  # OK: else branch ensures a value
```

**Void in variable reassignment:** If a variable already exists and is reassigned a void expression, the assignment is skipped and the variable retains its prior value.
```bloblang
$x = 10
$x = if false { 42 }    # Void: assignment skipped, $x keeps its value
output.result = $x       # 10
```

**Void as a function/map argument:** Passing void as an argument is invalid and causes a runtime error (similar to `deleted()`).
```bloblang
map double(val) { val * 2 }
output.result = double(if false { 42 })  # ERROR: void argument
```

**Void in expression context:** If an operator encounters void as an operand, it causes an error.
```bloblang
output.result = (if false { 42 }) + 1    # ERROR: void in expression
output.flag = !(if false { true })       # ERROR: void in expression
```

**Void as a lambda return value:** Void propagates transparently out of a lambda — the lambda itself does not error. The consuming context then determines what happens:

- **`map`**: void is an error — the lambda must return a value for every element. Use an explicit `else` branch to keep elements unchanged, or return `deleted()` to remove them. Extension methods may also support `deleted()` as a lambda return value.
- **`filter`**: requires a boolean — void is an error.
- Other methods that require a specific type will error if they receive void.

```bloblang
# map: void is an error, must always return a value
input.items.map(x -> if x > 0 { x * 2 } else { x })     # Positive doubled, others kept
input.items.map(x -> if x > 0 { x * 2 })                 # ERROR when x <= 0: void
input.items.map(x -> if x > 0 { x } else { deleted() })  # Non-positive elements removed

# filter requires a boolean: receiving void is an error
input.items.filter(x -> if x > 0 { true })         # ERROR when x <= 0: filter received void, not bool
input.items.filter(x -> if x > 0 { true } else { false })  # OK: always returns bool
```

**Void in match arms:** Match arms are transparent — void produced by a case arm flows out of the match expression and behaves exactly as it would from any other expression. In an assignment context, void causes the assignment to be skipped; in other contexts (collection literals, expressions, etc.) void is an error.
```bloblang
output.result = match input.x {
  "a" => if false { "value" },   # Void: assignment skipped, prior value (if any) preserved
  _ => "default",
}
```

**Sources of void:** Void is produced by an if-expression without a final `else` when no condition is true (including `else if` chains without a final `else`), by a match expression without `_` when no case matches (Section 4.2), and by certain standard library methods when no result exists (e.g., `.find()` when no element matches — see Section 13.6). In all cases, void follows the same rules:

**Summary of void behavior by context:**

| Context | Behavior |
|---------|----------|
| Output field assignment (`output.x = void`) | Assignment skipped; prior value (if any) preserved |
| Variable declaration (`$x = void`) | Runtime error |
| Variable reassignment (`$x = void`, `$x` exists) | Assignment skipped; prior value preserved |
| Collection literal (`[1, void, 3]`) | Error |
| Object literal (`{"a": void}`) | Error |
| Function/map argument (`f(void)`) | Error |
| `map` lambda return | Error (value required) |
| `filter` lambda return | Error (boolean required) |
| Other lambda return | Propagates to consuming context |
| `.catch()` receiver (`void.catch(...)`) | Void passes through (catch not triggered — void is not an error) |
| `.or()` receiver (`void.or(x)`) | Returns `x` (void rescued) |
| Other method call (`void.type()`) | Error |
| Expression operand (`void + 1`) | Error |

**Note:** `.or()` also rescues `deleted()` — see Section 8.3.

## 4.2 Match Expressions vs Statements

**Match Expression** (returns value):
```bloblang
output.sound = match input.animal {
  "cat" => "meow",
  "dog" => "woof",
  _ => "unknown",
}
```

**Exhaustiveness:** Match expressions and statements are **not required** to be exhaustive. If no case matches, the match produces **void** — exactly like an if-expression without `else`. The void behavior follows the same rules as Section 4.1:

- **Match expression** (in assignment): void causes the assignment to be skipped (no-op)
- **Match statement**: no-op (no side effects, execution continues)
- **Match in collection literal**: void is an error (use `_` or `deleted()`)

```bloblang
# Not exhaustive - void if animal is "bird" (assignment skipped)
output.sound = match input.animal {
  "cat" => "meow",
  "dog" => "woof",
}

# Exhaustive - always produces a value
output.sound = match input.animal {
  "cat" => "meow",
  "dog" => "woof",
  _ => "unknown",  # Catch all other values
}
```

**Match Statement** (multiple assignments):
```bloblang
match input.type() {
  "object" => {
    output = input.map_values(v -> transform(v))
  },
  "array" => {
    output = input.map(elem -> transform(elem))
  },
  _ => {
    output = input
  },
}
```

**Parsing disambiguation:** Like `if`, the syntactic context determines statement vs expression form. Match statements are only valid at top-level or inside other statement bodies.

**Case body syntax:** Expression match cases allow either a bare expression or a braced body (`"cat" => "meow"` or `"cat" => { $x = "me"; $x + "ow" }`). Statement match cases always require braces (`"cat" => { output.sound = "meow" }`) because braces are needed to delimit the statement body. Empty statement case bodies (e.g., `"ignore" => { }`) are valid and are no-ops.

### Three Match Forms

**1. Equality match (`match expr { value => ... }`):** The matched expression is evaluated **once**, then each case value is compared against it using equality (`==`). Cases are evaluated in order; the first case that matches is selected and subsequent case expressions are not evaluated. Case expressions are ordinary expressions with the same scope access as the surrounding context (variables, `input`, `output`, etc. as appropriate). If a case expression evaluates to a **boolean**, an error is thrown — this catches the common mistake of writing conditions in equality match instead of using `as`. Use `if`/`else` to match against boolean values directly. Boolean literals (`true`, `false`) as case expressions **must** be rejected at compile time. Cases involving dynamic values that happen to be boolean at runtime are runtime errors.

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

# Boolean case values are an error in equality match:
output.tier = match input.score {
  input.score >= 100 => "gold",  # ERROR: case evaluated to boolean in equality match
}
# Fix: use 'as' for boolean conditions
output.tier = match input.score as s {
  s >= 100 => "gold",
  _ => "other",
}

# Note: this also means you cannot equality-match on boolean values,
# since the case literals true/false are themselves booleans:
output.label = match input.flag {
  true => "yes",    # ERROR: case evaluated to boolean in equality match
  false => "no",
}
# Fix: use if/else for boolean values
output.label = if input.flag { "yes" } else { "no" }
```

**Rationale for the boolean restriction:** In equality match, a case like `input.score >= 100` is almost always a mistake — the user meant to use `as` for boolean conditions, not compare the matched value against `true`/`false`. Rejecting boolean cases catches this common error. The trade-off is that you cannot equality-match on boolean values (`match input.flag { true => ..., false => ... }`). This is intentional: `if`/`else` handles the boolean case more clearly, and multi-way dispatch on a value that could be `true`, `false`, or a non-boolean is better expressed with `match ... as` or `if`/`else if`/`else` chains.

**Compile-time vs runtime detection:** Boolean literals (`true`, `false`) as case expressions are caught at compile time as a convenience — their type is statically known. Dynamic expressions whose type is not known until runtime (e.g., `match x { $var => ... }` where `$var` happens to be boolean) produce the same error at runtime. In both cases, the fix is the same: use `match ... as` for boolean conditions, or `if`/`else` for boolean dispatch. The split in error timing reflects what the compiler can prove statically, not a semantic difference.

**2. Boolean match with `as` (`match expr as x { bool => ... }`):** The matched expression is evaluated **once** and bound to the variable. The `as` binding is available in case conditions, result expressions, and statement bodies (for match statements). It is block-scoped to the match — it cannot be referenced after the match closes. Each case must be a **boolean expression** (evaluated in order, first `true` wins). If a case evaluates to a non-boolean value, an error is thrown. The wildcard `_` is exempt from this requirement — it always matches unconditionally.

```bloblang
output.tier = match input.score as s {
  s >= 100 => "gold",
  s >= 50 => "silver",
  _ => "bronze",
}
```

Use `as` when you need range checks or complex conditions against the matched value.

**3. Boolean match (`match { bool => ... }`):** No matched expression. Each case must be a **boolean expression**. Cases are evaluated in order, and the first one that yields `true` is selected. If a case evaluates to a non-boolean value, an error is thrown. The wildcard `_` is exempt — it always matches unconditionally.

```bloblang
output.category = match {
  input.score >= 90 => "A",
  input.score >= 80 => "B",
  input.score >= 70 => "C",
  _ => "F",
}
```

**Key distinction:** Without `as`, case values are compared by equality against the matched expression (and boolean case values are an error). With `as`, case expressions must be booleans.

**Wildcard `_`:** In all three match forms, `_` is an unconditional catch-all — it always matches regardless of context. In equality match it matches any value; in boolean match forms it is not evaluated as a boolean expression but simply matches unconditionally. `_` is a syntactic form, not an expression — it can only appear as a match case pattern, not in arbitrary expression positions.

**Non-exhaustive match:** If no case matches and there is no `_` catch-all, match *expressions* produce void (see Section 4.1) and match *statements* are no-ops (no assignments are executed). The `_` wildcard is the only catch-all mechanism — there is no `else` keyword for match.

## 4.3 Block-Scoped Variables

```bloblang
output.processed = if input.has_discount {
  $rate = input.discount_rate.or(0.10)
  $base = input.price
  $base * (1 - $rate)
} else {
  input.price
}

output.formatted = match input.currency {
  "USD" => {
    $symbol = "$"
    $amount = input.amount.round(2)
    $symbol + $amount.string()
  },
  "EUR" => {
    $amount = input.amount.round(2)
    $amount.string() + " EUR"
  },
  _ => {
    $amount = input.amount.round(2)
    input.currency + " " + $amount.string()
  },
}
```
