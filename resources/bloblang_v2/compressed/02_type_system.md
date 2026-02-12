# 2. Type System & Coercion

Bloblang V2 is **dynamically typed** - types determined at runtime.

## 2.1 Runtime Types

| Type | Description | Examples |
|------|-------------|----------|
| `string` | UTF-8 text (operations are codepoint-based) | `"hello"`, `""` |
| `number` | 64-bit float | `42`, `3.14`, `-10` |
| `bool` | Boolean | `true`, `false` |
| `null` | Null value | `null` |
| `bytes` | Byte array (operations are byte-based) | `"hello".bytes()` |
| `array` | Ordered collection | `[1, "two", true]` |
| `object` | Key-value map | `{"key": "value"}` |
| `lambda` | Function value | `x -> x * 2` |

**TODO:** Consider whether to expand the `number` type into distinct types (`int64`, `float64`, `uint64`, etc.) to provide better precision control and type safety, or keep the single general-purpose number type for simplicity.

**Important:** String operations (indexing, `.length()`, slicing, etc.) work on **Unicode codepoints**, not grapheme clusters. This means complex emoji and combining characters may span multiple codepoints. Byte operations work on individual bytes in the UTF-8 encoding.

## 2.2 Type Introspection

```bloblang
output.type = input.value.type()  # Returns type name as string

# Type checking
output.is_array = input.items.type() == "array"
output.is_null = input.maybe.type() == "null"
```

## 2.3 Type Coercion

**The `+` Operator:**
- Requires **same types**: both strings or both numbers
- **No implicit conversion**

```bloblang
# ✅ Valid
output.sum = 5 + 3              # 8 (number)
output.concat = "hello" + " world"  # "hello world" (string)

# ❌ Error: Type mismatch
output.bad = 5 + "3"            # ERROR

# ✅ Explicit conversion required
output.ok = 5.string() + "3"    # "53" (string)
output.ok2 = 5 + "3".number()   # 8 (number)
```

**Other Operators:**
- Arithmetic (`-`, `*`, `/`, `%`): Require numbers (null errors)
- Comparison (`>`, `<`, `>=`, `<=`): Require comparable same types (null errors)
- Equality (`==`, `!=`): Compare both type and value (see below)
- Logical (`&&`, `||`): Require booleans

**Equality Semantics:**

Both type and value must match for equality to return `true`. Different types always return `false` (not an error):

```bloblang
# Different types: always false
5 == "5"             # false (number vs string)
true == 1            # false (bool vs number)
null == 0            # false (null vs number)
null == false        # false (null vs bool)

# Same type, same value: true
5 == 5               # true
5.0 == 5             # true (both are numbers - 64-bit float)
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
null == 5            # false (different types: null vs number)
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
- `.number()` - Parse to number
- `.bool()` - Convert to boolean
- `.bytes()` - Convert to byte array
- `.type()` - Get type name

```bloblang
output.str = input.count.string()        # "42"
output.num = "3.14".number()             # 3.14
output.bool = "true".bool()              # true
output.bytes = "hello".bytes()           # byte array
```
