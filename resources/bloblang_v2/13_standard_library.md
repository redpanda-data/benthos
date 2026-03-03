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
- **Returns:** timestamp
- **Example:** `now().ts_unix()` → `1709500000`

### `random()`

Return a random float64 in the range [0.0, 1.0).

- **Parameters:** none
- **Returns:** float64
- **Example:** `random()` → `0.7312...`

### `random_int(min, max)`

Return a random int64 in the inclusive range [min, max]. Error if min > max.

- **Parameters:** `min` (int64), `max` (int64)
- **Returns:** int64
- **Example:** `random_int(1, 100)` → `42`

### `range(start, stop, step)`

Generate an array of integers from `start` (inclusive) to `stop` (exclusive) with the given step. Error if step is zero. If step is positive and start >= stop, or step is negative and start <= stop, returns an empty array.

- **Parameters:** `start` (int64), `stop` (int64), `step` (int64)
- **Returns:** array of int64
- **Examples:**
  ```bloblang
  range(0, 5, 1)      # [0, 1, 2, 3, 4]
  range(0, 10, 3)     # [0, 3, 6, 9]
  range(5, 0, -1)     # [5, 4, 3, 2, 1]
  range(0, 0, 1)      # []
  range(0, 5, 0)      # ERROR: step cannot be zero
  ```

### `char(codepoint)`

Convert a Unicode codepoint (int32) to a single-character string. This is the inverse of string indexing (`"hello"[0]` → `104`).

