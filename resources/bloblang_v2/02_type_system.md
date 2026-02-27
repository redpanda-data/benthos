# 2. Type System & Coercion

Bloblang V2 is **dynamically typed** - types determined at runtime.

## 2.1 Runtime Types

| Type | Description | Examples |
|------|-------------|----------|
| `string` | UTF-8 text (operations are codepoint-based) | `"hello"`, `""` |
| `int32` | 32-bit signed integer | `42.int32()` |
| `int64` | 64-bit signed integer (default for integer literals) | `42`, `-10` (unary minus) |
| `uint32` | 32-bit unsigned integer | `42.uint32()` |
| `uint64` | 64-bit unsigned integer | `42.uint64()` |
| `float32` | 32-bit IEEE 754 float | `3.14.float32()` |
| `float64` | 64-bit IEEE 754 float (default for float literals) | `3.14`, `-10.5` (unary minus) |
| `bool` | Boolean | `true`, `false` |
| `null` | Null value | `null` |
| `bytes` | Byte array (operations are byte-based) | `"hello".bytes()` |
| `array` | Ordered collection | `[1, "two", true]` |
| `object` | Key-value map | `{"key": "value"}` |
| `lambda` | Function value | `x -> x * 2` |

**Important:** String operations (indexing, `.length()`, etc.) work on **Unicode codepoints**, not grapheme clusters. This means complex emoji and combining characters may span multiple codepoints. Byte operations work on individual bytes in the UTF-8 encoding.

**Void:** Void is not a runtime type — it is the absence of a value, produced when an if-expression without `else` has a false condition (Section 4.1). Void cannot be stored in variables, passed as arguments, or used in expressions (all are errors). It only exists transiently to signal "no value was produced," and the surrounding context determines what happens (assignment skipped, collection element omitted, etc.). Since void can never be the receiver of a method call, `.type()` on void is not possible. See Section 4.1 for full void semantics.

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
- Both bytes: byte concatenation
- Both numeric: addition (with promotion, see below)
- Any cross-family mix (string + number, bytes + string, etc.): **error**

```bloblang
output.sum = 5 + 3                  # 8 (int64)
output.concat = "hello" + " world"  # "hello world" (string)
output.joined = "ab".bytes() + "cd".bytes()  # byte concatenation
output.bad = 5 + "3"                # ERROR: cannot add int64 and string
output.bad2 = "hello" + "world".bytes()  # ERROR: cannot add string and bytes
output.ok = 5.string() + "3"        # "53" (explicit conversion)
```

**Other Operators:**
- Arithmetic (`-`, `*`, `/`, `%`): Require numeric types (null errors), with promotion
- Comparison (`>`, `<`, `>=`, `<=`): Require comparable same types (null errors), with numeric promotion
- Equality (`==`, `!=`): Numeric types use promotion then compare by value; non-numeric types require same type and value; cross-family is always `false` (see below)
- Logical (`&&`, `||`): Require booleans

### Numeric Type Promotion

When arithmetic, comparison, or equality operators are applied to operands of different numeric types, both operands are promoted to a common type before the operation. Non-numeric types (string, bool, null, etc.) are never promoted — mixing them with numbers is always an error (for arithmetic/comparison) or always `false` (for equality).

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

**Modulo follows standard promotion rules** (not the division rule). The result type is determined by the promoted operand type. For float operands, modulo uses `fmod` semantics (IEEE 754 remainder):

| Operand types | Result type | Example |
|---------------|-------------|---------|
| int64 % int64 | int64 | `7 % 2 → 1` |
| int32 % int64 | int64 | Promoted to int64 |
| Any integer % any float | float64 | `7 % 2.0 → 1.0` |
| float64 % float64 | float64 | `7.5 % 2.0 → 1.5` |

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

# Modulo: follows standard promotion (not the division rule)
output.i = 7 % 2                    # 1 (int64)
output.j = 7.0 % 2.0                # 1.0 (float64, fmod)
output.k = 7.5 % 2.0                # 1.5 (float64, fmod)
output.l = 7 % 2.0                  # 1.0 (float64: int64 promoted to float64, fmod)

# Division by zero: always an error
output.bad = 7 / 0                  # ERROR: division by zero
output.bad = 7.0 / 0.0              # ERROR: division by zero

# Modulo by zero: always an error
output.bad = 7 % 0                  # ERROR: modulo by zero
```

**Note:** Promoting uint64 to int64 may overflow for values larger than 2^63-1 (the maximum int64 value). Promoting int64 or uint64 to float64 may lose precision for values larger than 2^53. Use explicit conversion if exact large-integer arithmetic is required.

**Integer overflow:** Overflow behavior for integer arithmetic (e.g., int64 max + 1) is implementation-defined. Implementations may wrap, saturate, or error.

**Special float values (NaN, Infinity):** Division by zero is always an error — it does not produce Infinity or NaN. However, NaN and Infinity values may enter the system through input data. When they do, Bloblang follows IEEE 754 semantics:
- `NaN == NaN` is `false` (NaN is not equal to anything, including itself)
- `NaN != NaN` is `true`
- `NaN > x`, `NaN < x`, `NaN >= x`, `NaN <= x` are all `false` for any `x`
- Arithmetic with NaN produces NaN
- Infinity compares normally (`Infinity > 1.0` is `true`, `Infinity == Infinity` is `true`)

**Equality Semantics:**

For non-numeric types, both type and value must match for equality to return `true`. Different non-numeric types always return `false` (not an error).

For numeric types, the same promotion rules used for arithmetic apply before comparison. Both operands are promoted to a common numeric type, then compared by value. This means `5 == 5.0` is `true` (int64 promoted to float64, values match).

Cross-family comparisons (numeric vs non-numeric) always return `false`.

```bloblang
# Numeric equality: promotion rules applied
5 == 5.0                   # true (int64 promoted to float64, same value)
5.int32() == 5             # true (int32 promoted to int64, same value)
5.int32() == 5.float64()   # true (both promoted to float64, same value)
5 == 6.0                   # false (promoted, different value)

# Non-numeric types: type and value must match
"hello" == "hello"   # true
true == true         # true
null == null         # true
"a" == "b"           # false

# Cross-family: always false
5 == "5"             # false (numeric vs string)
true == 1            # false (bool vs numeric)
null == 0            # false (null vs numeric)

# Collections: structural equality (value-based, numeric promotion applies within)
[1, 2] == [1, 2]     # true (same contents)
[1, 2] == [2, 1]     # false (different order — arrays are ordered)
{"a": 1} == {"a": 1} # true (same keys and values)
{"a": 1, "b": 2} == {"b": 2, "a": 1}  # true (key order irrelevant for objects)
```

**Object key ordering:** Object key ordering is **not preserved**. Programs must not depend on iteration order in `map_object`, JSON serialization order, or any other context where keys are enumerated. Object equality compares keys and values regardless of order.

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
