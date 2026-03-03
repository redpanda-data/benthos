# 13. Standard Library

All implementations must provide these functions and methods. This is the complete required standard library — implementations may offer additional functions and methods beyond this list.

---

## 13.1 Functions

### `uuid_v4()`

Generate a random UUID v4 string.

- **Parameters:** none
- **Returns:** string
- **Example:** `uuid_v4()` → `"a3e7f1b0-1234-4abc-9def-567890abcdef"`

### `now()`

Return the current timestamp.

- **Parameters:** none
- **Returns:** timestamp (implementation-specific type; supports `.ts_format()` and `.ts_unix()`)
- **Example:** `now().ts_unix()` → `1709500000`

### `random()`

Return a random float64 in the range [0.0, 1.0).

- **Parameters:** none
- **Returns:** float64
- **Example:** `random()` → `0.7312...`

### `throw(message)`

Throw a custom error. The error propagates and can be caught with `.catch()`. If uncaught, it halts the mapping.

- **Parameters:** `message` (string, required). Non-string arguments are a compile-time error.
- **Returns:** never (always produces an error)
- **Example:** `throw("value is required")`
- **See:** Section 8.4

### `deleted()`

Return a deletion marker. When assigned to a target, removes it. When included in a collection literal, omits the element/field.

- **Parameters:** none
- **Returns:** deletion marker (not a runtime type)
- **See:** Section 9.2

---

## 13.2 Type Conversion Methods

These are the only way to create non-default numeric types, since literals are always int64 or float64.

### `.string()`

Convert a value to its string representation.

- **Receiver:** any type
- **Returns:** string
- **Examples:**
  ```bloblang
  42.string()          # "42"
  3.14.string()        # "3.14"
  true.string()        # "true"
  null.string()        # "null"
  ```

### `.int32()`

Convert a value to int32. Errors if the value cannot be represented as int32 (out of range or non-numeric string).

- **Receiver:** numeric types, string
- **Returns:** int32
- **Example:** `"42".int32()` → `42` (int32)

### `.int64()`

Convert a value to int64. Errors if the value cannot be represented as int64.

- **Receiver:** numeric types, string
- **Returns:** int64
- **Example:** `"42".int64()` → `42` (int64)

### `.uint32()`

Convert a value to uint32. Errors if the value is negative or out of range.

- **Receiver:** numeric types, string
- **Returns:** uint32
- **Example:** `"255".uint32()` → `255` (uint32)

### `.uint64()`

Convert a value to uint64. Errors if the value is negative or out of range.

- **Receiver:** numeric types, string
- **Returns:** uint64
- **Example:** `"1000".uint64()` → `1000` (uint64)

### `.float32()`

Convert a value to float32. Precision loss may occur for large values.

- **Receiver:** numeric types, string
- **Returns:** float32
- **Example:** `"3.14".float32()` → `3.14` (float32)

### `.float64()`

Convert a value to float64. Precision loss may occur for large integers.

- **Receiver:** numeric types, string
- **Returns:** float64
- **Example:** `"3.14".float64()` → `3.14` (float64)

### `.bool()`

Convert a value to boolean.

- **Receiver:** string (`"true"`, `"false"`), numeric (0 = false, non-zero = true)
- **Returns:** bool
- **Example:** `"true".bool()` → `true`

### `.bytes()`

Convert a value to a byte array (UTF-8 encoding for strings).

- **Receiver:** string, bytes
- **Returns:** bytes
- **Example:** `"hello".bytes()` → byte array (5 bytes)

---

## 13.3 Type Introspection

### `.type()`

Return the type name of a value as a string. Works on any type including null.

- **Receiver:** any type (including null)
- **Returns:** string — one of `"string"`, `"int32"`, `"int64"`, `"uint32"`, `"uint64"`, `"float32"`, `"float64"`, `"bool"`, `"null"`, `"bytes"`, `"array"`, `"object"`, `"lambda"`
- **Examples:**
  ```bloblang
  "hello".type()       # "string"
  42.type()            # "int64"
  3.14.type()          # "float64"
  null.type()          # "null"
  [1, 2].type()        # "array"
  {"a": 1}.type()      # "object"
  ```

---

## 13.4 String Methods

### `.uppercase()`

Convert a string to uppercase.

- **Receiver:** string
- **Returns:** string
- **Example:** `"hello".uppercase()` → `"HELLO"`

### `.lowercase()`

Convert a string to lowercase.

- **Receiver:** string
- **Returns:** string
- **Example:** `"HELLO".lowercase()` → `"hello"`

### `.trim()`

Remove leading and trailing whitespace.

- **Receiver:** string
- **Returns:** string
- **Example:** `"  hello  ".trim()` → `"hello"`

### `.split(delimiter)`

Split a string by a delimiter.

- **Receiver:** string
- **Parameters:** `delimiter` (string)
- **Returns:** array of strings
- **Examples:**
  ```bloblang
  "a,b,c".split(",")      # ["a", "b", "c"]
  "hello".split("")        # ["h", "e", "l", "l", "o"]
  ```

### `.replace_all(old, new)`

Replace all occurrences of a substring.

- **Receiver:** string
- **Parameters:** `old` (string), `new` (string)
- **Returns:** string
- **Example:** `"hello world".replace_all("o", "0")` → `"hell0 w0rld"`

---

## 13.5 Array Methods

### `.length()`

Return the number of elements. Also works on strings (codepoints), bytes (byte count), and objects (key count).

- **Receiver:** array, string, bytes, object
- **Returns:** int64
- **Examples:**
  ```bloblang
  [1, 2, 3].length()        # 3
  "hello".length()           # 5 (codepoints)
  "hello".bytes().length()   # 5 (bytes)
  {"a": 1, "b": 2}.length() # 2
  ```

### `.first()`

