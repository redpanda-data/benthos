# 4. Expressions

## 4.1 Path Expressions

Navigate nested structures using dot notation:
```
input.user.profile.email
output.result.data.id
```

Paths may reference:
- **Input document**: `input.field`
- **Output document**: `output.field`
- **Variables**: `$variable_name`
- **Metadata**: `@metadata_key`

### 4.1.1 Indexing (Arrays, Strings, Bytes)

Access array elements, string characters, or bytes using square bracket notation with integer indices:

```bloblang
# Array indexing (0-based)
output.first = input.items[0]           # First element
output.second = input.items[1]          # Second element
output.tenth = input.items[9]           # Tenth element

# String indexing (byte position, returns single-character string)
output.first_char = input.text[0]       # First character
output.third_char = input.text[2]       # Third character

# Bytes indexing (returns byte value as number 0-255)
output.first_byte = input.data[0]       # First byte as number
output.byte_val = input.data[10]        # Byte at position 10

# Negative indexing (Python-style, works for all types)
output.last = input.items[-1]           # Last array element
output.last_char = input.text[-1]       # Last character of string
output.last_byte = input.data[-1]       # Last byte as number
```

**Index Expressions**: The index can be any expression that evaluates to an integer:
```bloblang
output.element = input.items[input.position]
output.dynamic = input.data[$index_var]
output.computed = input.values[input.offset + 1]
```

**Chaining**: Indexing can be chained with other path operations:
```bloblang
# Array indexing with chaining
output.user_name = input.users[0].name
output.nested = input.data[2].items[5].value
output.mixed = input.matrix[input.row][$col_var].name.uppercase()

# String indexing with chaining
output.first_char_upper = input.text[0].uppercase()
output.initial = input.users[0].name[0]  # First char of first user's name

# Bytes indexing (returns number, can use number methods)
output.first_byte_hex = input.data[0].string()
```

**Negative Index Semantics**:
- `-1` accesses the last element
- `-2` accesses the second-to-last element
- `-n` accesses the nth element from the end

For an array of length N:
- Positive index `i` accesses element at position `i` (valid range: `0` to `N-1`)
- Negative index `-i` accesses element at position `N-i` (equivalent to `N-i`)

**Error Behavior**:
- **Out-of-bounds access** (positive or negative) throws a mapping error for all types
- Use `.catch()` to provide fallback values for potentially invalid indices:
  ```bloblang
  # Arrays
  output.safe = input.items[10].catch(null)         # null if index out of bounds
  output.last_safe = input.items[-1].catch("empty") # "empty" if array is empty

  # Strings
  output.char = input.text[100].catch("")           # "" if string is shorter than 100 bytes
  output.first = input.text[0].catch("N/A")         # "N/A" if string is empty

  # Bytes
  output.byte = input.data[50].catch(0)             # 0 if bytes has fewer than 51 bytes
  ```

**Return Types**:
- **Array indexing**: Returns the element at the specified position (any type)
- **String indexing**: Returns a single-character string at the byte position
- **Bytes indexing**: Returns the byte value as a number (0-255)

**String Indexing Semantics**:
- Indexing is by **byte position**, not character/rune position
- For ASCII text, byte position equals character position
- For UTF-8 multi-byte characters, indexing by byte may split a character
- Returns a single-byte string (may be invalid UTF-8 if it splits a multi-byte character)
- Use `.index(n)` method for safer character-aware indexing if needed

**Type Requirements**:
- Target must be an **array**, **string**, or **bytes** type (throws error otherwise)
- Index expression must evaluate to an **integer** (throws error for non-integer or null)
- Out-of-bounds access throws an error for all types

### 4.1.2 Null-Safe Navigation

The null-safe operators `?.` and `?[` provide concise null handling in path expressions:

```bloblang
# Null-safe field access
output.name = input.user?.name              # null if user is null
output.email = input.user?.profile?.email   # null if any step is null
output.nested = input.a?.b?.c?.d            # null if any field is null

# Null-safe array indexing
output.first = input.items?[0]              # null if items is null
output.last = input.items?[-1]              # null if items is null

# Null-safe string indexing
output.first_char = input.text?[0]          # null if text is null
output.initial = input.user?.name?[0]       # null if user or name is null

# Null-safe bytes indexing
output.first_byte = input.data?[0]          # null if data is null

# Combined null-safe operations
output.user_name = input.users?[0]?.name
output.deep = input.orders?[5]?.items?[0]?.product?.name

# Mixed safe and unsafe navigation
output.value = input.user?.address.city     # Unsafe access to city (will error if address is null)
output.safe = input.user?.address?.city     # Fully null-safe
```

