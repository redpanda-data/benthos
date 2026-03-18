# 3. Expressions & Statements

## 3.1 Path Expressions

Access nested data: `input.user.email`, `output.result.id`

**Path roots:**
- **Top-level (in assignments):** `input`, `output`, or `$variable` only
- **In expressions within maps/lambdas:** Parameters available as bare identifiers (e.g., `data.field` where `data` is a parameter). Parameters are **read-only** and can only appear in expressions, never as assignment targets.
- **Match with `as`:** Bound variable available as bare identifier in expressions (e.g., `match input as x { x.field ... }`)

**Metadata:** `input@.key`, `output@.key`

**Important:** Bare identifiers (parameters and match bindings) are read-only and can only be used in expressions on the right-hand side. They cannot be assigned to.

**Name resolution:** Every bare identifier in an expression must resolve to a bound name — a map parameter, lambda parameter, match `as` binding, map name, or standard library function name. Namespace-qualified references (`namespace::name`) also resolve to maps from imported modules. An unresolved bare identifier is a **compile-time error**. This catches typos like `inpt.field` (instead of `input.field`) at compile time rather than allowing them to parse and fail later. Resolution priority (innermost wins): parameters > maps > standard library functions. User-defined maps shadow standard library functions of the same name.

**Map and function name references:** When a bare identifier or qualified name resolves to a map or standard library function, it is only valid in two contexts: (1) as a call with parentheses — `double(x)`, `math::double(x)` — or (2) as an argument to a higher-order method — `.map(double)`, `.filter(math::is_positive)`. Using a map or function name as a general-purpose expression (e.g., `output.x = double`, `$fn = uuid_v4`) is a **compile-time error**. See Section 5.5 for details.

**Field names:** Keywords are valid as field names without quoting — `input.map`, `output.if`, `data.match` all work. Use `."quoted"` for fields with special characters, spaces, or names starting with digits:
```bloblang
input.map                    # Valid: keyword as field name
input."field with spaces"    # Quoting needed: spaces
output."special-chars"       # Quoting needed: contains hyphen
data."123"                   # Quoting needed: starts with digit
```

### Indexing

```bloblang
input.items[0]      # Array: first element
input.items[-1]     # Array: last element
input.items[-2]     # Array: second-to-last element
input["field"]      # Object: dynamic field access
input[$var]         # Object: dynamic field access with variable
input.name[0]       # String: first codepoint as int64 (Unicode codepoint value)
input.name[-1]      # String: last codepoint as int64
input.data[0]       # Bytes: first byte as int64 (0-255)
input.data[-1]      # Bytes: last byte as int64
```

**Negative indexing:** For arrays, strings, and bytes, negative indices count from the end: `-1` is last, `-2` is second-to-last, etc. Out-of-bounds negative indices throw errors.

**Semantics:**
- **Objects:** Indexed by string, returns field value (dynamic field access). Non-string indices are an error (no implicit conversion).
- **Arrays:** Indexed by number, returns element at position. The index value must be a whole number — float values like `2.0` are accepted but `1.5` is a runtime error. Non-numeric indices are an error.
- **Strings:** Indexed by number (codepoint position), returns int64 (Unicode codepoint value). The same whole-number requirement applies. Negative indices count from the end. Use `.char()` to convert back to a string.
- **Bytes:** Indexed by number (byte position), returns int64 (byte value 0-255). The same whole-number requirement applies. Negative indices count from the end.
- **All other types** (bool, numeric, null, timestamp): indexing is a runtime error.