- **Parameters:** `codepoint` (int32 or int64 — must be a valid Unicode codepoint)
- **Returns:** string
- **Errors:** if the value is not a valid Unicode codepoint
- **Examples:**
  ```bloblang
  char(104)        # "h"
  char(233)        # "é"
  char(128512)     # "😀"
  char("hello"[0]) # "h" (round-trip from string indexing)
  ```

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
  42.string()          # "42" (int64 → string)
  3.14.string()        # "3.14" (float64 → string)
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
- **Returns:** string — one of `"string"`, `"int32"`, `"int64"`, `"uint32"`, `"uint64"`, `"float32"`, `"float64"`, `"bool"`, `"null"`, `"bytes"`, `"timestamp"`, `"array"`, `"object"`, `"lambda"`
- **Examples:**
  ```bloblang
  "hello".type()       # "string"
  42.type()            # "int64"
  3.14.type()          # "float64"
  null.type()          # "null"
  now().type()         # "timestamp"
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

### `.capitalize()`

Convert the first character of each word to uppercase and the rest to lowercase.

- **Receiver:** string
- **Returns:** string
- **Example:** `"hello world".capitalize()` → `"Hello World"`

### `.trim()`

Remove leading and trailing whitespace.

- **Receiver:** string
- **Returns:** string
- **Example:** `"  hello  ".trim()` → `"hello"`

### `.trim_prefix(prefix)`

Remove the given prefix from the start of the string. If the string does not start with the prefix, it is returned unchanged.

- **Receiver:** string
- **Parameters:** `prefix` (string)
- **Returns:** string
- **Examples:**
  ```bloblang
  "hello world".trim_prefix("hello ")   # "world"
  "hello world".trim_prefix("xyz")      # "hello world"
  ```

### `.trim_suffix(suffix)`

Remove the given suffix from the end of the string. If the string does not end with the suffix, it is returned unchanged.

- **Receiver:** string
- **Parameters:** `suffix` (string)
- **Returns:** string
- **Examples:**
  ```bloblang
  "hello world".trim_suffix(" world")   # "hello"
  "hello world".trim_suffix("xyz")      # "hello world"
  ```

### `.contains(substring)`

Check if a string contains a substring. Also works on arrays and objects (see Sections 13.5 and 13.6).

- **Receiver:** string
- **Parameters:** `substring` (string)
- **Returns:** bool
- **Examples:**
  ```bloblang
  "hello world".contains("world")   # true
  "hello world".contains("xyz")     # false
  ```

### `.has_prefix(prefix)`

Check if a string starts with the given prefix.

- **Receiver:** string
- **Parameters:** `prefix` (string)
- **Returns:** bool
- **Example:** `"hello world".has_prefix("hello")` → `true`

### `.has_suffix(suffix)`

Check if a string ends with the given suffix.

- **Receiver:** string
- **Parameters:** `suffix` (string)
- **Returns:** bool
- **Example:** `"hello world".has_suffix("world")` → `true`

### `.index_of(substring)`

Return the codepoint index of the first occurrence of a substring, or -1 if not found.

- **Receiver:** string
- **Parameters:** `substring` (string)
- **Returns:** int64
- **Examples:**
  ```bloblang
  "hello world".index_of("world")   # 6
  "hello world".index_of("xyz")     # -1
  ```

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

### `.slice(low, high)`

Extract a substring by codepoint indices. `low` is inclusive, `high` is exclusive. Negative indices count from the end. Also works on arrays (see Section 13.5).

- **Receiver:** string
- **Parameters:** `low` (int64), `high` (int64)
- **Returns:** string
- **Examples:**
  ```bloblang
  "hello world".slice(0, 5)    # "hello"
  "hello world".slice(6, 11)   # "world"
  "hello world".slice(-5, -1)  # "worl"
  ```

### `.reverse()`

Reverse a string by codepoints. Also works on arrays (see Section 13.5).

- **Receiver:** string
- **Returns:** string
- **Example:** `"hello".reverse()` → `"olleh"`

### `.repeat(count)`

Return the string repeated `count` times. Error if count is negative.

- **Receiver:** string
- **Parameters:** `count` (int64)
- **Returns:** string
- **Examples:**
  ```bloblang
  "ab".repeat(3)    # "ababab"
  "x".repeat(0)     # ""
  ```

### `.re_match(pattern)`

Test if a string matches a regular expression. Returns true if the pattern matches any part of the string.

- **Receiver:** string
- **Parameters:** `pattern` (string — regular expression)
- **Returns:** bool
- **Examples:**
  ```bloblang
  "hello123".re_match("[0-9]+")     # true
  "hello".re_match("^[a-z]+$")     # true
  "hello".re_match("^[0-9]+$")     # false
  ```

### `.re_find_all(pattern)`

Return all non-overlapping matches of a regular expression.

- **Receiver:** string
- **Parameters:** `pattern` (string — regular expression)
- **Returns:** array of strings
- **Examples:**
  ```bloblang
  "foo123bar456".re_find_all("[0-9]+")   # ["123", "456"]
  "hello".re_find_all("[0-9]+")          # []
  ```

### `.re_replace_all(pattern, replacement)`

Replace all matches of a regular expression with a replacement string.

- **Receiver:** string
- **Parameters:** `pattern` (string — regular expression), `replacement` (string)
- **Returns:** string
- **Example:** `"foo 123 bar 456".re_replace_all("[0-9]+", "N")` → `"foo N bar N"`

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

### `.contains(value)`

Check if an array contains a value (compared by equality). Also works on strings (see Section 13.4) and objects (see Section 13.6).

- **Receiver:** array
- **Parameters:** `value` (any)
- **Returns:** bool
- **Examples:**
  ```bloblang
  [1, 2, 3].contains(2)           # true
  [1, 2, 3].contains(4)           # false
  ["a", "b"].contains("b")        # true
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

### `.flat_map(elem -> array)`

Transform each element into an array and flatten the results into a single array. The lambda must return an array for every element — void is an error. If the lambda returns `deleted()`, the element is omitted.

- **Receiver:** array
- **Parameters:** lambda (one parameter → array)
- **Returns:** array
- **Examples:**
  ```bloblang
  [1, 2, 3].flat_map(x -> [x, x * 10])           # [1, 10, 2, 20, 3, 30]
  ["a,b", "c,d"].flat_map(x -> x.split(","))      # ["a", "b", "c", "d"]
  ```

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

### `.sort_by(elem -> key)`

Sort an array using a key function. Sort is **stable**. The lambda extracts a sort key from each element; keys are compared using the same rules as `.sort()`.

