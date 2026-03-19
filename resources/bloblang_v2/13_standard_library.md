# 13. Standard Library

All implementations must provide these functions and methods. This is the complete required standard library — implementations may offer additional functions and methods beyond this list.

**Namespace sharing:** Standard library functions share the same namespace as user-defined maps. User-defined maps shadow standard library functions of the same name. Resolution priority: parameters > maps > standard library functions. Like map names, standard library function names without parentheses can be passed as arguments to higher-order methods (e.g., `.sort_by(abs)`) but cannot be used as general-purpose expressions or stored in variables (Section 5.5).

**Named arguments and arity:** All standard library functions and methods support named arguments using the parameter names shown in their signatures. For example, `random_int(min: 1, max: 100)` and `.replace_all(old: "x", new: "y")` are valid. The same rules apply as for user maps (Section 5.3): positional and named arguments cannot be mixed in the same call, duplicate named arguments are a compile-time error, and extra or mismatched arguments are errors. Parameters with default values may be omitted (Section 5.1). Parameters marked with `?` in their signatures are truly optional — they may be omitted entirely, and the method's documented behavior applies when they are absent.

**Parameter type promotion:** When a method or function signature documents a specific numeric type (e.g., `int64`), any numeric type is accepted. Integer parameters accept any numeric value that is a whole number — float values like `2.0` are accepted but `1.5` is an error (consistent with indexing rules in Section 3.1). Float parameters accept any numeric type (integers are promoted to float using the standard promotion rules in Section 2.3). In all cases, checked promotion applies — values that cannot be represented exactly in the target type are a runtime error.

**Lambda return values — void and `deleted()`:** Unless a method's documentation explicitly states otherwise, void and `deleted()` as lambda return values are runtime errors. Methods that support `deleted()` as a lambda return document this explicitly — see `.map()`, `.map_values()`, `.map_keys()`, `.map_entries()` (element/entry omitted from result), and `.catch()` (result flows to calling context with normal semantics).

