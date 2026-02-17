# 2. Type System & Coercion

Bloblang V2 is **dynamically typed** - types determined at runtime.

## 2.1 Runtime Types

| Type | Description | Examples |
|------|-------------|----------|
| `string` | UTF-8 text (operations are codepoint-based) | `"hello"`, `""` |
| `int32` | 32-bit signed integer | `42.int32()` |
| `int64` | 64-bit signed integer (default for integer literals) | `42`, `-10` |
| `uint32` | 32-bit unsigned integer | `42.uint32()` |
| `uint64` | 64-bit unsigned integer | `42.uint64()` |
| `float32` | 32-bit IEEE 754 float | `3.14.float32()` |
| `float64` | 64-bit IEEE 754 float (default for float literals) | `3.14`, `-10.5` |
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
- Both strings: string concatenation
- Both numeric: addition (with promotion, see below)
- String + number (or any other cross-family mix): **error**

```bloblang
output.sum = 5 + 3                  # 8 (int64)
output.concat = "hello" + " world"  # "hello world" (string)
output.bad = 5 + "3"                # ERROR: cannot add int64 and string
output.ok = 5.string() + "3"        # "53" (explicit conversion)
```

**Other Operators:**
- Arithmetic (`-`, `*`, `/`, `%`): Require numeric types (null errors), with promotion
- Comparison (`>`, `<`, `>=`, `<=`): Require comparable same types (null errors)
- Equality (`==`, `!=`): Compare both type and value (see below)
- Logical (`&&`, `||`): Require booleans

### Numeric Type Promotion

When arithmetic operators are applied to operands of different numeric types, both operands are promoted to a common type before the operation. Non-numeric types (string, bool, null, etc.) are never promoted — mixing them with numbers is always an error.

**Promotion rules (applied in order):**

| Operand types | Promoted to | Rationale |
|---------------|-------------|-----------|
| Same type | No promotion | `int64 + int64 → int64` |
| Same signedness, different width | Wider type | `int32 + int64 → int64`, `float32 + float64 → float64` |
| Signed + unsigned integer | int64 | `int32 + uint32 → int64`, `int64 + uint64 → int64` |
| Any integer + any float | float64 | `int64 + float32 → float64`, `uint32 + float64 → float64` |

**Division always produces a float:**

| Operand types | Result type |
|---------------|-------------|
| float32 / float32 | float32 |
| All other combinations | float64 |

```bloblang
# Same type: no promotion
output.a = 5 + 3                    # 8 (int64)
output.b = 5.0 + 3.0                # 8.0 (float64)

# Different width: promote to wider
output.c = 5.int32() + 10           # 15 (int64: int32 promoted to int64)

# Signed + unsigned: promote to int64
output.d = 5.int32() + 10.uint32()  # 15 (int64)

# Integer + float: promote to float64
output.e = 5 + 3.0                  # 8.0 (float64)

# Division: always float
output.f = 7 / 2                    # 3.5 (float64)
output.g = 20 / 4 / 2               # 2.5 (float64)
output.h = 10.0 / 3.0               # 3.333... (float64)
```

**Note:** Promoting int64 or uint64 to float64 may lose precision for values larger than 2^53. Use explicit conversion if exact large-integer arithmetic is required.

**Equality Semantics:**

Both type and value must match for equality to return `true`. Different types always return `false` (not an error). This means `5.int32()` is not equal to `5` (int64) or `5.float64()`:

```bloblang
# Different types: always false
5 == "5"                   # false (int64 vs string)
5 == 5.0                   # false (int64 vs float64)
5.int32() == 5             # false (int32 vs int64)
5.int32() == 5.float64()   # false (int32 vs float64)
true == 1                  # false (bool vs int64)
null == 0                  # false (null vs int64)

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

**Required** conversion methods (these are the only way to create non-default numeric types since literals are always int64 or float64):
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

**Type promotion in arithmetic:** Mixed numeric types are automatically promoted (see Section 2.3). Non-numeric types always require explicit conversion:

```bloblang
output.sum = 5 + 10                     # int64 + int64 = int64
output.sum = 5 + 10.0                   # int64 + float64 = float64 (promoted)
output.sum = 5 + "10"                   # ERROR: cannot add int64 and string
output.sum = 5 + "10".int64()           # int64 + int64 = int64 (explicit conversion)
