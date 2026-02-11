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

### 4.1.1 Array Indexing

Access array elements using square bracket notation with integer indices:

```bloblang
# Positive indexing (0-based)
output.first = input.items[0]           # First element
output.second = input.items[1]          # Second element
output.tenth = input.items[9]           # Tenth element

# Negative indexing (Python-style)
output.last = input.items[-1]           # Last element
output.second_last = input.items[-2]    # Second-to-last element
output.third_last = input.items[-3]     # Third-to-last element
```

**Index Expressions**: The index can be any expression that evaluates to an integer:
```bloblang
output.element = input.items[input.position]
output.dynamic = input.data[$index_var]
output.computed = input.values[input.offset + 1]
```

**Chaining**: Array indexing can be chained with other path operations:
```bloblang
output.user_name = input.users[0].name
output.nested = input.data[2].items[5].value
output.mixed = input.matrix[input.row][$col_var].name.uppercase()
```

**Negative Index Semantics**:
- `-1` accesses the last element
- `-2` accesses the second-to-last element
- `-n` accesses the nth element from the end

For an array of length N:
- Positive index `i` accesses element at position `i` (valid range: `0` to `N-1`)
- Negative index `-i` accesses element at position `N-i` (equivalent to `N-i`)

**Error Behavior**:
- **Out-of-bounds access** (positive or negative) throws a mapping error
- Use `.catch()` to provide fallback values for potentially invalid indices:
  ```bloblang
  output.safe = input.items[10].catch(null)       # Returns null if index out of bounds
  output.last_safe = input.items[-1].catch("empty") # Returns "empty" if array is empty
  ```

**Type Requirements**:
- Target must be an array type (throws error otherwise)
- Index expression must evaluate to an integer (throws error for non-integer or null)

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
