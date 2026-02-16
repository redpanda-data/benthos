# 2. Type System & Coercion

Bloblang V2 is **dynamically typed** - types determined at runtime.

## 2.1 Runtime Types

| Type | Description | Examples |
|------|-------------|----------|
| `string` | UTF-8 text (operations are codepoint-based) | `"hello"`, `""` |
| `int32` | 32-bit signed integer | `42`, `-10` |
| `int64` | 64-bit signed integer | `42`, `-10` |
| `uint32` | 32-bit unsigned integer | `42`, `255` |
| `uint64` | 64-bit unsigned integer | `42`, `1000` |
| `float32` | 32-bit IEEE 754 float | `3.14`, `-10.5` |
| `float64` | 64-bit IEEE 754 float | `3.14`, `-10.5` |
| `bool` | Boolean | `true`, `false` |
| `null` | Null value | `null` |
| `bytes` | Byte array (operations are byte-based) | `"hello".bytes()` |
| `array` | Ordered collection | `[1, "two", true]` |
| `object` | Key-value map | `{"key": "value"}` |
| `lambda` | Function value | `x -> x * 2` |

**Important:** String operations (indexing, `.length()`, etc.) work on **Unicode codepoints**, not grapheme clusters. This means complex emoji and combining characters may span multiple codepoints. Byte operations work on individual bytes in the UTF-8 encoding.

## 2.2 Type Introspection

```bloblang
output.type = input.value.type()  # Returns type name as string

# Type checking
output.is_array = input.items.type() == "array"
output.is_null = input.maybe.type() == "null"
```

## 2.3 Type Coercion

**The `+` Operator:**
- Requires **same types**: both strings or both numeric types
- **No implicit conversion**
- String concatenation and numeric addition are distinguished by operand types

```bloblang
# ✅ Valid
output.sum = 5 + 3              # 8 (int64)
output.concat = "hello" + " world"  # "hello world" (string)

# ❌ Error: Type mismatch
output.bad = 5 + "3"            # ERROR

# ✅ Explicit conversion required
output.ok = 5.string() + "3"    # "53" (string)
output.ok2 = 5 + "3".int64()    # 8    (int64)
```

**Other Operators:**
- Arithmetic (`-`, `*`, `/`, `%`): Require numeric types (null errors)
- Comparison (`>`, `<`, `>=`, `<=`): Require comparable same types (null errors)
- Equality (`==`, `!=`): Compare both type and value (see below)
- Logical (`&&`, `||`): Require booleans

**Equality Semantics:**

Both type and value must match for equality to return `true`. Different types always return `false` (not an error). This means `int32(5)` is not equal to `int64(5)` or `float64(5)`:

```bloblang
# Different types: always false
5 == "5"             # false (int64 vs string)
5 == 5.0             # false (int64 vs float64)
int32(5) == int64(5) # false (int32 vs int64)
true == 1            # false (bool vs int64)
null == 0            # false (null vs int64)

# Same type, same value: true
5 == 5               # true
5.0 == 5.0           # true (both are float64)
"hello" == "hello"   # true
true == true         # true
null == null         # true

# Same type, different value: false
5 == 10              # false
"a" == "b"           # false

# Collections: structural equality (value-based)
[1, 2] == [1, 2]     # true (same contents)
[1, 2] == [2, 1]     # false (different order)
{"a": 1} == {"a": 1} # true (same structure and values)
```

## 2.4 Null Handling

**Null errors immediately in most operations:**
```bloblang
# ❌ Errors
null + 5             # ERROR: arithmetic requires numbers
null.uppercase()     # ERROR: method doesn't support null
null > 5             # ERROR: ordering requires comparable types

# ✅ Equality comparisons work with null
null == null         # true (same type and value)
null != null         # false
null == 5            # false (different types: null vs int64)
null != 5            # true

# ✅ Null-safe navigation prevents errors
input.user?.name           # null if user is null (no error)
input.items?[0]            # null if items is null (no error)

# ✅ .or() provides defaults for null values
input.value.or("default")  # "default" if value is null
```

## 2.5 Type Conversions

Explicit conversion methods:
- `.string()` - Convert to string
- `.int32()` - Convert to int32
- `.int64()` - Convert to int64
- `.uint32()` - Convert to uint32
- `.uint64()` - Convert to uint64
- `.float32()` - Convert to float32
- `.float64()` - Convert to float64
- `.bool()` - Convert to boolean
- `.bytes()` - Convert to byte array
- `.type()` - Get type name

```bloblang
output.str = input.count.string()        # "42"
output.i32 = "42".int32()               # 42 (int32)
output.i64 = "42".int64()               # 42 (int64)
output.u32 = "255".uint32()             # 255 (uint32)
output.u64 = "1000".uint64()            # 1000 (uint64)
output.f32 = "3.14".float32()           # 3.14 (float32)
output.f64 = "3.14".float64()           # 3.14 (float64)
output.bool = "true".bool()             # true
output.bytes = "hello".bytes()          # byte array
```

**Type coercion in arithmetic:** Mixed numeric types in arithmetic operations require explicit conversion. The result type follows the dominant type in the operation:

```bloblang
# Explicit conversion required for mixed types
output.sum = 5.int64() + 10            # int64 + int64 = int64
output.sum = 5.float64() + 10.0         # float64 + float64 = float64
output.sum = 5 + 10.0                   # ERROR: mixed types