**Regular expressions:** All regex parameters use [RE2 syntax](https://github.com/google/re2/wiki/Syntax). RE2 guarantees linear-time matching (no catastrophic backtracking). Notable exclusions from RE2: backreferences and lookahead/lookbehind assertions are not supported.

---

## 13.1 Functions

### `uuid_v4()`

Generate a random UUID v4 string.

- **Parameters:** none
- **Returns:** string
- **Example:** `uuid_v4()` → `"a3e7f1b0-1234-4abc-9def-567890abcdef"`

### `now()`

Return the current timestamp. Each call returns a fresh timestamp — consecutive calls may return different values, including within `.map()` lambdas.

- **Parameters:** none
- **Returns:** timestamp
- **Example:** `now().ts_unix()` → `1709500000`

### `random_int(min, max)`

Return a random int64 in the inclusive range [min, max]. Error if min > max.

- **Parameters:** `min` (int64), `max` (int64)
- **Returns:** int64
- **Example:** `random_int(1, 100)` → `42`

### `range(start, stop, step?)`

Generate an array of integers from `start` (inclusive) to `stop` (exclusive) with the given step. When `step` is omitted, the direction is inferred: `1` if `start <= stop`, `-1` if `start > stop`. Error if step is zero. Error if an explicit step contradicts the direction (positive step with start > stop, or negative step with start < stop). If start == stop, returns an empty array regardless of step.

- **Parameters:** `start` (int64), `stop` (int64), `step` (int64, optional — inferred from direction when omitted)
- **Returns:** array of int64
- **Examples:**
  ```bloblang
  range(0, 5)         # [0, 1, 2, 3, 4] (step inferred as 1)
  range(5, 0)         # [5, 4, 3, 2, 1] (step inferred as -1)
  range(0, 5, 1)      # [0, 1, 2, 3, 4] (explicit step)
  range(0, 10, 3)     # [0, 3, 6, 9]
  range(5, 0, -2)     # [5, 3, 1]
  range(0, 0)         # []
  range(0, 5, -1)     # ERROR: step direction contradicts start/stop
  range(5, 0, 1)      # ERROR: step direction contradicts start/stop
  range(0, 5, 0)      # ERROR: step cannot be zero
  ```

### `timestamp(year, month, day, hour = 0, minute = 0, second = 0, nano = 0, timezone = "UTC")`

Construct a timestamp from individual components.

- **Parameters:**
  - `year` (int64)
  - `month` (int64, 1–12)
  - `day` (int64, 1–31)
  - `hour` (int64, 0–23, default `0`)
  - `minute` (int64, 0–59, default `0`)
  - `second` (int64, 0–59, default `0`)
  - `nano` (int64, 0–999999999, default `0`)
  - `timezone` (string, IANA timezone name or `"UTC"`, default `"UTC"`)
- **Returns:** timestamp
- **Errors:** if any component is out of range, or if the timezone is not recognized
- **Examples:**
  ```bloblang
  timestamp(2024, 3, 1)                                    # 2024-03-01T00:00:00Z
  timestamp(2024, 3, 1, 12, 30, 0)                         # 2024-03-01T12:30:00Z
  timestamp(2024, 3, 1, 12, 30, 0, 0, "America/New_York")  # 2024-03-01T12:30:00-05:00
  timestamp(year: 2024, month: 3, day: 1)                  # 2024-03-01T00:00:00Z
  timestamp(year: 2024, month: 12, day: 25, hour: 8)       # 2024-12-25T08:00:00Z
  ```

### `second()`

Return the number of nanoseconds in one second (`1000000000`). This is a convenience constant for use with `.ts_add()` and other duration arithmetic.

- **Parameters:** none
- **Returns:** int64 (`1000000000`)
- **Examples:**
  ```bloblang
  now().ts_add(second())              # 1 second later
  now().ts_add(second() * -30)        # 30 seconds ago
  ```

### `minute()`

Return the number of nanoseconds in one minute (`60000000000`). Convenience constant equivalent to `second() * 60`.

- **Parameters:** none
- **Returns:** int64 (`60000000000`)
- **Examples:**
  ```bloblang
  now().ts_add(minute())              # 1 minute later
  now().ts_add(minute() * 5)          # 5 minutes later
  ```

### `hour()`

Return the number of nanoseconds in one hour (`3600000000000`). Convenience constant equivalent to `second() * 3600`.

- **Parameters:** none
- **Returns:** int64 (`3600000000000`)
- **Examples:**
  ```bloblang
  now().ts_add(hour())                # 1 hour later
  now().ts_add(hour() * -2)           # 2 hours ago
  ```

### `day()`

Return the number of nanoseconds in one day (`86400000000000`). Convenience constant equivalent to `second() * 86400`. Note: this is exactly 24 hours — it does not account for daylight saving time transitions or leap seconds.

- **Parameters:** none
- **Returns:** int64 (`86400000000000`)
- **Examples:**
  ```bloblang
  now().ts_add(day())                 # 1 day later
  now().ts_add(day() * 7)             # 1 week later
  ```

### `throw(message)`

Throw a custom error. The error propagates and can be caught with `.catch()`. If uncaught, it halts the mapping.

- **Parameters:** `message` (string, required). Non-string literal arguments are a compile-time error; dynamic arguments that evaluate to a non-string type at runtime are a runtime error.
- **Returns:** never (always produces an error)
- **Example:** `throw("value is required")`
- **See:** Section 8.4

### `deleted()`

Return a deletion marker. When assigned to a field or metadata key, removes it. When included in a collection literal, omits the element/field. When assigned to the root output (`output = deleted()`), drops the entire message and immediately exits the mapping. Assigning `deleted()` to a variable (`$var = deleted()`) is a runtime error.

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
- **Conversion rules:**
  - Integer types: decimal representation (`42` → `"42"`, `-10` → `"-10"`)
  - Float types: any shortest decimal representation that round-trips exactly back to the same float value, always including either a decimal point or exponent to distinguish the result from an integer string. When a non-exponent form and an exponent form have the same length, prefer the non-exponent form. `0.0` → `"0.0"`, `3.14` → `"3.14"`, `1000000.0` → `"1e+06"` (exponent form is shorter). Negative zero normalizes to `"0.0"` (not `"-0.0"`). NaN produces `"NaN"`, Infinity produces `"Infinity"`, negative Infinity produces `"-Infinity"`. **Cross-implementation note:** Different shortest-representation algorithms (Ryu, Grisu3, etc.) may produce different valid outputs for the same float value. Conformance tests should compare parsed numeric values rather than exact string representations.
  - Bool: `"true"` or `"false"`
  - Null: `"null"`
  - Timestamp: RFC 3339 format with shortest-precision fractional seconds — trailing zeros are removed and the fractional part (including `.`) is omitted entirely when zero. Examples: `"2024-03-01T12:00:00Z"`, `"2024-03-01T12:00:00.123Z"`. UTC is represented as `Z`. This matches `.ts_format()` with default arguments.
  - Bytes: UTF-8 decode (error if bytes are not valid UTF-8)
  - Array, object: compact JSON (equivalent to `.format_json()` with default parameters — object keys sorted lexicographically by Unicode codepoint value)
  - Lambda: error
  - **Containers with bytes:** If an array or object contains a bytes value (at any nesting depth), `.string()` errors — bytes have no implicit serialization format. Convert bytes explicitly before including them in structures that will be stringified (e.g., use `.encode("base64")` or `.string()` on individual bytes values before embedding them in arrays or objects).
- **Examples:**
  ```bloblang
  42.string()          # "42" (int64 → string)
  3.14.string()        # "3.14" (float64 → string)
  0.0.string()         # "0.0" (float64 — always includes decimal point)
  true.string()        # "true"
  null.string()        # "null"
  [1, 2].string()      # "[1,2]"
  {"a": 1}.string()    # "{\"a\":1}"
  ```

### `.int32()`

Convert a value to int32. Errors if the value cannot be represented as int32 (out of range or non-numeric string). Float values are **truncated** toward zero (fractional part discarded). Errors if the truncated value is out of int32 range.

- **Receiver:** numeric types, string
- **Returns:** int32
- **Examples:**
  ```bloblang
  "42".int32()       # 42 (int32)
  3.7.int32()        # 3 (int32: truncated toward zero)
  (-3.7).int32()     # -3 (int32: truncated toward zero)
  ```

### `.int64()`

Convert a value to int64. Errors if the value cannot be represented as int64. Float values are **truncated** toward zero.

- **Receiver:** numeric types, string
- **Returns:** int64
- **Examples:**
  ```bloblang
  "42".int64()       # 42 (int64)
  3.9.int64()        # 3 (int64: truncated toward zero)
  ```

### `.uint32()`

Convert a value to uint32. Errors if the value is negative or out of range. Float values are **truncated** toward zero.

- **Receiver:** numeric types, string
- **Returns:** uint32
- **Example:** `"255".uint32()` → `255` (uint32)

### `.uint64()`

Convert a value to uint64. Errors if the value is negative or out of range. Float values are **truncated** toward zero.

- **Receiver:** numeric types, string
- **Returns:** uint64
- **Example:** `"1000".uint64()` → `1000` (uint64)

### `.float32()`

Convert a value to float32. Precision loss may occur for large values. Unlike implicit numeric promotion in arithmetic (Section 2.3), explicit conversion methods are unchecked — the caller is opting in to potential precision loss.

- **Receiver:** numeric types, string
- **Returns:** float32
- **Example:** `"3.14".float32()` → `3.14` (float32)

### `.float64()`

Convert a value to float64. Precision loss may occur for large integers. Unlike implicit numeric promotion in arithmetic (Section 2.3), explicit conversion methods are unchecked — the caller is opting in to potential precision loss.

- **Receiver:** numeric types, string
- **Returns:** float64
- **Example:** `"3.14".float64()` → `3.14` (float64)

### `.bool()`

Convert a value to boolean.

- **Receiver:** bool (identity — returned as-is), string (`"true"`, `"false"`), numeric (0 = false, non-zero = true)
- **Returns:** bool
- **Special float values:** Negative zero (`(-0.0).bool()`) is `false` (it is equal to zero per IEEE 754). Infinity and negative Infinity are `true` (non-zero). NaN is an error (neither zero nor non-zero).
- **Design note:** Numeric-to-boolean conversion is an explicit opt-in via `.bool()` — it does not happen implicitly. Logical operators (`&&`, `||`, `!`) still require boolean operands; `5 && true` is an error. This differs from V1, where numbers were silently accepted as booleans in logical expressions.
- **Examples:**
  ```bloblang
  true.bool()      # true (identity)
  "true".bool()    # true
  "false".bool()   # false
  1.bool()         # true
  0.bool()         # false
  ```

### `.char()`

Convert an integer (Unicode codepoint) to a single-character string. This is the inverse of string indexing (`"hello"[0]` → `104`).

- **Receiver:** any integer type (int64, int32, uint32, uint64)
- **Returns:** string (single codepoint)
- **Errors:** if the value is not a valid Unicode codepoint
- **Examples:**
  ```bloblang
  104.char()        # "h"
  233.char()        # "é"
  128512.char()     # "😀"
  "hello"[0].char() # "h" (round-trip from string indexing)
  ```

### `.bytes()`

Convert a value to a byte array. For strings, returns the UTF-8 encoding. For all other supported types, equivalent to `.string().bytes()` (UTF-8 encoding of the string representation).

- **Receiver:** any type
- **Returns:** bytes
- **Conversion rules:**
  - String: UTF-8 encoding
  - Bytes: returned as-is
  - All other types (numeric, bool, null, timestamp, array, object): UTF-8 encoding of `.string()` result. Since this goes through `.string()`, containers (arrays/objects) with nested bytes values will error — convert bytes values explicitly (e.g., `.encode("base64")`) before calling `.bytes()` on a container.
  - Lambda: error
- **Examples:**
  ```bloblang
  "hello".bytes()          # byte array (5 bytes)
  "hello".bytes().bytes()  # byte array (unchanged)
  42.bytes()               # byte array of "42" (2 bytes)
  true.bytes()             # byte array of "true" (4 bytes)
  ```

---

## 13.3 Type Introspection

### `.type()`

Return the type name of a value as a string. Works on any type including null.

- **Receiver:** any type (including null)
- **Returns:** string — one of `"string"`, `"int32"`, `"int64"`, `"uint32"`, `"uint64"`, `"float32"`, `"float64"`, `"bool"`, `"null"`, `"bytes"`, `"timestamp"`, `"array"`, `"object"`
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

## 13.4 Sequence Methods

These methods work on multiple sequence-like types: strings (codepoint-based), arrays (element-based), and bytes (byte-based). Each method is documented once; per-type behavior is noted where it differs.

### `.length()`

Return the length of a sequence, or the number of keys in an object.

- **Receiver:** string, array, bytes, object
- **Returns:** int64
- **Semantics:** strings count codepoints, arrays count elements, bytes count bytes, objects count keys
- **Examples:**
  ```bloblang
  "hello".length()           # 5 (codepoints)
  [1, 2, 3].length()        # 3 (elements)
  "hello".bytes().length()   # 5 (bytes)
  {"a": 1, "b": 2}.length() # 2 (keys)
  ```

### `.contains(target)`

Check if a sequence contains the given target.

- **Receiver:** string, array, bytes
- **Parameters:** `target` — string (for string/bytes receiver) or any (for array receiver)
- **Returns:** bool
- **Semantics:**
  - **string:** searches for a substring
  - **array:** searches for an element by equality
  - **bytes:** searches for a byte subsequence (target must be bytes)
- **Examples:**
  ```bloblang
  "hello world".contains("world")     # true
  [1, 2, 3].contains(2)              # true
  "hello".bytes().contains("ll".bytes())  # true
  ```
- **Note:** For object key checking, see `.has_key()` (Section 13.7).

### `.index_of(target)`

Return the index of the first occurrence of the target, or -1 if not found.

- **Receiver:** string, array, bytes
- **Parameters:** `target` — string (for string receiver), any (for array receiver), bytes (for bytes receiver)
- **Returns:** int64
- **Semantics:**
  - **string:** returns codepoint index of first occurrence of substring
  - **array:** returns element index of first match by equality
  - **bytes:** returns byte index of first occurrence of byte subsequence
- **Examples:**
  ```bloblang
  "hello world".index_of("world")    # 6
  [10, 20, 30].index_of(20)         # 1
  "hello".bytes().index_of("ll".bytes())  # 2
  ```

### `.slice(low, high?)`

Extract a subsequence. `low` is inclusive, `high` is exclusive. When `high` is omitted, the slice extends to the end of the sequence. Negative indices count from the end. Indices are clamped to the length — out-of-bounds indices do not error. If `low >= high` after clamping, returns an empty value of the same type.

- **Receiver:** string, array, bytes
- **Parameters:** `low` (int64), `high` (int64, optional — defaults to sequence length when omitted)
- **Returns:** same type as receiver
- **Semantics:** strings slice by codepoint, arrays by element, bytes by byte
- **Examples:**
  ```bloblang
  "hello world".slice(0, 5)          # "hello"
  "hello world".slice(6)             # "world" (high omitted — to end)
  "hello world".slice(-5, -1)        # "worl"
  [1, 2, 3, 4, 5].slice(1, 3)       # [2, 3]
  [1, 2, 3, 4, 5].slice(2)          # [3, 4, 5] (high omitted — to end)
  "hello".bytes().slice(0, 3)        # bytes("hel")
  "hello".slice(0, 100)              # "hello" (high clamped to length)
  [1, 2, 3].slice(3, 1)             # [] (low >= high)
  ```

### `.reverse()`

Reverse a sequence.

- **Receiver:** string, array, bytes
- **Returns:** same type as receiver
- **Semantics:** strings reverse by codepoint, arrays by element, bytes by byte
- **Examples:**
  ```bloblang
  "hello".reverse()                  # "olleh"
  [1, 2, 3].reverse()               # [3, 2, 1]
  "hello".bytes().reverse()          # bytes("olleh")
  ```

---

## 13.5 String Methods

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

Remove leading and trailing Unicode whitespace — characters with the Unicode `White_Space` property (space, `\t`, `\n`, `\r`, `\f`, `\v`, non-breaking space U+00A0, and other Unicode space characters).

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

### `.split(delimiter)`

Split a string by a delimiter.

- **Receiver:** string
- **Parameters:** `delimiter` (string)
- **Returns:** array of strings
- **Examples:**
  ```bloblang
  "a,b,c".split(",")      # ["a", "b", "c"]
  "hello".split("")        # ["h", "e", "l", "l", "o"]
  "👋🏽".split("")          # ["👋", "🏽"] (splits by codepoint, not grapheme)
  "".split("")             # [] (no codepoints)
  "".split(",")            # [""] (empty string with non-empty delimiter)
  ```

### `.replace_all(old, new)`

Replace all occurrences of a substring.

- **Receiver:** string
- **Parameters:** `old` (string), `new` (string)
- **Returns:** string
- **Example:** `"hello world".replace_all("o", "0")` → `"hell0 w0rld"`

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
- **Parameters:** `pattern` (string — RE2 regular expression)
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
- **Parameters:** `pattern` (string — RE2 regular expression)
- **Returns:** array of strings
- **Examples:**
  ```bloblang
  "foo123bar456".re_find_all("[0-9]+")   # ["123", "456"]
  "hello".re_find_all("[0-9]+")          # []
  ```

### `.re_replace_all(pattern, replacement)`

Replace all matches of a regular expression with a replacement string. The replacement string supports RE2 expansion syntax: `$0` for the full match, `$1`/`$2` for numbered capture groups, and `${name}` for named capture groups. Use `$$` for a literal `$`.

- **Receiver:** string
- **Parameters:** `pattern` (string — RE2 regular expression), `replacement` (string — with RE2 expansion)
- **Returns:** string
- **Examples:**
  ```bloblang
  "foo 123 bar 456".re_replace_all("[0-9]+", "N")           # "foo N bar N"
  "John Smith".re_replace_all("(\\w+) (\\w+)", "$2, $1")    # "Smith, John"
  "2024-03-01".re_replace_all("(?P<y>\\d{4})-(?P<m>\\d{2})-(?P<d>\\d{2})", "${d}/${m}/${y}")
  # "01/03/2024"
  ```

---

## 13.6 Array Methods

### `.filter(fn)`

Return a new array containing only elements for which the lambda returns `true`. The lambda must return a boolean — non-boolean return values (including void) are an error.

- **Receiver:** array
- **Parameters:** `fn` — lambda (one parameter: element → bool)
- **Returns:** array
- **Examples:**
  ```bloblang
  [1, 2, 3, 4].filter(x -> x > 2)     # [3, 4]
  [1, -2, 3].filter(x -> x > 0)        # [1, 3]
  ```

### `.map(fn)`

Transform each element of an array. Returns a new array.

- The lambda must return a value for every element — void is an error
- If the lambda returns `deleted()`, the element is omitted from the result

- **Receiver:** array
- **Parameters:** `fn` — lambda (one parameter: element → any)
- **Returns:** array
- **Examples:**
  ```bloblang
  [1, 2, 3].map(x -> x * 2)                              # [2, 4, 6]
  [1, -2, 3].map(x -> if x > 0 { x } else { deleted() }) # [1, 3]
  [1, -2, 3].map(x -> if x > 0 { x * 10 })               # ERROR: void when x <= 0
  ```
- **See:** Section 4.1 for void and deleted() behavior in lambda returns

### `.sort()`

Sort an array in ascending order. Sort is **stable** (equal elements preserve relative order). All elements must belong to the same sortable type family — mixing across families is an error.

**Sortable type families:**
- **Numeric** (int32, int64, uint32, uint64, float32, float64): promoted before comparison using standard rules (Section 2.3)
- **String**: compared lexicographically by Unicode codepoint
- **Timestamp**: compared chronologically

Bool, null, bytes, array, and object are not sortable — an array containing these types will error. Cross-family mixing (e.g., numbers with strings) is also an error.

**NaN ordering:** NaN values sort after all other numeric values (including Infinity). This follows the total ordering convention used by Go and Java rather than IEEE 754 comparison semantics. Multiple NaN values maintain their relative order (stable sort).

- **Receiver:** array
- **Returns:** array
- **Examples:**
  ```bloblang
  [3, 1, 2].sort()           # [1, 2, 3]
  ["b", "a", "c"].sort()     # ["a", "b", "c"]
  [3, 1.5, 2].sort()         # [1.5, 2, 3] (int64 promoted to float64)
  [].sort()                  # [] (empty array, trivially valid)
  [1, "a", true].sort()      # ERROR: cannot sort mixed type families
  ```

### `.sort_by(fn)`

Sort an array using a key function. Sort is **stable**. The lambda extracts a sort key from each element; keys are compared using the same rules as `.sort()`.

- **Receiver:** array
- **Parameters:** `fn` — lambda (one parameter: element → comparable value)
- **Returns:** array
- **Examples:**
  ```bloblang
  [{"name": "Bob"}, {"name": "Alice"}].sort_by(x -> x.name)
  # [{"name": "Alice"}, {"name": "Bob"}]

  [3, -1, 2].sort_by(x -> x.abs())   # [-1, 2, 3] (sorted by absolute value)
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
  [1, [], 2].flatten()                 # [1, 2] (empty array spliced as zero elements)
  ```

### `.unique(fn?)`

Remove duplicate elements, preserving the first occurrence of each value. When `fn` is provided, the lambda extracts a comparison key from each element — elements are considered duplicates if their keys are equal. When `fn` is omitted, elements are compared directly by equality. Comparison uses equality semantics (Section 2.3), except that all NaN values are considered equal (consistent with `.sort()`'s total ordering). At most one NaN is retained.

- **Receiver:** array
- **Parameters:** `fn` — lambda (one parameter: element → any), optional. When provided, extracts a comparison key from each element.
- **Returns:** array
- **Examples:**
  ```bloblang
  [1, 2, 2, 3, 1].unique()                    # [1, 2, 3]
  ["a", "b", "a"].unique()                    # ["a", "b"]
  [{"id": 1, "v": "a"}, {"id": 1, "v": "b"}].unique(x -> x.id)  # [{"id": 1, "v": "a"}]
  ["hello", "HELLO", "world"].unique(x -> x.lowercase())          # ["hello", "world"]
  ```

### `.without_index(index)`

Return a new array with the element at the given index removed. Remaining elements shift down. Negative indices count from the end. Out-of-bounds indices are an error.

- **Receiver:** array
- **Parameters:** `index` (int64)
- **Returns:** array
- **Examples:**
  ```bloblang
  [10, 20, 30].without_index(1)     # [10, 30]
  [10, 20, 30].without_index(0)     # [20, 30]
  [10, 20, 30].without_index(-1)    # [10, 20]
  [10, 20, 30].without_index(5)     # ERROR: index out of bounds
  ```
- **Design note:** Unlike `.without()` on objects (which accepts an array of keys), `.without_index()` takes a single index. Chaining multiple `.without_index()` calls is error-prone because indices shift after each removal. To remove multiple elements, use `.filter()` or `.enumerate().filter(...).map(e -> e.value)` instead.

### `.enumerate()`

Convert an array to an array of `{"index": i, "value": v}` objects.

- **Receiver:** array
- **Returns:** array of objects
- **Example:**
  ```bloblang
  ["a", "b", "c"].enumerate()
  # [{"index": 0, "value": "a"}, {"index": 1, "value": "b"}, {"index": 2, "value": "c"}]
  ```

### `.any(fn)`

Test if any element satisfies the predicate. Returns `false` for empty arrays. **Must** short-circuit on first `true` — subsequent elements are not evaluated (this is a required semantic, not an optimization).

- **Receiver:** array
- **Parameters:** `fn` — lambda (one parameter: element → bool)
- **Returns:** bool
- **Examples:**
  ```bloblang
  [1, 2, 3].any(x -> x > 2)      # true
  [1, 2, 3].any(x -> x > 5)      # false
  [].any(x -> true)               # false
  ```

### `.all(fn)`

Test if all elements satisfy the predicate. Returns `true` for empty arrays. **Must** short-circuit on first `false` — subsequent elements are not evaluated (this is a required semantic, not an optimization).

- **Receiver:** array
- **Parameters:** `fn` — lambda (one parameter: element → bool)
- **Returns:** bool
- **Examples:**
  ```bloblang
  [1, 2, 3].all(x -> x > 0)      # true
  [1, 2, 3].all(x -> x > 2)      # false
  [].all(x -> false)              # true
  ```

### `.find(fn)`

Return the first element that satisfies the predicate. **Must** short-circuit — subsequent elements are not evaluated after the first match (this is a required semantic, not an optimization). If no element matches, produces **void** — use `.or()` to provide a fallback.

**Design note:** `.find()` produces void on no match (rather than returning `null` or erroring) because `null` could be a legitimate array element, and "no match" is genuinely the absence of a value — exactly what void represents. This is consistent with if-without-else and match-without-`_`, which also produce void when no branch yields a value. In contrast, `.index_of()` returns `-1` because indices are non-negative integers, making `-1` an unambiguous "not found" sentinel.

- **Receiver:** array
- **Parameters:** `fn` — lambda (one parameter: element → bool)
- **Returns:** any (the element), or void if no element matches
- **Examples:**
  ```bloblang
  [1, 2, 3].find(x -> x > 1)                # 2
  [1, 2, 3].find(x -> x > 5)                # void (no match)
  [1, 2, 3].find(x -> x > 5).or(0)          # 0 (void rescued)
  output.val = [1, 2].find(x -> x > 5)      # assignment skipped (void)
  $x = [1, 2].find(x -> x > 5)              # ERROR: void in variable declaration
  $x = [1, 2].find(x -> x > 5).or(0)        # 0
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

Sum all numeric elements. All elements must be numeric — non-numeric elements are an error. Elements are pairwise promoted using the same rules as `+` (Section 2.3) — e.g., mixing int64 and float64 promotes all to float64. Returns `0` (int64) for empty arrays.

- **Receiver:** array of numeric values
- **Returns:** numeric (promoted type)
- **Examples:**
  ```bloblang
  [1, 2, 3].sum()        # 6 (int64)
  [1.5, 2.5].sum()       # 4.0 (float64)
  [1, 1.5, 2].sum()      # 4.5 (float64: int64 promoted to float64)
  [].sum()                # 0 (int64)
  ```

### `.min()`

Return the minimum element of an array. All elements must belong to the same sortable type family (same rules as `.sort()`). Empty arrays are an error.

- **Receiver:** array of sortable values (numeric, string, or timestamp — not mixed)
- **Returns:** same type as elements (promoted type for mixed numeric subtypes)
- **Examples:**
  ```bloblang
  [3, 1, 2].min()              # 1 (int64)
  [3.5, 1.2, 2.8].min()       # 1.2 (float64)
  [3, 1.5, 2].min()            # 1.5 (float64: int64 promoted)
  ["c", "a", "b"].min()        # "a"
  [].min()                     # ERROR: empty array
  ```

### `.max()`

Return the maximum element of an array. All elements must belong to the same sortable type family (same rules as `.sort()`). Empty arrays are an error.

- **Receiver:** array of sortable values (numeric, string, or timestamp — not mixed)
- **Returns:** same type as elements (promoted type for mixed numeric subtypes)
- **Examples:**
  ```bloblang
  [3, 1, 2].max()              # 3 (int64)
  [3.5, 1.2, 2.8].max()       # 3.5 (float64)
  [3, 1.5, 2].max()            # 3.0 (float64: int64 promoted)
  ["c", "a", "b"].max()        # "c"
  [].max()                     # ERROR: empty array
  ```

### `.fold(initial, fn)`

Reduce an array to a single value by applying an accumulator function to each element. The lambda receives the running tally and the current element, and returns the new tally.

- **Receiver:** array
- **Parameters:** `initial` (any — starting value), `fn` — lambda (two parameters: tally, element → any)
- **Returns:** any (the final tally)
- **Examples:**
  ```bloblang
  [1, 2, 3].fold(0, (tally, x) -> tally + x)          # 6
  [1, 2, 3].fold(1, (tally, x) -> tally * x)          # 6
  ["a", "b"].fold("", (tally, x) -> tally + x + ",")  # "a,b,"
  ```

### `.collect()`

Convert an array of `{"key": k, "value": v}` objects back into an object. Last value wins on duplicate keys.

- **Receiver:** array of objects (each must have `"key"` (string) and `"value"` (any) fields; extra fields are ignored)
- **Returns:** object
- **Errors:** if any element is not an object, is missing `"key"` or `"value"` fields, or if `"key"` is not a string
- **Examples:**
  ```bloblang
  [{"key": "a", "value": 1}, {"key": "b", "value": 2}].collect()     # {"a": 1, "b": 2}
  [{"key": "a", "value": 1}, {"key": "a", "value": 2}].collect()     # {"a": 2} (last value wins)
  [{"key": "a", "value": 1, "extra": true}].collect()                 # {"a": 1} (extra fields ignored)
  [{"key": "a", "value": 1}, {"bad": true}].collect()                 # ERROR: element missing "key"/"value" fields
  ```
- **Note:** `.collect()` returns an object, and object key ordering is not preserved (Section 2.3). Sorting entries before `.collect()` (e.g., `.iter().sort_by(e -> e.key).collect()`) does not produce an object with ordered iteration — the sort order is lost. JSON serialization is deterministic (keys sorted lexicographically by `.format_json()` and `.string()`), but iteration order via `.iter()`, `.keys()`, and `.values()` is not guaranteed.

---

## 13.7 Object Methods

### `.iter()`

Convert an object to an array of `{"key": k, "value": v}` objects. Order is not guaranteed.

- **Receiver:** object
- **Returns:** array of objects (each with string field `"key"` and any-typed field `"value"`)
- **Examples:**
  ```bloblang
  {"a": 1, "b": 2}.iter()
  # [{"key": "a", "value": 1}, {"key": "b", "value": 2}] (order not guaranteed)

  # Extract keys
  {"a": 1, "b": 2}.iter().map(e -> e.key)         # ["a", "b"] (order not guaranteed)

  # Extract values
  {"a": 1, "b": 2}.iter().map(e -> e.value)        # [1, 2] (order not guaranteed)

  # Complex transforms — use iter/collect
  {"a": 1, "b": 2}.iter().map(e -> {"key": e.key.uppercase(), "value": e.value * 10}).collect()
  # {"A": 10, "B": 20}

  # Filter entries
  {"a": 1, "b": 2, "c": 3}.iter().filter(e -> e.value > 1).collect()
  # {"b": 2, "c": 3}
  ```
- **Note:** For common transforms, prefer the dedicated methods `.map_values()`, `.map_keys()`, `.map_entries()`, and `.filter_entries()` — they are more concise. Use `.iter()`/`.collect()` for complex transforms that don't fit those patterns.

### `.keys()`

Return the keys of an object as an array of strings. Order is not guaranteed.

- **Receiver:** object
- **Returns:** array of strings
- **Examples:**
  ```bloblang
  {"a": 1, "b": 2}.keys()           # ["a", "b"] (order not guaranteed)
  {}.keys()                          # []
  ```

### `.values()`

Return the values of an object as an array. Order is not guaranteed, but corresponds to the same order as `.keys()` within a single call.

- **Receiver:** object
- **Returns:** array of any
- **Examples:**
  ```bloblang
  {"a": 1, "b": 2}.values()         # [1, 2] (order not guaranteed)
  {}.values()                        # []
  ```

### `.has_key(key)`

Check if an object contains the given key.

- **Receiver:** object
- **Parameters:** `key` (string)
- **Returns:** bool
- **Examples:**
  ```bloblang
  {"a": 1, "b": 2}.has_key("a")     # true
  {"a": 1, "b": 2}.has_key("c")     # false
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

### `.map_values(fn)`

Transform the values of an object, keeping keys unchanged. Returns a new object.

- The lambda must return a value for every entry — void is an error
- If the lambda returns `deleted()`, the entry is omitted from the result

- **Receiver:** object
- **Parameters:** `fn` — lambda (one parameter: value → any)
- **Returns:** object
- **Examples:**
  ```bloblang
  {"a": 1, "b": 2}.map_values(v -> v * 10)              # {"a": 10, "b": 20}
  {"a": "hello", "b": "world"}.map_values(v -> v.uppercase())  # {"a": "HELLO", "b": "WORLD"}
  {"a": 1, "b": -2}.map_values(v -> if v > 0 { v } else { deleted() })  # {"a": 1}
  ```

### `.map_keys(fn)`

Transform the keys of an object, keeping values unchanged. Returns a new object. If multiple keys map to the same new key, last value wins.

- The lambda must return a value for every entry — void is an error
- If the lambda returns `deleted()`, the entry is omitted from the result (the `deleted()` check runs before the type check, so this does not trigger a type error)
- Otherwise, the lambda must return a string — non-string return values are an error

- **Receiver:** object
- **Parameters:** `fn` — lambda (one parameter: key (string) → string | deleted)
- **Returns:** object
- **Examples:**
  ```bloblang
  {"a": 1, "b": 2}.map_keys(k -> k.uppercase())         # {"A": 1, "B": 2}
  {"user_name": "Alice"}.map_keys(k -> k.replace_all("_", "-"))  # {"user-name": "Alice"}
  ```

### `.map_entries(fn)`

Transform both keys and values of an object. The lambda receives two parameters (key, value) and must return an object with `"key"` (string) and `"value"` (any) fields. If multiple entries produce the same key, last value wins. **Errors:** lambda returns a non-object, returned object is missing `"key"` or `"value"` field, or `"key"` is not a string.

- The lambda must return a value for every entry — void is an error
- If the lambda returns `deleted()`, the entry is omitted from the result

- **Receiver:** object
- **Parameters:** `fn` — lambda (two parameters: key (string), value (any) → object with `"key"` and `"value"` fields)
- **Returns:** object
- **Examples:**
  ```bloblang
  {"a": 1, "b": 2}.map_entries((k, v) -> {"key": k.uppercase(), "value": v * 10})
  # {"A": 10, "B": 20}
  ```

### `.filter_entries(fn)`

Filter entries of an object. The lambda receives two parameters (key, value) and must return a boolean. Returns a new object containing only entries for which the lambda returns `true`.

- **Receiver:** object
- **Parameters:** `fn` — lambda (two parameters: key (string), value (any) → bool)
- **Returns:** object
- **Examples:**
  ```bloblang
  {"a": 1, "b": 2, "c": 3}.filter_entries((k, v) -> v > 1)    # {"b": 2, "c": 3}
  {"aa": 1, "b": 2}.filter_entries((k, v) -> k.length() > 1)   # {"aa": 1}
  ```

---

## 13.8 Numeric Methods

### `.abs()`

Return the absolute value. For signed integer types, errors if the result overflows (the most-negative value of each signed type has no positive counterpart). For unsigned types, returns the value unchanged.

- **Receiver:** any numeric type
- **Returns:** same type as receiver
- **Examples:**
  ```bloblang
  (-5).abs()      # 5 (int64)
  3.14.abs()      # 3.14 (float64)
  (-3.14).abs()   # 3.14 (float64)
  (-2147483648).int32().abs()  # ERROR: int32 overflow
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

### `.round(n = 0)`

Round a float to `n` decimal places using **half-even rounding** (banker's rounding, IEEE 754 default). Defaults to `0` (round to nearest integer). Negative `n` rounds to powers of 10: `-1` rounds to nearest 10, `-2` to nearest 100, etc.

- **Receiver:** float32, float64
- **Parameters:** `n` (int64, default `0` — number of decimal places; negative values round to powers of 10)
- **Returns:** same float type as receiver
- **Examples:**
  ```bloblang
  3.7.round()        # 4.0 (default: round to nearest integer)
  2.5.round()        # 2.0 (half-even: rounds to nearest even)
  3.456.round(2)     # 3.46
  2.5.round(0)       # 2.0 (half-even: rounds to nearest even)
  3.5.round(0)       # 4.0 (half-even: rounds to nearest even)
  1234.0.round(-2)   # 1200.0 (round to nearest hundred)
  1250.0.round(-2)   # 1200.0 (half-even: rounds to nearest even hundred)
  ```

---

## 13.9 Time Methods

### `.ts_parse(format = "%Y-%m-%dT%H:%M:%S%f%z")`

Parse a string into a timestamp using the given format string. Defaults to RFC 3339 format when no format is specified.

- **Receiver:** string
- **Parameters:** `format` (string, default `"%Y-%m-%dT%H:%M:%S%f%z"` — strftime format)
- **Returns:** timestamp
- **Errors:** if the string does not match the format, or if the format string contains unrecognized directives
- **Examples:**
  ```bloblang
  "2024-03-01T12:00:00Z".ts_parse()                    # RFC 3339 (default format)
  "2024-03-01T12:00:00.123Z".ts_parse()                # RFC 3339 with fractional seconds
  "2024-03-01T12:00:00+05:30".ts_parse()               # RFC 3339 with offset
  "2024-03-01".ts_parse("%Y-%m-%d")                    # explicit format
  ```

**Required strftime directives:** All implementations must support the following subset. Additional directives are implementation-defined.

| Directive | Meaning | Example |
|-----------|---------|---------|
| `%Y` | 4-digit year | `2024` |
| `%m` | Month (01–12) | `03` |
| `%d` | Day of month (01–31) | `01` |
| `%H` | Hour, 24-hour (00–23) | `14` |
| `%M` | Minute (00–59) | `30` |
| `%S` | Second (00–59) | `05` |
| `%f` | Fractional seconds (nanosecond precision, optional — see note) | `.123456789` |
| `%z` | UTC offset or `Z` (see note) | `Z`, `+05:30` |
| `%Z` | Timezone name (IANA or abbreviation) | `UTC`, `America/New_York` |
| `%a` | Abbreviated weekday name | `Mon` |
| `%A` | Full weekday name | `Monday` |
| `%b` | Abbreviated month name | `Jan` |
| `%B` | Full month name | `January` |
| `%p` | AM/PM (uppercase) | `PM` |
| `%I` | Hour, 12-hour (01–12) | `02` |
| `%j` | Day of year (001–366) | `061` |
| `%%` | Literal `%` | `%` |

**`%f` semantics:** `%f` is not part of POSIX strftime but is widely supported for sub-second precision. **Parsing:** `%f` is optional — it consumes the leading `.` and 1–9 fractional digits if present, padding to nanoseconds (e.g., `.123` → 123000000 ns). If no `.` follows the seconds, `%f` matches zero characters and contributes zero fractional seconds. This allows a single format string like `"%Y-%m-%dT%H:%M:%S%f%z"` to parse both `"2024-03-01T12:00:00Z"` and `"2024-03-01T12:00:00.123Z"`. **Formatting:** `%f` emits the shortest representation that retains precision — trailing zeros are removed, and the directive is omitted entirely (including the `.`) when fractional seconds are zero. Examples: 123456789 ns → `.123456789`, 123000000 ns → `.123`, 0 ns → (empty). This differs from Python's `%f`, which always emits exactly 6 digits.

**`%z` semantics:** **Parsing:** `%z` accepts both `Z` (UTC) and UTC offsets in the forms `+HH:MM`, `-HH:MM`, `+HHMM`, or `-HHMM`. **Formatting:** `%z` emits `Z` for UTC (the shortest RFC 3339 representation) and `±HH:MM` for all other offsets.

### `.ts_format(format = "%Y-%m-%dT%H:%M:%S%f%z")`

Format a timestamp as a string using the given format string. Defaults to RFC 3339 format when no format is specified. Supports the same required directives as `.ts_parse()`.

- **Receiver:** timestamp
- **Parameters:** `format` (string, default `"%Y-%m-%dT%H:%M:%S%f%z"` — strftime format)
- **Returns:** string
- **Examples:**
  ```bloblang
  now().ts_format()              # "2024-03-01T12:00:00Z" (RFC 3339, default format)
  now().ts_format("%Y-%m-%d")    # "2024-03-01"
  ```
- **Note:** `.ts_format()` with default arguments produces the same output as `.string()` on a timestamp. Both use RFC 3339 with shortest-precision fractional seconds.

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

### `.ts_from_unix()`

Convert a Unix timestamp (seconds since epoch) to a timestamp. Integer receivers produce second-precision timestamps. Float receivers provide sub-second precision — the fractional part is interpreted as fractions of a second. **Precision note:** float64 has ~15-17 significant decimal digits. For current Unix timestamps (~10 integer digits), this leaves ~6-7 fractional digits of precision — sufficient for microseconds but not nanoseconds. For full nanosecond precision, use `.ts_from_unix_nano()` with an int64 value instead.

- **Receiver:** any numeric type (integers are widened to int64; float32 is widened to float64). uint64 values exceeding int64 range are a runtime error, consistent with signed+unsigned promotion rules (Section 2.3)
- **Returns:** timestamp
- **Examples:**
  ```bloblang
  1709500000.ts_from_unix()       # timestamp: 2024-03-03T...Z (second precision)
  1709500000.5.ts_from_unix()     # timestamp: 2024-03-03T...500000000Z (sub-second)
  1709500000.123456.ts_from_unix()  # ~microsecond precision (float64 limit)
  ```

### `.ts_from_unix_milli()`

Convert a Unix timestamp in milliseconds to a timestamp. Provides exact millisecond precision using integer arithmetic.

- **Receiver:** int64
- **Returns:** timestamp
- **Examples:**
  ```bloblang
  1709500000000.ts_from_unix_milli()       # same as 1709500000.ts_from_unix()
  1709500000123.ts_from_unix_milli()       # exact millisecond precision
  ```

### `.ts_from_unix_micro()`

Convert a Unix timestamp in microseconds to a timestamp. Provides exact microsecond precision using integer arithmetic.

- **Receiver:** int64
- **Returns:** timestamp
- **Examples:**
  ```bloblang
  1709500000000000.ts_from_unix_micro()       # same as 1709500000.ts_from_unix()
  1709500000123456.ts_from_unix_micro()       # exact microsecond precision
  ```

### `.ts_from_unix_nano()`

Convert a Unix timestamp in nanoseconds to a timestamp. Provides exact nanosecond precision using integer arithmetic. This is the lossless round-trip counterpart to `.ts_unix_nano()`.

- **Receiver:** int64
- **Returns:** timestamp
- **Examples:**
  ```bloblang
  1709500000000000000.ts_from_unix_nano()          # same as 1709500000.ts_from_unix()
  1709500000123456789.ts_from_unix_nano()          # exact nanosecond precision
  now().ts_unix_nano().ts_from_unix_nano()          # lossless round-trip
  ```

### `.ts_add(nanos)`

Add a duration in nanoseconds to a timestamp. Negative values subtract. Use `second()` to avoid raw nanosecond constants. If the resulting timestamp would be outside the representable range, a runtime error is thrown (consistent with integer overflow rules in Section 2.3).

- **Receiver:** timestamp
- **Parameters:** `nanos` (int64 — duration in nanoseconds)
- **Returns:** timestamp
- **Examples:**
  ```bloblang
  now().ts_add(second())              # 1 second later
  now().ts_add(second() * -60)        # 1 minute ago
  now().ts_add(second() * 86400)      # 1 day later
  ```

---

## 13.10 Error Handling Methods

### `.catch(fn)`

Handle errors. Called only when the expression to its left produces an error. If the expression succeeds, `.catch()` returns its value unchanged. The error object has a single field: `.what` (string, the error message).

- **Receiver:** any expression (catches errors from the left-hand side)
- **Parameters:** `fn` — lambda (one parameter: error object → any)
- **Returns:** any (either the original value or the lambda's result)
- **`deleted()` and void from handler:** The handler lambda may return `deleted()` or void — these flow to the calling context with normal semantics. For example, in a field assignment, `deleted()` removes the field and void skips the assignment. This mirrors `.or()`, which also supports `deleted()` (Section 8.3).
- **Examples:**
  ```bloblang
  input.date.ts_parse("%Y-%m-%d").catch(err -> null)
  input.date.ts_parse("%Y-%m-%d").catch(err -> throw("parse failed: " + err.what))

  # deleted() from handler — removes field on error
  output.date = input.raw_date.ts_parse("%Y-%m-%d").catch(err -> deleted())

  # void from handler — skips assignment on error
  output.date = input.raw_date.ts_parse("%Y-%m-%d").catch(err -> if input.strict { throw(err.what) })
  ```
- **See:** Section 8.2

### `.or(default)`

Provide a default value for null, void, or `deleted()`. Takes exactly one argument — zero or multiple arguments are a compile-time error. Uses **short-circuit evaluation** — the argument is only evaluated if the receiver is null, void, or `deleted()`. Along with `.catch()`, this is one of only two methods that can be called on void or `deleted()`. `.catch()` passes them through unchanged; `.or()` actively rescues them.

- **Receiver:** any expression (including void and `deleted()`)
- **Parameters:** `default` (any expression, lazily evaluated; exactly one argument required)
- **Returns:** any (either the original value, or the default if receiver was null/void/deleted)
- **Examples:**
  ```bloblang
  input.name.or("Anonymous")
  input.name.or(throw("name is required"))  # throw() only evaluated if name is null
  (if false { "hello" }).or("world")        # "world" (void rescued)
  (match input.x { "a" => 1 }).or(0)       # 0 if no case matched (void rescued)
  some_map(input.value).or("fallback")      # "fallback" if map returned deleted()
  ```
- **See:** Section 8.3; Section 8.6 for how `.or()` and `.catch()` compose when chained together

### `.not_null(message = "unexpected null value")`

Assert that a value is not null. Returns the value unchanged if it is not null; throws an error with the given message if it is null. This is a concise alternative to `.or(throw("message"))` for null validation.

- **Receiver:** any type (including null)
- **Parameters:** `message` (string, default `"unexpected null value"`)
- **Returns:** the receiver value, unchanged (if not null)
- **Errors:** if the receiver is null, throws an error with the given message
- **Examples:**
  ```bloblang
  "hello".not_null()                          # "hello" (not null, returned as-is)
  42.not_null()                               # 42
  null.not_null()                             # ERROR: unexpected null value
  null.not_null("name is required")           # ERROR: name is required
  input.name.not_null("name is required")     # value if present, error if null
  ```
- **Note:** `.not_null()` is a regular method — it cannot be called on void or `deleted()` (those error before reaching the method). To validate against null, void, and `deleted()` simultaneously, use `.or(throw("message"))` instead (Section 8.3).

---

## 13.11 Parsing Methods

### `.parse_json()`

Parse a JSON string into a value. Errors if the string is not valid JSON.

**Numeric type mapping:** JSON numbers without a decimal point or exponent are parsed as int64 if the value fits in int64 range; if it exceeds int64 range, the value is parsed as float64 (which may lose precision for very large integers). JSON numbers with a decimal point or exponent are parsed as float64 (matching Bloblang float literal rules). **Note:** Large unsigned integers (between 2^63 and 2^64-1) exceed int64 range and are parsed as float64, which loses precision. To handle these values losslessly, represent them as JSON strings and convert explicitly: `"18446744073709551615".uint64()`.

- **Receiver:** string, bytes (bytes are interpreted as UTF-8-encoded JSON; errors if bytes are not valid UTF-8)
- **Returns:** any (the parsed value)
- **Examples:**
  ```bloblang
  `{"name":"Alice"}`.parse_json()    # {"name": "Alice"}
  `[1,2,3]`.parse_json()            # [1, 2, 3] (int64 elements)
  `"hello"`.parse_json()            # "hello"
  `42`.parse_json()                 # 42 (int64: no decimal point)
  `3.14`.parse_json()               # 3.14 (float64: has decimal point)
  `1e3`.parse_json()                # 1000.0 (float64: has exponent)
  ```

### `.format_json(indent = "", no_indent = false, escape_html = true)`

Serialize a value to a JSON string. Object keys are sorted lexicographically by Unicode codepoint value (consistent with string comparison semantics in Section 2.3). Timestamp values are formatted as RFC 3339 strings (Section 2.3). **Note:** Since object key ordering is not preserved (Section 2.3) and keys are sorted on output, `.parse_json().format_json()` may produce different key ordering than the original JSON string.

- **Receiver:** any type (except bytes)
- **Parameters:**
  - `indent` (string, default `""`) — Indentation string. When non-empty, each element in a JSON object or array begins on a new line, indented with one or more copies of this string according to nesting depth.
  - `no_indent` (bool, default `false`) — Disable indentation entirely, overriding `indent`. Produces compact output with no extra whitespace.
  - `escape_html` (bool, default `true`) — Escape HTML-sensitive characters (`<`, `>`, `&`) as Unicode escape sequences in strings.
- **Returns:** string
- **Errors:** if the value is or contains a bytes value (at any nesting depth). Bytes have no implicit JSON serialization — use `.encode("base64")` or `.encode("hex")` before serializing. NaN and Infinity float values also error (not representable in JSON).
- **Numeric serialization:**
  - Integer types (int32, int64, uint32, uint64): serialized as JSON integers (no decimal point, no quotes). Large uint64 values (> 2^53) are serialized as-is — the JSON spec imposes no range limit, though consumers using float64 may lose precision.
  - Float types (float32, float64): serialized as any shortest decimal representation that round-trips exactly, always including either a decimal point or exponent to distinguish from integer serialization. This matches `.string()` behavior (including the cross-implementation note) and ensures that `.format_json()` → `.parse_json()` preserves the float type. Exponent notation is permitted (e.g., `1e+06`).
- **Examples:**
  ```bloblang
  {"name": "Alice"}.format_json()                          # `{"name":"Alice"}`
  [1, 2, 3].format_json()                                 # `[1,2,3]`
  {"time": now()}.format_json()                            # `{"time":"2024-03-01T12:00:00Z"}`
  {"name": "Alice", "age": 30}.format_json(indent: "  ")   # pretty-printed with 2-space indent
  {"name": "Alice"}.format_json(indent: "\t")              # pretty-printed with tab indent
  {"html": "<b>hi</b>"}.format_json()                      # `{"html":"\u003cb\u003ehi\u003c/b\u003e"}`
  {"html": "<b>hi</b>"}.format_json(escape_html: false)    # `{"html":"<b>hi</b>"}`
  ```

### `.encode(scheme)`

Encode a value into a string using the specified encoding scheme.

- **Receiver:** string, bytes
- **Parameters:** `scheme` — string, one of: `"base64"`, `"base64url"`, `"base64rawurl"`, `"hex"`
- **Returns:** string
- **Schemes:**
  - `"base64"` — standard Base64 with padding (RFC 4648)
  - `"base64url"` — URL-safe Base64 with padding (RFC 4648)
  - `"base64rawurl"` — URL-safe Base64 without padding (RFC 4648)
  - `"hex"` — lowercase hexadecimal
- **Examples:**
  ```bloblang
  "hello".bytes().encode("base64")       # "aGVsbG8="
  "hello".bytes().encode("hex")          # "68656c6c6f"
  "hello".encode("base64")              # "aGVsbG8=" (string treated as UTF-8 bytes)
  ```

### `.decode(scheme)`

Decode a string from the specified encoding scheme into bytes.

- **Receiver:** string
- **Parameters:** `scheme` — string, one of: `"base64"`, `"base64url"`, `"base64rawurl"`, `"hex"`
- **Returns:** bytes
- **Errors:** if the input is not valid for the specified scheme
- **Examples:**
  ```bloblang
  "aGVsbG8=".decode("base64")            # bytes("hello")
  "68656c6c6f".decode("hex")             # bytes("hello")
  "aGVsbG8=".decode("base64").string()   # "hello"
  ```
