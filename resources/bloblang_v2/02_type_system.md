# 2. Type System & Coercion

Bloblang V2 is **dynamically typed** - types determined at runtime.

## 2.1 Runtime Types

| Type | Description | Examples |
|------|-------------|----------|
| `string` | UTF-8 text (operations are codepoint-based) | `"hello"`, `""` |
| `int32` | 32-bit signed integer | `42.int32()` |
| `int64` | 64-bit signed integer (default for integer literals) | `42`, `-10` (unary minus) |
| `uint32` | 32-bit unsigned integer | `42.uint32()` |
| `uint64` | 64-bit unsigned integer | `42.uint64()`, `"18446744073709551615".uint64()` |
| `float32` | 32-bit IEEE 754 float | `3.14.float32()` |
| `float64` | 64-bit IEEE 754 float (default for float literals) | `3.14`, `-10.5` (unary minus) |
| `bool` | Boolean | `true`, `false` |
| `null` | Null value | `null` |
| `bytes` | Byte array (operations are byte-based; no implicit JSON serialization — see Section 13.11) | `"hello".bytes()` |
| `array` | Ordered collection | `[1, "two", true]` |
| `object` | Key-value map | `{"key": "value"}` |
| `timestamp` | Point in time with nanosecond precision | `now()`, `"2024-03-01".ts_parse("%Y-%m-%d")` |
| `lambda` | Function value (see assignment restrictions below) | `x -> x * 2` |

**Large uint64 values:** Integer literals are always int64, so values exceeding int64 range (> 9223372036854775807) cannot be written as bare literals. To create large uint64 values, parse from a string: `"18446744073709551615".uint64()`. Writing the value as a bare literal (e.g., `18446744073709551615.uint64()`) is a compile error because the literal exceeds int64 range before the conversion method is applied.

**Important:** String operations (indexing, `.length()`, etc.) work on **Unicode codepoints**, not grapheme clusters. This means complex emoji and combining characters may span multiple codepoints. Byte operations work on individual bytes in the UTF-8 encoding.

**No Unicode normalization:** Strings are compared codepoint-by-codepoint without normalization. Different Unicode representations of the same visual character (e.g., precomposed `é` U+00E9 vs decomposed `e` U+0065 + `◌́` U+0301) are **not equal** and may have different `.length()` values. This matches the behavior of Go, Rust, and most systems languages. If input data may contain mixed normalization forms, use an explicit normalization step before comparison.

**Lambda restrictions:** Lambdas are computation values, not data values — they cannot be serialized or returned from calls.

*Assignment:* The only valid assignment target for a lambda is a plain variable (`$fn = x -> x * 2`). Assigning a lambda to any other target is a runtime error:
- `output.field = lambda` — error
- `output@.key = lambda` — error
- `$var.field = lambda` — error (nested field of a variable)

*Collection literals:* Lambdas cannot appear inside array or object literals. This is a runtime error regardless of the assignment target:
- `[x -> x * 2]` — error
- `{"fn": x -> x}` — error

*Return values:* Maps, lambdas, functions, and methods cannot return lambda values. If a map body, lambda body, or method produces a lambda as its result, this is a runtime error. Lambdas can be passed as arguments (e.g., `.map(x -> x * 2)`) and stored in plain variables, but they cannot flow out of a call as a return value.

These restrictions ensure that lambdas remain in the computation domain — they are used to parameterize operations, not to build data structures or create higher-order call chains.