**String indexing is codepoint-based, not grapheme-based:**
```bloblang
# Simple characters (1 codepoint each)
"hello"[0]       # 104 (int64: codepoint for 'h')
"café"[3]        # 233 (int64: codepoint for 'é')

# Emoji (1 codepoint)
"😀"[0]          # 128512 (int64: codepoint for 😀)

# Complex graphemes (multiple codepoints)
"👋🏽"[0]         # 128075 (int64: base emoji 👋)
"👋🏽"[1]         # 127995 (int64: skin tone modifier 🏽)

# Family emoji with ZWJ (zero-width joiners)
"👨‍👩‍👧‍👦"[0]    # 128104 (int64: man 👨)
"👨‍👩‍👧‍👦"[1]    # 8205 (int64: zero-width joiner)

# Round-trip: index to codepoint, .char() back to string
"hello"[0].char()  # "h"
"café"[3].char()   # "é"
```

**All string operations are codepoint-based:**
```bloblang
"hello".length()     # 5 (codepoints)
"👋🏽".length()      # 2 (codepoints: base emoji + skin tone modifier)
"café".length()      # 4 (codepoints)
```

**Byte operations are byte-based:**
```bloblang
"hello".bytes()[0]          # 104 (int64: byte value of 'h')
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

**Null-safe method calls:** `?.` also works before method calls. If the receiver is null, the method is not called and `null` is returned. Arguments are not evaluated.
```bloblang
input.value?.uppercase()        # null if value is null (method not called)
input.user?.name?.trim()        # chains null-safe field access and null-safe method call
```

**Note:** `?.`, `?[]`, and `?.method()` only short-circuit on `null` values. Type errors (e.g., accessing a field on a string, or calling a string method on a number) still throw errors:
```bloblang
null?.uppercase()           # null (short-circuited: value is null)
5?.uppercase()              # ERROR: uppercase requires string (value is not null, wrong type)
"hello"?.nonfield?.trim()   # ERROR: cannot access field on string (not null, wrong type)
```

## 3.2 Operators

**Precedence** (high to low):
1. Field access, indexing, method calls: `.`, `?.`, `[]`, `?[]`, `.method()`, `?.method()`
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

**Lambda arrow (`->`):** The `->` token is not a binary operator and does not participate in the precedence hierarchy. A lambda is recognized by its distinct prefix — `identifier ->` or `(params) ->` — which the parser can identify before any precedence comparison. The arrow then consumes the entire right-hand side as the lambda body. For example, `x -> x + 1` parses as `x -> (x + 1)`, and `5 + x -> x * 2` parses as `5 + (x -> x * 2)` (an error, since a lambda is only valid as a method argument).

```bloblang
# Precedence examples
output.calc = input.a + input.b * 2          # * before +
output.check = input.x > 10 && input.y < 20  # > before &&
output.neg = -input.value                    # Unary minus
output.not = !input.flag                     # Logical not

# Precedence trap: method calls bind tighter than unary minus
# -10.string()                               # ERROR: parses as -(10.string()) = -("10")
output.neg_str = (-10).string()              # OK: "-10"

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
output.roll = random_int(1, 6)
```

**Named arguments:** Functions and user maps support named arguments:
```bloblang
# Positional (order matters)
output.result = some_function(arg1, arg2, arg3)

# Named (order doesn't matter)
output.result = some_function(param1: arg1, param2: arg2, param3: arg3)
output.result = some_function(param3: arg3, param1: arg1, param2: arg2)

# Cannot mix positional and named
output.result = some_function(arg1, param2: arg2)  # ERROR

# Duplicate named arguments are a compile-time error
output.result = some_function(param1: arg1, param1: arg2)  # ERROR
```

**Methods** (chained):
```bloblang
output.upper = input.text.uppercase()
output.len = input.items.length()
output.parsed = input.date.ts_parse("%Y-%m-%d")
output.parsed = input.date.ts_parse(format: "%Y-%m-%d")  # Named
```

**Method Chaining:**
```bloblang
output.result = input.text
  .trim()
  .lowercase()
  .replace_all(" ", "-")