**Semantics**:
- `?.` returns `null` if the left operand is `null` or the field doesn't exist
- `?[` returns `null` if the left operand is `null`
- The entire expression short-circuits to `null` at the first null value
- Null-safe operators only handle `null` values, not errors (use `.catch()` for errors)

**Short-Circuit Behavior**:
```bloblang
# If input.user is null:
input.user?.profile?.email  # Returns null immediately, never evaluates .profile or .email

# If input.user exists but input.user.profile is null:
input.user?.profile?.email  # Returns null after checking profile, never evaluates .email
```

**Null-Safe vs Error Handling**:
```bloblang
# Null-safe: handles null/missing fields
output.safe = input.user?.name              # null if user is null or name is missing

# Error handling: handles operation failures
output.parsed = input.data.parse_json().catch({})  # {} if parse fails

# Combined: handle both null and errors
output.result = input.data?.parse_json().catch({})  # null if data is null, {} if parse fails
```

**Type Errors Still Throw**:
```bloblang
# These still throw errors (not handled by ?.)
input.number?.uppercase()    # Error: can't call uppercase() on number (even though ?. used)
input.number?[0]             # Error: can't index number (only array/string/bytes supported)
input.object?[0]             # Error: can't index object (use .field access instead)
```

**Comparison with `.or()` Method**:
```bloblang
# Using .or() method (only handles null on the final value)
output.name = input.user.name.or("anonymous")  # Errors if user is null

# Using ?. operator (handles null at any step)
output.name = input.user?.name.or("anonymous") # null if user is null, then .or() provides fallback
```

**Best Practices**:
```bloblang
# Use ?. for optional nested navigation
output.city = input.user?.address?.city

# Combine with .or() for defaults
output.city = input.user?.address?.city.or("Unknown")

# Use .catch() for operation errors
output.parsed = input.data?.parse_json().catch({})

# Don't over-use - be explicit about which fields are optional
output.value = input.required.optional?.field  # Clear which is optional
```

## 4.2 Boolean Operators

- `!` - logical NOT
- `>`, `>=`, `==`, `<`, `<=` - comparison (value and type equality)
- `&&` - logical AND
- `||` - logical OR

## 4.3 Arithmetic Operators

- `+` - addition or string concatenation
- `-` - subtraction
- `*` - multiplication
- `/` - division
- `%` - modulo

## 4.4 Functions

Functions generate or retrieve values without a target:
```
uuid_v4()                    # Generate UUID
now()                        # Current timestamp
hostname()                   # System hostname
content()                    # Raw message bytes
env("VAR_NAME")              # Environment variable
random_int(seed, min, max)   # Random integer
deleted()                    # Deletion marker
```

**Syntax**: `function_name()` or `function_name(args...)`

**Arguments**: Positional or named.
```
random_int(timestamp_unix_nano(), 1, 100)
random_int(seed: timestamp_unix_nano(), min: 1, max: 100)
```

## 4.5 Methods

Methods transform target values and support chaining:
```
input.text.uppercase()
input.data.parse_json()
input.items.filter(item -> item.score > 50)
input.name.trim().lowercase().replace_all("_", "-")
```

**Syntax**: `target.method_name()` or `target.method_name(args...)`

**Common Categories**:
- **String**: `.uppercase()`, `.lowercase()`, `.trim()`, `.replace_all()`, `.split()`, `.contains()`
- **Number**: `.floor()`, `.ceil()`, `.round()`, `.abs()`
- **Array**: `.filter()`, `.map_each()`, `.sort()`, `.sort_by()`, `.length()`, `.join()`
- **Object**: `.keys()`, `.values()`, `.map_each()`, `.without()`
- **Parsing**: `.parse_json()`, `.parse_csv()`, `.parse_xml()`, `.parse_yaml()`
- **Encoding**: `.encode_base64()`, `.decode_base64()`, `.format_json()`, `.hash()`
- **Timestamp**: `.ts_parse()`, `.ts_format()`, `.ts_unix()`
- **Type**: `.string()`, `.number()`, `.bool()`, `.type()`, `.not_null()`, `.not_empty()`
- **Error Handling**: `.catch()`, `.or()`

## 4.6 Lambda Expressions

Anonymous functions with **explicit parameter naming** for higher-order methods:
```
input.items.filter(item -> item.score > 50)
input.items.map_each(item -> item.name.uppercase())
input.items.sort_by(item -> item.timestamp)
```

**Syntax**: `parameter -> expression`

**Explicit Naming**: All lambda parameters must be explicitly named. The language has no implicit context variable.

**Parenthesized Context**: Use lambda expressions to capture and name contexts:
```
input.foo.(x -> x.bar + x.baz)  # Explicitly name context as 'x'
```