**Void:** Void is not a runtime type — it is the absence of a value, produced when an if-expression without `else` has a false condition, or when a match expression without `_` has no matching case (Section 4.1). Void cannot be stored in variables, passed as arguments, used in expressions, or included in collection literals (all are errors). It only exists transiently to signal "no value was produced," and is only meaningful in assignments where it causes the assignment to be skipped (a no-op). The exceptions are `.or()`, which rescues void by returning its argument (Section 8.3), and `.catch()`, which passes void through unchanged (Section 8.2). All other method calls on void are errors — e.g., `.type()` on void is not possible. See Section 4.1 for full void semantics.

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
- Comparison (`>`, `<`, `>=`, `<=`): Require comparable same types (null errors), with numeric promotion. Comparable types are: numeric types (with promotion), timestamps, strings (lexicographic by Unicode codepoint), and bytes (lexicographic by byte value)
- Equality (`==`, `!=`): Numeric types use promotion then compare by value; non-numeric types require same type and value; cross-family is always `false` (see below)
- Logical (`!`, `&&`, `||`): Require booleans

### Numeric Type Promotion

When arithmetic, comparison, or equality operators are applied to operands of different numeric types, both operands are promoted to a common type before the operation. Non-numeric types (string, bool, null, etc.) are never promoted — mixing them with numbers is always an error (for arithmetic/comparison) or always `false` (for equality).

**Promotion rules (applied in order):**

| Operand types | Promoted to | Error condition |
|---------------|-------------|----------------|
| Same type | No promotion | `int64 + int64 → int64` |
| Same signedness, different width | Wider type | `int32 + int64 → int64`, `float32 + float64 → float64` |
| Signed + unsigned integer | int64 | Error if uint64 value > 2^63-1 (cannot fit in int64) |
| Any integer + any float | float64 | Error if integer magnitude > 2^53 (cannot be represented exactly) |

**Promotion is checked, not silent.** All widening promotions that are lossless (e.g., int32 → int64, float32 → float64) always succeed. Promotions that may lose data are validated at runtime and **throw an error** if the value cannot be represented exactly in the target type. This applies to both operands — if either operand cannot be safely promoted, the operation errors.

**Division always produces a float:**

| Operand types | Result type |
|---------------|-------------|
| float32 / float32 | float32 |
| All other combinations | float64 |

There is no integer division operator. To get an integer result, convert explicitly: `(7 / 2).int64()` truncates toward zero (result: `3`), `(7 / 2).floor().int64()` floors (result: `3`). These differ for negative operands: `(-7 / 2).int64()` is `-3` (truncation), `(-7 / 2).floor().int64()` is `-4` (floor).

**Modulo follows standard promotion rules** (not the division rule). The result type is determined by the promoted operand type. For float operands, modulo uses **truncated division remainder** semantics (equivalent to C `fmod`), where the result has the same sign as the dividend:

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

# Different width: promote to wider (always lossless)
output.c = 5.int32() + 10           # 15 (int64: int32 promoted to int64)

# Signed + unsigned: promote to int64 (checked)
output.d = 5.int32() + 10.uint32()  # 15 (int64: both fit)
output.bad = 5 + "9999999999999999999".uint64()  # ERROR: uint64 value exceeds int64 range

# Integer + float: promote to float64 (checked)
output.e = 5 + 3.0                  # 8.0 (float64: 5 fits exactly)
output.bad = 9007199254740993 + 1.0 # ERROR: int64 value exceeds float64 exact range (> 2^53)

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

**Integer overflow:** Integer arithmetic that overflows the result type is always a runtime error. This applies to all integer types (int32, int64, uint32, uint64) and all arithmetic operators (`+`, `-`, `*`, `%`). Implementations must detect overflow and throw an error rather than wrapping or saturating.

```bloblang
output.bad = 9223372036854775807 + 1    # ERROR: int64 overflow
output.bad = (-2147483648).int32() - 1.int32()  # ERROR: int32 overflow
output.ok = 9223372036854775807.uint64() + 1.uint64()  # 9223372036854775808 (uint64, no overflow)
```