```

**Method resolution:** Method names are resolved at compile time against the set of known methods (standard library + implementation extensions). Calling an unknown method is a **compile-time error**. Type compatibility between the receiver and the method is checked at **runtime** (since types are dynamic).

```bloblang
input.value.nonexistent()   # Compile-time error: unknown method
input.value.uppercase()     # OK at compile time; runtime error if value is not a string
```

**Type requirements:** Methods work on specific types. Calling a method on an incompatible type (including null) results in a runtime error. Use null-safe operators to skip methods when values might be null:
```bloblang
input.value?.uppercase()    # Skip method if value is null
input.value.uppercase()     # ERROR if value is null (uppercase requires string)
input.value.type()          # Works on any type including null
```

## 3.4 Lambda Expressions

Lambdas are inline syntax for method arguments — they are not values that can be stored in variables. `$fn = x -> x * 2` is not valid. For reusable transforms, use named maps (Section 5).

Lambda parameters are **read-only** and available as bare identifiers within the lambda body.

**Single parameter:**
```bloblang
input.items.map(item -> item.value * 2)   # 'item' is read-only parameter
input.items.filter(x -> x > 10)
```

**Multiple parameters:**
```bloblang
input.data.map_values(v -> v.uppercase())
```

**Multi-statement body:**
```bloblang
input.items.map(item -> {
  $base = item.price * item.quantity
  $tax = $base * 0.1
  $base + $tax
})
```

Lambda blocks must end with an expression (the return value). Statement-only blocks are invalid.

**Discard parameters (`_`):**
```bloblang
# Discard unused parameters with _
input.data.map_entries((_, v) -> v * 2)            # Discard key, use value
input.items.fold(0, (_, elem) -> elem.value)       # Discard accumulator

# Multiple discards allowed
input.data.map_entries((_, _) -> "constant")
```

`_` as a parameter means "this argument is required by the call signature but unused." It is not bound — referencing `_` in the body is a compile error (it remains a keyword). Multiple `_` parameters are allowed in the same parameter list.

**Default parameters:** Parameters with defaults must come after all required parameters. Default values must be literals (`42`, `"hello"`, `true`, `false`, `null`). Discard parameters (`_`) cannot have defaults. Default parameters follow the same rules as in maps (Section 5.1).

**Parameter shadowing:** Lambda parameter names shadow any map names with the same name within the lambda body. The parameter always wins. Imported namespaces are not affected since they use `::` syntax.
```bloblang
map double(x) { x * 2 }
input.items.map(double -> double * 2)     # double is the parameter, not the map
```

**Purity:** Lambdas cannot assign to `output` or `output@` (no side effects). Because lambda bodies are expression contexts (Section 3.8), any assignment to a variable name from an outer scope creates a new shadow binding in the lambda scope, leaving the outer variable unchanged.

## 3.5 Conditional Expressions

If and match can be used as expressions (returning a value) or as statements (containing assignments). See Section 4 for full semantics including void behavior, match forms, and the expression/statement distinction.

```bloblang
# If expression
output.category = if input.score >= 80 { "high" } else { "low" }

# If expression with else-if
output.tier = if input.score >= 90 { "gold" } else if input.score >= 50 { "silver" } else { "bronze" }

# Match: equality, boolean with 'as', boolean without expression
output.sound = match input.animal { "cat" => "meow", _ => "unknown" }
output.tier = match input.score as s { s >= 100 => "gold", _ => "other" }
output.grade = match { input.score >= 90 => "A", _ => "F" }
```

Conditional expressions cannot assign to `output` or `output@`.

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

Escape sequences: `\\`, `\"`, `\n`, `\t`, `\r`, `\uXXXX` (4-digit Unicode codepoint, BMP only), `\u{X...}` (1–6 hex digit Unicode codepoint, any plane). Examples: `\u0041` for 'A', `\u{1F600}` for '😀', `\u{41}` for 'A'.

Raw strings use backticks. No escape processing — content is used as-is:
```bloblang
`This is a raw string.
It can contain "quotes" without escaping.
Backslashes are literal: C:\path\to\file
Newlines are preserved as-is.`
```

