# 3. Expressions & Statements

## 3.1 Path Expressions

Access nested data: `input.user.email`, `output.result.id`

**Path roots:** `input`, `output`, `$variable`
**Metadata:** `input@.key`, `output@.key`

### Indexing

```bloblang
input.items[0]      # First element
input.items[-1]     # Last element (negative indices)
input.name[0]       # First character/rune (strings → single-char string)
input.data[0]       # First byte as number 0-255 (bytes)
```

**Semantics:**
- **Strings:** Indexed by rune position (UTF-8 character), returns single-character string
- **Bytes:** Indexed by byte position, returns number (0-255)
- **Arrays:** Indexed by element position

Out-of-bounds throws error. Use `.catch()` for safety.

### Null-Safe Navigation

```bloblang
input.user?.address?.city    # null if any part is null
input.items?[0]?.name        # null-safe indexing

# Mix with .or() for defaults
input.contact?.email.or("no-email@example.com")
```

## 3.2 Operators

**Precedence** (high to low):
1. Field access, indexing: `.`, `?.`, `[]`, `?[]`
2. Unary: `!`, `-`
3. Multiplicative: `*`, `/`, `%`
4. Additive: `+`, `-`
5. Comparison: `>`, `>=`, `<`, `<=`
6. Equality: `==`, `!=`
7. Logical AND: `&&`
8. Logical OR: `||`

```bloblang
# Examples
output.calc = input.a + input.b * 2          # * before +
output.check = input.x > 10 && input.y < 20  # > before &&
output.neg = -input.value                    # Unary minus
output.not = !input.flag                     # Logical not
```

## 3.3 Functions & Methods

**Functions** (standalone):
```bloblang
output.id = uuid_v4()
output.time = now()
output.random = random()
```

**Methods** (chained):
```bloblang
output.upper = input.text.uppercase()
output.len = input.items.length()
output.parsed = input.date.ts_parse("2006-01-02")
```

**Method Chaining:**
```bloblang
output.result = input.text
  .trim()
  .lowercase()
  .replace_all(" ", "-")
```

## 3.4 Lambda Expressions

**Single parameter:**
```bloblang
input.items.map_each(item -> item.value * 2)
input.items.filter(x -> x > 10)
```

**Multiple parameters:**
```bloblang
input.items.reduce((acc, item) -> acc + item.price, 0)
input.scores.reduce((sum, score, index) -> sum + (score * index), 0)
```

**Multi-statement body:**
```bloblang
input.items.map_each(item -> {
  $base = item.price * item.quantity
  $tax = $base * 0.1
  $base + $tax
})
```

Lambda blocks must end with an expression (the return value). Statement-only blocks are invalid.

**First-class (stored in variables):**
```bloblang
$add = (a, b) -> a + b
$double = x -> x * 2

output.sum = $add(5, 10)
output.doubled = input.items.map_each($double)
```

**Purity:** Lambdas cannot assign to `output` or `output@` (no side effects).

## 3.5 Conditional Expressions

**If Expression:**
```bloblang
output.category = if input.score >= 80 {
  "high"
} else if input.score >= 50 {
  "medium"
} else {
  "low"
}

# With block-scoped variables
output.age = if input.birthdate != null {
  $parsed = input.birthdate.ts_parse("2006-01-02")
  $now = now()
  ($now.ts_unix() - $parsed.ts_unix()) / 31536000
} else {
  null
}
```

**Match Expression:**
```bloblang
output.sound = match input.animal as a {
  a == "cat" => "meow"
  a == "dog" => "woof"
  a.contains("bird") => "chirp"
  _ => "unknown"
}

# Boolean match (no expression)
output.tier = match {
  input.score >= 100 => "gold"
  input.score >= 50 => "silver"
  _ => "bronze"
}
```

**Purity:** Conditional expressions cannot assign to `output` or `output@`.

## 3.6 Literals

**Arrays:**
```bloblang
[1, 2, 3]
["a", input.field, uuid_v4()]
```

**Objects:**
```bloblang
{"name": "Alice", "age": 30}
{"id": input.id, "timestamp": now()}
```

## 3.7 Statements

**Assignment:**
```bloblang
output.field = expression
output.user.id = input.id              # Creates nested structure
output."special.field" = value         # Quoted field names
```

**Variable Declaration:**
```bloblang
$user_id = input.user.id
$name = input.name.uppercase()
```

Variables are **immutable** (cannot reassign in same scope).
Variables are **block-scoped** with shadowing support.

**Metadata Assignment:**
```bloblang
output@.kafka_topic = "processed"
output@.kafka_key = input.id
```

**Deletion:**
```bloblang
output.password = deleted()      # Remove field
output = deleted()               # Filter entire message
```

## 3.8 Variable Scope & Shadowing

```bloblang
$value = 10
output.outer = $value  # 10

output.inner = if input.flag {
  $value = 20          # Shadows outer $value
  $value               # Returns 20
}

output.still_outer = $value  # Still 10 (outer unchanged)
```

Variables declared in blocks are only accessible within that block and nested blocks.

## 3.9 Statements vs Expressions

**Statements** (cause side effects):
- Assignments: `output.field = value`, `output@.key = value`
- Variable declarations: `$var = value`
- If/match statements (with multiple assignments)

**Expressions** (return values):
- All operators, function calls, method chains
- If/match expressions (return single value)
- Lambdas

**Rule:** Expressions cannot contain assignments to `output` or `output@`.