- **Receiver:** array
- **Parameters:** lambda (one parameter → comparable value)
- **Returns:** array
- **Examples:**
  ```bloblang
  [{"name": "Bob"}, {"name": "Alice"}].sort_by(x -> x.name)
  # [{"name": "Alice"}, {"name": "Bob"}]

  [3, -1, 2].sort_by(x -> x.abs())   # [-1, 2, 3] (sorted by absolute value)
  ```

### `.reverse()`

Reverse an array. Also works on strings (see Section 13.4).

- **Receiver:** array
- **Returns:** array
- **Example:** `[1, 2, 3].reverse()` → `[3, 2, 1]`

### `.slice(low, high)`

Extract a subarray by indices. `low` is inclusive, `high` is exclusive. Negative indices count from the end. Also works on strings (see Section 13.4).

- **Receiver:** array
- **Parameters:** `low` (int64), `high` (int64)
- **Returns:** array
- **Examples:**
  ```bloblang
  [1, 2, 3, 4, 5].slice(1, 3)    # [2, 3]
  [1, 2, 3, 4, 5].slice(-2, 5)   # [4, 5]
  ```

### `.take(n)`

Return the first `n` elements of an array. If `n` exceeds the array length, returns the entire array.

- **Receiver:** array
- **Parameters:** `n` (int64, must be >= 0)
- **Returns:** array
- **Examples:**
  ```bloblang
  [1, 2, 3, 4, 5].take(3)    # [1, 2, 3]
  [1, 2].take(10)             # [1, 2]
  [1, 2, 3].take(0)           # []
  ```

### `.drop(n)`

Return the array with the first `n` elements removed. If `n` exceeds the array length, returns an empty array.

- **Receiver:** array
- **Parameters:** `n` (int64, must be >= 0)
- **Returns:** array
- **Examples:**
  ```bloblang
  [1, 2, 3, 4, 5].drop(2)    # [3, 4, 5]
  [1, 2].drop(10)             # []
  ```

### `.append(value)`

Return a new array with the value appended to the end.

- **Receiver:** array
- **Parameters:** `value` (any)
- **Returns:** array
- **Example:** `[1, 2].append(3)` → `[1, 2, 3]`

### `.concat(other)`

Concatenate two arrays. Returns a new array with all elements from both.

- **Receiver:** array
- **Parameters:** `other` (array)
- **Returns:** array
- **Example:** `[1, 2].concat([3, 4])` → `[1, 2, 3, 4]`

### `.flatten()`

Flatten nested arrays by one level. Non-array elements are kept as-is.

- **Receiver:** array
- **Returns:** array
- **Examples:**
  ```bloblang
  [[1, 2], [3, 4]].flatten()          # [1, 2, 3, 4]
  [[1, [2]], [3]].flatten()            # [1, [2], 3] (one level only)
  [1, 2, 3].flatten()                  # [1, 2, 3] (no nesting, unchanged)
  ```

### `.unique()`

Remove duplicate elements, preserving the first occurrence of each value. Comparison uses equality semantics (Section 2.3).

- **Receiver:** array
- **Returns:** array
- **Examples:**
  ```bloblang
  [1, 2, 2, 3, 1].unique()        # [1, 2, 3]
  ["a", "b", "a"].unique()        # ["a", "b"]
  ```

### `.enumerate()`

Convert an array to an array of `{"index": i, "value": v}` objects.

- **Receiver:** array
- **Returns:** array of objects
- **Example:**
  ```bloblang
  ["a", "b", "c"].enumerate()
  # [{"index": 0, "value": "a"}, {"index": 1, "value": "b"}, {"index": 2, "value": "c"}]
  ```

### `.any(elem -> bool)`

Test if any element satisfies the predicate. Returns `false` for empty arrays. Short-circuits on first `true`.

- **Receiver:** array
- **Parameters:** lambda (one parameter → bool)
- **Returns:** bool
- **Examples:**
  ```bloblang
  [1, 2, 3].any(x -> x > 2)      # true
  [1, 2, 3].any(x -> x > 5)      # false
  [].any(x -> true)               # false
  ```

### `.all(elem -> bool)`