Return the first element of an array. Error on empty arrays.

- **Receiver:** array
- **Returns:** any (the element)
- **Examples:**
  ```bloblang
  [1, 2, 3].first()            # 1
  [null, 1].first()             # null (first element is null)
  [].first()                    # ERROR: empty array
  [].first().catch(err -> 0)    # 0
  ```

### `.last()`

Return the last element of an array. Error on empty arrays.

- **Receiver:** array
- **Returns:** any (the element)
- **Examples:**
  ```bloblang
  [1, 2, 3].last()             # 3
  [].last()                     # ERROR: empty array
  ```

### `.filter(elem -> bool)`

Return a new array containing only elements for which the lambda returns `true`. The lambda must return a boolean — non-boolean return values (including void) are an error.

- **Receiver:** array
- **Parameters:** lambda (one parameter → bool)
- **Returns:** array
- **Examples:**
  ```bloblang
  [1, 2, 3, 4].filter(x -> x > 2)     # [3, 4]
  [1, -2, 3].filter(x -> x > 0)        # [1, 3]
  ```

### `.map_array(elem -> expr)`

Transform each element of an array. Returns a new array.

- The lambda must return a value for every element — void is an error
- If the lambda returns `deleted()`, the element is omitted from the result

- **Receiver:** array
- **Parameters:** lambda (one parameter → any)
- **Returns:** array
- **Examples:**
  ```bloblang
  [1, 2, 3].map_array(x -> x * 2)                              # [2, 4, 6]
  [1, -2, 3].map_array(x -> if x > 0 { x } else { deleted() }) # [1, 3]
  [1, -2, 3].map_array(x -> if x > 0 { x * 10 })               # ERROR: void when x <= 0
  ```
- **See:** Section 4.1 for void and deleted() behavior in lambda returns

### `.sort()`

Sort an array in ascending order. Sort is **stable** (equal elements preserve relative order). All elements must be the same type family — mixed types are an error. Numeric types are promoted before comparison using the standard promotion rules (Section 2.3).

- **Receiver:** array
- **Returns:** array
- **Examples:**
  ```bloblang
  [3, 1, 2].sort()           # [1, 2, 3]
  ["b", "a", "c"].sort()     # ["a", "b", "c"]
  [3, 1.5, 2].sort()         # [1.5, 2, 3] (int64 promoted to float64)
  [1, "a", true].sort()      # ERROR: cannot sort mixed types
  ```

---

## 13.6 Object Methods

### `.map_object((key, value) -> expr)`

Transform each value of an object. Returns a new object with the same keys.

- The lambda must return a value for every entry — void is an error
- If the lambda returns `deleted()`, the key-value pair is removed from the result

- **Receiver:** object
- **Parameters:** lambda (two parameters: key string, value any → any)
- **Returns:** object
- **Examples:**
  ```bloblang
  {"a": 1, "b": 2}.map_object((k, v) -> v * 10)             # {"a": 10, "b": 20}
  {"x": "hello"}.map_object((k, v) -> v.uppercase())         # {"x": "HELLO"}
  {"a": 1, "b": -2}.map_object((k, v) -> if v > 0 { v } else { deleted() })  # {"a": 1}
  ```
- **See:** Section 4.1 for void and deleted() behavior in lambda returns

---

## 13.7 Numeric Methods

### `.round(n)`

Round a float to `n` decimal places using **half-even rounding** (banker's rounding, IEEE 754 default).

- **Receiver:** float32, float64
- **Parameters:** `n` (integer — number of decimal places)
- **Returns:** same float type as receiver
- **Examples:**
  ```bloblang
  3.456.round(2)     # 3.46
  2.5.round(0)       # 2.0 (half-even: rounds to nearest even)
  3.5.round(0)       # 4.0 (half-even: rounds to nearest even)
  ```

---

## 13.8 Time Methods

### `.ts_parse(format)`

Parse a string into a timestamp using the given format string.

- **Receiver:** string
- **Parameters:** `format` (string — Go-style time format, e.g. `"2006-01-02"`)
- **Returns:** timestamp
- **Errors:** if the string does not match the format
- **Example:** `"2024-03-01".ts_parse("2006-01-02")`

### `.ts_format(format)`

Format a timestamp as a string using the given format string.

- **Receiver:** timestamp
- **Parameters:** `format` (string — Go-style time format)
- **Returns:** string
- **Example:** `now().ts_format("2006-01-02")` → `"2024-03-01"`

### `.ts_unix()`

Convert a timestamp to a Unix timestamp (seconds since epoch).

- **Receiver:** timestamp
- **Returns:** int64
- **Example:** `now().ts_unix()` → `1709500000`

---

## 13.9 Error Handling Methods

### `.catch(err -> expr)`

Handle errors. Called only when the expression to its left produces an error. If the expression succeeds, `.catch()` returns its value unchanged. The error object has a single field: `.what` (string, the error message).

- **Receiver:** any expression (catches errors from the left-hand side)
- **Parameters:** lambda (one parameter: error object → any)
- **Returns:** any (either the original value or the lambda's result)
- **Examples:**
  ```bloblang
  input.date.ts_parse("2006-01-02").catch(err -> null)
  input.date.ts_parse("2006-01-02").catch(err -> throw("parse failed: " + err.what))
  ```
- **See:** Section 8.2

### `.or(default)`

Provide a default value for null. Uses **short-circuit evaluation** — the argument is only evaluated if the receiver is null.

- **Receiver:** any expression
- **Parameters:** `default` (any expression, lazily evaluated)
- **Returns:** any (either the original non-null value or the default)
- **Examples:**
  ```bloblang
  input.name.or("Anonymous")
  input.name.or(throw("name is required"))  # throw() only evaluated if name is null
  ```
- **See:** Section 8.3