Raw string rules:
- Content between backticks is taken strictly verbatim (no escape sequences, no stripping)
- All characters between the backticks are included, including any leading or trailing newlines
- Cannot contain a literal backtick character (use a regular double-quoted string instead — backticks do not need escaping in regular strings)

**Arrays:** (trailing commas are permitted)
```bloblang
[1, 2, 3]
["a", input.field, uuid_v4()]
```

**Objects:** (trailing commas are permitted)
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

# Dynamic index: auto-creation type determined by index type at runtime
$key = "name"
output.data[$key] = "Alice"    # $key is string → output.data created as object
$idx = 0
output.list[$idx] = "first"    # $idx is int → output.list created as array

# Collision with non-object/non-array value is an error
output.user = "Alice"
output.user.name = "Alice"     # ERROR: output.user is a string, not an object
```

**Array index gaps:** Assigning to an index beyond the current length of an array fills intermediate indices with `null`:
```bloblang
$arr = [10, 20]
$arr[5] = 30                   # [10, 20, null, null, null, 30]
output.items[2] = "x"         # output.items is [null, null, "x"]
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
$user.address = deleted()             # Removes the field from the object inside $user

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

Variable path assignment (`$var.field = expr`) is available in all contexts — both statement contexts (top-level, if/match statement bodies) and expression contexts (if/match expressions, lambda bodies, map bodies). In expression contexts, only variable assignments are allowed (no `output` assignments).

Variables are **block-scoped** with shadowing support (inner blocks can declare new variables with the same name).

**Metadata Assignment:**
```bloblang
output@.kafka_topic = "processed"
output@.kafka_key = input.id
```

**Deletion:**
```bloblang
output.password = deleted()      # Remove field
output = deleted()               # Drop message, exit mapping
```

## 3.8 Variable Scope & Shadowing

Variables are block-scoped with different rules for **statement** and **expression** contexts:

**In expression contexts** (if/match expressions, lambda bodies, map bodies): assigning to a variable name that exists in an outer scope **shadows** it — a new variable is created in the inner scope, and the outer variable is unchanged. This preserves the functional, side-effect-free nature of expressions.

```bloblang
$x = 1

output.result = if true {
  $x = 3            # Shadowing: NEW variable in inner scope
  $x                # 3
}

output.outer = $x   # Still 1 (inner $x doesn't affect outer)
```

**In statement contexts** (if/match statements at top-level): assigning to a variable that exists in an outer scope **modifies** it. New variables declared inside a statement block are **block-scoped** — they are not visible in the outer scope.

```bloblang
$count = 0
if input.flag {
  $count = 1        # Modifies outer $count (already exists)
  $temp = "found"   # Block-scoped: only visible inside this block
}
output.count = $count  # 1 if flag was true, 0 if false
output.temp = $temp    # Compile-time error: $temp does not exist
```

To use a variable after a conditional, pre-declare it:
```bloblang
$temp = null
if input.flag {
  $temp = "found"   # Modifies outer $temp (already exists)
}
output.temp = $temp    # OK: null or "found"
```

**Reassignment at the same scope level:** Assigning to a variable that was declared in the *same* scope is always reassignment (mutation), not shadowing — this applies in both statement and expression contexts. Shadowing only occurs when an inner scope references a variable from an outer scope.
```bloblang
$x = 1
$x = 2              # Reassignment: same variable, now has value 2
output.a = $x       # 2

output.b = if true {
  $a = 1
  $a = 2            # Reassignment within the same expression body (not shadowing)
  $a                 # 2
}
```

**Rationale:** Bloblang is mostly functional, but if/match statements are an intentional imperative escape hatch — they can assign to `output` and modify existing outer variables. New variable declarations are always block-scoped in both statement and expression contexts. The key difference: in statement contexts, assigning to an *existing* outer variable modifies it; in expression contexts, it shadows (creates a new inner variable). Neither context leaks new variables to the outer scope.

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