Test if all elements satisfy the predicate. Returns `true` for empty arrays. Short-circuits on first `false`.

- **Receiver:** array
- **Parameters:** lambda (one parameter → bool)
- **Returns:** bool
- **Examples:**
  ```bloblang
  [1, 2, 3].all(x -> x > 0)      # true
  [1, 2, 3].all(x -> x > 2)      # false
  [].all(x -> false)              # true
  ```

### `.find(elem -> bool)`

Return the first element that satisfies the predicate. Error if no element matches — use `.catch()` to handle.

- **Receiver:** array
- **Parameters:** lambda (one parameter → bool)
- **Returns:** any (the element)
- **Examples:**
  ```bloblang
  [1, 2, 3].find(x -> x > 1)                  # 2
  [1, 2, 3].find(x -> x > 5)                  # ERROR: no match
  [1, 2, 3].find(x -> x > 5).catch(err -> 0)  # 0
  ```

### `.join(delimiter)`

Join array elements into a string with a delimiter. All elements must be strings — non-string elements are an error.

- **Receiver:** array of strings
- **Parameters:** `delimiter` (string)
- **Returns:** string
- **Examples:**
  ```bloblang
  ["a", "b", "c"].join(",")     # "a,b,c"
  ["hello", "world"].join(" ")  # "hello world"
  [].join(",")                  # ""
  ```

### `.sum()`

Sum all numeric elements. Returns 0 for empty arrays. All elements must be numeric — non-numeric elements are an error. The result type follows standard promotion rules.

- **Receiver:** array of numeric values
- **Returns:** numeric (promoted type)
- **Examples:**
  ```bloblang
  [1, 2, 3].sum()        # 6 (int64)
  [1.5, 2.5].sum()       # 4.0 (float64)
  [1, 1.5, 2].sum()      # 4.5 (float64: int64 promoted to float64)
  [].sum()                # 0 (int64)
  ```

### `.fold(initial, (tally, elem) -> expr)`

Reduce an array to a single value by applying an accumulator function to each element. The lambda receives the running tally and the current element, and returns the new tally.

