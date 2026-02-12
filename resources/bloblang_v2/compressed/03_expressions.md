# 3. Expressions & Statements

## 3.1 Path Expressions

Access nested data: `input.user.email`, `output.result.id`

**Path roots:** `input`, `output`, `$variable`
**Metadata:** `input@.key`, `output@.key`

**Quoted field names:** Use `."quoted"` for fields with special characters, spaces, or any name. Dot required before quote:
```bloblang
input."field with spaces"
output."special-chars"."nested.field"
user."123"                    # Field name that starts with number
data."any field"              # Can quote any field, not just special ones
```

### Indexing

```bloblang
input.items[0]      # Array: first element
input.items[-1]     # Array: last element (negative indices)
input["field"]      # Object: dynamic field access
input[$var]         # Object: dynamic field access with variable
input.name[0]       # String: first codepoint (→ single-codepoint string)
input.data[0]       # Bytes: first byte as number 0-255
```

**Semantics:**
- **Objects:** Indexed by string, returns field value (dynamic field access)
- **Arrays:** Indexed by number, returns element at position
- **Strings:** Indexed by number (codepoint position), returns single-codepoint string
- **Bytes:** Indexed by number (byte position), returns number (0-255)

**String indexing is codepoint-based, not grapheme-based:**
```bloblang
# Simple characters (1 codepoint each)
"hello"[0]       # "h"
"café"[3]        # "é" (1 codepoint)

# Emoji (1 codepoint)
"😀"[0]          # "😀" (full emoji)

# Complex graphemes (multiple codepoints)
"👋🏽"[0]         # "👋" (base emoji only, without skin tone modifier)
"👋🏽"[1]         # "🏽" (skin tone modifier alone)

# Family emoji with ZWJ (zero-width joiners)
"👨‍👩‍👧‍👦"[0]    # "👨" (man only, not full family)
"👨‍👩‍👧‍👦"[1]    # Zero-width joiner (invisible character)
```

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

**Associativity:**
- **Left-associative:** Arithmetic (`+`, `-`, `*`, `/`, `%`), Logical (`&&`, `||`)
- **Non-associative:** Comparison (`>`, `>=`, `<`, `<=`), Equality (`==`, `!=`)

```bloblang
# Precedence examples
output.calc = input.a + input.b * 2          # * before +
output.check = input.x > 10 && input.y < 20  # > before &&
output.neg = -input.value                    # Unary minus
output.not = !input.flag                     # Logical not

# Associativity examples (left-associative)
output.result = 10 - 5 - 2    # (10 - 5) - 2 = 3
output.result = 20 / 4 / 2    # (20 / 4) / 2 = 2.5
output.result = a && b && c   # (a && b) && c

# Non-associative (must use parentheses)
output.invalid = a < b < c    # ERROR: cannot chain comparisons
output.valid = a < b && b < c # OK: explicit logical combination
```

**Non-associative operator enforcement:** Chaining non-associative operators (e.g., `a < b < c`, `a == b == c`) is a parse error. Implementations must detect and reject such expressions during parsing rather than allowing them to fail at runtime.

## 3.3 Functions & Methods

**Functions** (standalone):
```bloblang
output.id = uuid_v4()
output.time = now()
output.random = random()
```

**Named arguments:** Functions, user maps, and lambdas support named arguments:
```bloblang
# Positional (order matters)
output.result = some_function(arg1, arg2, arg3)

# Named (order doesn't matter)
output.result = some_function(param1: arg1, param2: arg2, param3: arg3)
output.result = some_function(param3: arg3, param1: arg1, param2: arg2)

# Cannot mix positional and named
output.result = some_function(arg1, param2: arg2)  # ERROR
```

**Methods** (chained):
```bloblang
output.upper = input.text.uppercase()
output.len = input.items.length()
output.parsed = input.date.ts_parse("2006-01-02")
output.parsed = input.date.ts_parse(format: "2006-01-02")  # Named
```

**Method Chaining:**
```bloblang
output.result = input.text
  .trim()
  .lowercase()
  .replace_all(" ", "-")
```

**Type requirements:** Methods work on specific types. Calling a method on an incompatible type (including null) results in an error. Use null-safe operators to skip methods when values might be null:
```bloblang
input.value?.uppercase()    # Skip method if value is null
input.value.uppercase()     # ERROR if value is null (uppercase requires string)
input.value.type()          # Works on any type including null
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

# Positional arguments
output.sum = $add(5, 10)
output.doubled = input.items.map_each($double)

# Named arguments
output.sum = $add(a: 5, b: 10)
output.sum = $add(b: 10, a: 5)  # Order doesn't matter with named args
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
  a == "cat" => "meow",
  a == "dog" => "woof",
  a.contains("bird") => "chirp",
  _ => "unknown",
}

# Boolean match (no expression)
output.tier = match {
  input.score >= 100 => "gold",
  input.score >= 50 => "silver",
  _ => "bronze",
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
{"field with spaces": "value"}

# Keys can be expressions (must evaluate to string)
{$key: $value}                              # OK if $key is string, ERROR otherwise
{"prefix_" + input.type: input.value}       # OK: concatenation yields string
{input.field_name: input.field_value}       # OK if field_name is string, ERROR otherwise
{input.count.string(): input.value}         # Explicit conversion to string
```

**Key type requirement:** Object keys must be strings. If a key expression evaluates to a non-string type (number, boolean, null, etc.), a runtime error occurs. Use `.string()` for explicit conversion.

## 3.7 Statements

**Assignment:**
```bloblang
output.field = expression
output.user.id = input.id              # Creates nested structure
output."special.field" = value         # Quoted field (dot required)
output."field with spaces" = value     # Spaces in field name
```

**Variable Declaration:**
```bloblang
$user_id = input.user.id
$name = input.name.uppercase()
```

Variables are **mutable** and can be reassigned:
```bloblang
$count = 0
$count = $count + 1
$count = $count * 2
```

Variables are **block-scoped** with shadowing support (inner blocks can declare new variables with the same name).

**Variable deletion:**
```bloblang
$val = 10
$val = deleted()      # Variable is removed (ceases to exist)
$val                  # ERROR: variable does not exist
```

Assigning `deleted()` to a variable removes it entirely from the current scope.

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

Variables are block-scoped. Inner blocks can declare new variables that shadow outer variables:

```bloblang
$value = 10
output.outer = $value  # 10

output.inner = if input.flag {
  $value = 20          # Shadows outer $value (new variable in this scope)
  $value               # Returns 20
}

output.still_outer = $value  # Still 10 (outer $value unchanged by inner block)
```

**Reassignment vs Shadowing:**
```bloblang
$x = 1
$x = 2              # Reassignment: same variable, now has value 2
output.a = $x       # 2

output.b = if true {
  $x = 3            # Shadowing: NEW variable in inner scope
  $x                # 3
}

output.c = $x       # Still 2 (inner $x doesn't affect outer)
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
