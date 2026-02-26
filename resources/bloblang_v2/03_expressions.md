# 3. Expressions & Statements

## 3.1 Path Expressions

Access nested data: `input.user.email`, `output.result.id`

**Path roots:**
- **Top-level (in assignments):** `input`, `output`, or `$variable` only
- **In expressions within maps/lambdas:** Parameters available as bare identifiers (e.g., `data.field` where `data` is a parameter). Parameters are **read-only** and can only appear in expressions, never as assignment targets.
- **Match with `as`:** Bound variable available as bare identifier in expressions (e.g., `match input as x { x.field ... }`)

**Metadata:** `input@.key`, `output@.key`

**Important:** Bare identifiers (parameters and match bindings) are read-only and can only be used in expressions on the right-hand side. They cannot be assigned to.

**Name resolution:** Every bare identifier in an expression must resolve to a bound name — a map parameter, lambda parameter, or match `as` binding. An unresolved bare identifier is a **compile-time error**. This catches typos like `inpt.field` (instead of `input.field`) at compile time rather than allowing them to parse and fail later.

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
input.items[-1]     # Array: last element
input.items[-2]     # Array: second-to-last element
input["field"]      # Object: dynamic field access
input[$var]         # Object: dynamic field access with variable
input.name[0]       # String: first codepoint as int32 (Unicode codepoint value)
input.name[-1]      # String: last codepoint as int32
input.data[0]       # Bytes: first byte as int32 0-255
input.data[-1]      # Bytes: last byte as int32
```

**Negative indexing:** For arrays, strings, and bytes, negative indices count from the end: `-1` is last, `-2` is second-to-last, etc. Out-of-bounds negative indices throw errors.

**Semantics:**
- **Objects:** Indexed by string, returns field value (dynamic field access)
- **Arrays:** Indexed by number, returns element at position
- **Strings:** Indexed by number (codepoint position), returns int32 (Unicode codepoint value). Negative indices count from the end.
- **Bytes:** Indexed by number (byte position), returns int32 (byte value 0-255). Negative indices count from the end.

**String indexing is codepoint-based, not grapheme-based:**
```bloblang
# Simple characters (1 codepoint each)
"hello"[0]       # 104 (int32: codepoint for 'h')
"café"[3]        # 233 (int32: codepoint for 'é')

# Emoji (1 codepoint)
"😀"[0]          # 128512 (int32: codepoint for 😀)

# Complex graphemes (multiple codepoints)
"👋🏽"[0]         # 128075 (int32: base emoji 👋)
"👋🏽"[1]         # 127995 (int32: skin tone modifier 🏽)

# Family emoji with ZWJ (zero-width joiners)
"👨‍👩‍👧‍👦"[0]    # 128104 (int32: man 👨)
"👨‍👩‍👧‍👦"[1]    # 8205 (int32: zero-width joiner)
```

**All string operations are codepoint-based:**
```bloblang
"hello".length()     # 5 (codepoints)
"👋🏽".length()      # 2 (codepoints: base emoji + skin tone modifier)
"café".length()      # 4 (codepoints)
```

**Byte operations are byte-based:**
```bloblang
"hello".bytes()[0]          # 104 (int32: byte value of 'h')
"hello".bytes().length()    # 5 (bytes)
"👋".bytes().length()       # 4 (UTF-8 encoding uses 4 bytes)
```

Out-of-bounds indexing throws error. Use `.catch(err -> ...)` for safety.

### Null-Safe Navigation

```bloblang
input.user?.address?.city    # null if any part is null
input.items?[0]?.name        # null-safe indexing