- **Receiver:** array
- **Parameters:** `initial` (any — starting value), lambda (two parameters: tally, element → any)
- **Returns:** any (the final tally)
- **Examples:**
  ```bloblang
  [1, 2, 3].fold(0, (tally, x) -> tally + x)          # 6
  [1, 2, 3].fold(1, (tally, x) -> tally * x)          # 6
  ["a", "b"].fold("", (tally, x) -> tally + x + ",")  # "a,b,"
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

### `.map_keys(key -> expr)`

Transform each key of an object. Returns a new object with transformed keys and original values.

- The lambda must return a string for every key — non-string return values are an error
- If the lambda returns `deleted()`, the key-value pair is removed from the result

- **Receiver:** object
- **Parameters:** lambda (one parameter: key string → string)
- **Returns:** object
- **Examples:**
  ```bloblang
  {"a": 1, "b": 2}.map_keys(k -> k.uppercase())             # {"A": 1, "B": 2}
  {"name": "Alice", "age": 30}.map_keys(k -> "user_" + k)   # {"user_name": "Alice", "user_age": 30}
  {"a": 1, "b": 2}.map_keys(k -> if k == "a" { k } else { deleted() })  # {"a": 1}
  ```

### `.keys()`

Return the keys of an object as an array of strings. Order is not guaranteed.

- **Receiver:** object
- **Returns:** array of strings
- **Example:** `{"a": 1, "b": 2}.keys()` → `["a", "b"]` (order not guaranteed)

### `.values()`

Return the values of an object as an array. Order is not guaranteed, but matches the order of `.keys()` for the same object in the same call.

- **Receiver:** object
- **Returns:** array
- **Example:** `{"a": 1, "b": 2}.values()` → `[1, 2]` (order not guaranteed)

### `.contains(key)`

Check if an object contains a key. Also works on strings (see Section 13.4) and arrays (see Section 13.5).

- **Receiver:** object
- **Parameters:** `key` (string)
- **Returns:** bool
- **Examples:**
  ```bloblang
  {"a": 1, "b": 2}.contains("a")     # true
  {"a": 1, "b": 2}.contains("c")     # false
  ```

### `.merge(other)`

Merge two objects. If both objects contain the same key, the value from `other` wins.

- **Receiver:** object
- **Parameters:** `other` (object)
- **Returns:** object
- **Examples:**
  ```bloblang
  {"a": 1, "b": 2}.merge({"b": 3, "c": 4})   # {"a": 1, "b": 3, "c": 4}
  {"a": 1}.merge({})                            # {"a": 1}
  ```

### `.without(keys)`

Return a new object with the specified keys removed. Keys that don't exist are ignored.

- **Receiver:** object
- **Parameters:** `keys` (array of strings)
- **Returns:** object
- **Examples:**
  ```bloblang
  {"a": 1, "b": 2, "c": 3}.without(["a", "c"])   # {"b": 2}
  {"a": 1}.without(["x"])                          # {"a": 1}
  {"a": 1, "b": 2}.without([])                     # {"a": 1, "b": 2}
  ```

### `.key_values()`

Convert an object to an array of `{"key": k, "value": v}` objects. Order is not guaranteed.

- **Receiver:** object
- **Returns:** array of objects
- **Example:**
  ```bloblang
  {"a": 1, "b": 2}.key_values()
  # [{"key": "a", "value": 1}, {"key": "b", "value": 2}] (order not guaranteed)
  ```

---

## 13.7 Numeric Methods

### `.abs()`

Return the absolute value.

- **Receiver:** int32, int64, float32, float64 (errors on unsigned types — already non-negative)
- **Returns:** same type as receiver
- **Examples:**
  ```bloblang
  (-5).abs()      # 5 (int64)
  3.14.abs()      # 3.14 (float64)
  (-3.14).abs()   # 3.14 (float64)
  ```

### `.floor()`

Return the largest integer value less than or equal to the number.

- **Receiver:** float32, float64
- **Returns:** same float type as receiver
- **Examples:**
  ```bloblang
  3.7.floor()     # 3.0 (float64)
  (-3.2).floor()  # -4.0 (float64)
  ```

### `.ceil()`

Return the smallest integer value greater than or equal to the number.

- **Receiver:** float32, float64
- **Returns:** same float type as receiver
- **Examples:**
  ```bloblang
  3.2.ceil()      # 4.0 (float64)
  (-3.7).ceil()   # -3.0 (float64)
  ```

### `.round(n)`

Round a float to `n` decimal places using **half-even rounding** (banker's rounding, IEEE 754 default).

- **Receiver:** float32, float64
- **Parameters:** `n` (int64 — number of decimal places)
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

### `.ts_unix_milli()`

Convert a timestamp to a Unix timestamp in milliseconds.

- **Receiver:** timestamp
- **Returns:** int64
- **Example:** `now().ts_unix_milli()` → `1709500000000`

### `.ts_unix_micro()`

Convert a timestamp to a Unix timestamp in microseconds.

- **Receiver:** timestamp
- **Returns:** int64
- **Example:** `now().ts_unix_micro()` → `1709500000000000`

### `.ts_unix_nano()`

Convert a timestamp to a Unix timestamp in nanoseconds.

- **Receiver:** timestamp
- **Returns:** int64
- **Example:** `now().ts_unix_nano()` → `1709500000000000000`

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

---

## 13.10 Parsing Methods

### `.parse_json()`

Parse a JSON string into a value. Errors if the string is not valid JSON.

- **Receiver:** string, bytes
- **Returns:** any (the parsed value)
- **Examples:**
  ```bloblang
  `{"name":"Alice"}`.parse_json()    # {"name": "Alice"}
  `[1,2,3]`.parse_json()            # [1, 2, 3]
  `"hello"`.parse_json()            # "hello"
  ```

### `.format_json()`

Serialize a value to a JSON string.

- **Receiver:** any type (except lambda)
- **Returns:** string
- **Examples:**
  ```bloblang
  {"name": "Alice"}.format_json()   # `{"name":"Alice"}`
  [1, 2, 3].format_json()          # `[1,2,3]`
  ```
