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