# Mix with .or() for defaults
input.contact?.email.or("no-email@example.com")
```

**Note:** `?.` and `?[]` only short-circuit on `null` values. Type errors (e.g., accessing a field on a string) still throw errors.

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

Lambda parameters are **read-only** and available as bare identifiers within the lambda body.

**Single parameter:**
```bloblang
input.items.map_array(item -> item.value * 2)   # 'item' is read-only parameter
input.items.filter(x -> x > 10)
```

**Multiple parameters:**
```bloblang
input.data.map_object((key, value) -> value.uppercase())
```

**Multi-statement body:**
```bloblang
input.items.map_array(item -> {
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
output.doubled = input.items.map_array($double)

# Named arguments
output.sum = $add(a: 5, b: 10)
output.sum = $add(b: 10, a: 5)  # Order doesn't matter with named args
```

**Closure capture:** Lambdas capture variables from their enclosing scope **by reference**. If a closed-over variable is reassigned after the lambda is created, the lambda sees the new value when invoked.
```bloblang
$x = 1
$fn = y -> y + $x
$x = 2
output.result = $fn(10)    # 12: $fn sees the current value of $x (2), not the value at creation (1)
```

**Parameter shadowing:** Lambda parameter names shadow any map names with the same name within the lambda body. The parameter always wins. Imported namespaces are not affected since they use `::` syntax.
```bloblang
map double(x) { x * 2 }
input.items.map_array(double -> double * 2)     # double is the parameter, not the map
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
# Equality match: cases compared against matched value
output.sound = match input.animal {
  "cat" => "meow",
  "dog" => "woof",
  _ => "unknown",
}

# Boolean match with 'as': cases must be boolean expressions
output.tier = match input.score as s {
  s >= 100 => "gold",
  s >= 50 => "silver",
  _ => "bronze",
}

# Boolean match (no expression): cases must be boolean expressions
output.category = match {
  input.score >= 100 => "gold",   # Cases evaluated in order
  input.score >= 50 => "silver",  # First true case wins
  _ => "bronze",
}
```

**Match semantics:** There are three forms: `match expr { ... }` compares each case value by equality against the matched expression. `match expr as x { ... }` binds the matched value to `x` and each case must be a boolean expression. `match { ... }` (no expression) also requires each case to be a boolean expression. In all boolean forms, cases are evaluated in order, first `true` wins, and a non-boolean case throws an error.

**Purity:** Conditional expressions cannot assign to `output` or `output@`.

## 3.6 Literals

**Strings:**

Regular strings use double quotes with backslash escape sequences:
```bloblang
"hello world"
"line one\nline two"       # \n newline
"tab\there"                # \t tab
"quote: \"hi\""            # \" escaped quote
"backslash: \\"            # \\ literal backslash
```

Escape sequences: `\\`, `\"`, `\n`, `\t`, `\r`, `\uXXXX` (Unicode codepoint).

Raw strings use backticks. No escape processing — content is used as-is:
```bloblang
`This is a raw string.
It can contain "quotes" without escaping.
Backslashes are literal: C:\path\to\file
Newlines are preserved as-is.`
```

Raw string rules:
- Content between backticks is taken verbatim (no escape sequences)
- Newlines and whitespace are preserved as-is
- Cannot contain a literal backtick character

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

**Auto-creation of intermediate structures:** Assigning to a nested path automatically creates intermediate objects (and arrays when using index syntax) as needed:
```bloblang
# output starts as {}
output.user.address.city = "London"
# output is now {"user": {"address": {"city": "London"}}}

# Array auto-creation with index syntax
output.items[0].name = "first"
# output.items created as array, output.items[0] created as object

# Collision with non-object/non-array value is an error
output.user = "Alice"
output.user.name = "Alice"     # ERROR: output.user is a string, not an object
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

**Variable path assignment:** Variables support field and index assignment with the same semantics as `output`, including auto-creation of intermediate structures:
```bloblang
$user = {"name": "Alice"}
$user.name = "Bob"                    # Deep mutation: {"name": "Bob"}
$user.address.city = "London"         # Auto-creates intermediates
$user.tags[0] = "admin"              # Index assignment
$user.address = deleted()             # Removes the field

$val = "hello"
$val.field = "x"                      # ERROR: cannot assign field on string
```

Assigning a value to a variable always creates a logical copy, regardless of source (`input`, `output`, or another variable). Mutations to the variable never affect the original, and vice versa:
```bloblang
$data = input.record
$data.status = "processed"            # Mutates $data only; input unchanged

$snap = output.user
output.user.name = "changed"          # $snap unaffected
```

Variable path assignment is only available in statement contexts (top-level and if/match statement bodies). In expression contexts, only whole-variable assignment (`$var = expr`) is allowed.

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

Variables are block-scoped with different rules for **statement** and **expression** contexts:

**In expression contexts** (if/match expressions, lambdas, map bodies): assigning to a variable name that exists in an outer scope **shadows** it — a new variable is created in the inner scope, and the outer variable is unchanged. This preserves the functional, side-effect-free nature of expressions.

```bloblang
$x = 1

output.result = if true {
  $x = 3            # Shadowing: NEW variable in inner scope
  $x                # 3
}

output.outer = $x   # Still 1 (inner $x doesn't affect outer)
```

**In statement contexts** (if/match statements at top-level): variables are **not block-scoped**. Assigning to an existing outer variable modifies it, and new variables declared inside the block are visible in the outer scope after the block executes.

```bloblang
$count = 0
if input.flag {
  $count = 1        # Modifies outer $count
  $temp = "found"   # New variable, visible in outer scope
}
output.count = $count  # 1 if flag was true, 0 if false
output.temp = $temp    # "found" if flag was true, ERROR if false ($temp never created)
```

**Reassignment at the same scope level:**
```bloblang
$x = 1
$x = 2              # Reassignment: same variable, now has value 2
output.a = $x       # 2
```

**Rationale:** Bloblang is mostly functional, but if/match statements are an intentional imperative escape hatch — they can assign to `output`, modify outer variables, and introduce new variables into the outer scope. Expressions remain pure: no `output` assignments, no outer variable mutation, and variables are block-scoped with shadowing.

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