**Special float values (NaN, Infinity):** Division by zero is always an error — it does not produce Infinity or NaN. However, NaN and Infinity values may enter the system through input data. When they do, Bloblang follows IEEE 754 semantics:
- `NaN == NaN` is `false` (NaN is not equal to anything, including itself)
- `NaN != NaN` is `true`
- `NaN > x`, `NaN < x`, `NaN >= x`, `NaN <= x` are all `false` for any `x`
- Arithmetic with NaN produces NaN
- Infinity compares normally (`Infinity > 1.0` is `true`, `Infinity == Infinity` is `true`)
- Negative zero: `-0.0 == 0.0` is `true`, `-0.0 < 0.0` is `false` (they are equal per IEEE 754). `.string()` normalizes to `"0.0"` (not `"-0.0"`).

**NaN in other contexts:** `.sort()` uses total ordering for NaN — NaN sorts after all other numeric values, not IEEE 754 comparison (Section 13.6). `.unique()` treats all NaN values as equal, consistent with sort's total ordering (Section 13.6). `.bool()` on NaN is an error — NaN is neither zero nor non-zero (Section 13.2).

**Equality Semantics:**

For non-numeric types, both type and value must match for equality to return `true`. Strings compare codepoint-by-codepoint, bytes compare byte-by-byte, arrays compare element-by-element, and objects compare by key-value pairs regardless of key order. Different non-numeric types always return `false` (not an error). **Exception:** lambdas cannot be compared for equality — any `==` or `!=` with a lambda operand is a runtime error.

For numeric types, the same promotion rules used for arithmetic apply before comparison. Both operands are promoted to a common numeric type, then compared by value. This means `5 == 5.0` is `true` (int64 promoted to float64, values match).

Cross-family comparisons (numeric vs non-numeric) always return `false` (except lambdas, which error).

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

# Lambdas: equality is an error
(x -> x) == (x -> x) # ERROR: lambdas cannot be compared for equality
```

**Object key ordering:** Object key ordering is **not preserved**. Programs must not depend on iteration order in `.iter()`, JSON serialization order, or any other context where keys are enumerated. Object equality compares keys and values regardless of order.

**Timestamp semantics:** Timestamps represent a point in time with nanosecond precision. They support:

- **Equality and comparison:** Timestamps can be compared with `==`, `!=`, `<`, `>`, `<=`, `>=`. Earlier times are less than later times.
- **Arithmetic:** `timestamp - timestamp` returns an int64 (duration in nanoseconds). No other arithmetic operations are supported — adding two timestamps, or adding a number to a timestamp, is an error. Use `.ts_add(nanos)` to offset a timestamp by a duration, or `.ts_unix()` and related methods for numeric conversions. **Note:** int64 nanoseconds can represent approximately ±292 years; subtracting timestamps further apart than this is an integer overflow error (Section 2.3).
- **Methods:** `.ts_format()`, `.ts_add()`, `.ts_unix()`, `.ts_unix_milli()`, `.ts_unix_micro()`, `.ts_unix_nano()`, `.type()`, `.string()`.
- **Construction from numeric:** `.ts_from_unix()` on any numeric type (integers for second precision; floats for sub-second precision, limited by float64's ~15-17 significant digits). For exact sub-second precision, use `.ts_from_unix_milli()`, `.ts_from_unix_micro()`, or `.ts_from_unix_nano()` on int64 values. See Section 13.9.
- **Serialization:** When serialized to JSON, timestamps are formatted as RFC 3339 strings. When converted with `.string()`, the result is also RFC 3339. Trailing fractional zeros are trimmed (e.g., `.500000000` becomes `.5`; whole-second timestamps omit the fractional part entirely).

```bloblang
$a = now()
$b = now()
$a < $b                    # true (earlier < later)
$a == $a                   # true
$b - $a                    # int64: nanoseconds between the two timestamps
$a + 1                     # ERROR: cannot add timestamp and int64
$a.string()                # "2024-03-01T12:00:00Z" (RFC 3339, trailing zeros trimmed)
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

# ✅ .or() provides defaults for null, void, or deleted()
input.value.or("default")  # "default" if value is null, void, or deleted()
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
