# 3. Type System

Bloblang is dynamically typed with the following runtime types:
- **Number**: 64-bit integer or floating-point
- **String**: UTF-8 encoded character sequence
- **Boolean**: `true` or `false`
- **Null**: Represents absence of value
- **Bytes**: Raw byte sequence (via `content()` function)
- **Array**: Ordered collection of heterogeneous values (supports indexing with `[n]` and `[-n]`)
- **Object**: Unordered collection of key-value pairs

Type introspection via `.type()` method returns: `"number"`, `"string"`, `"bool"`, `"null"`, `"bytes"`, `"array"`, `"object"`.

## Array Type Details

Arrays are zero-indexed ordered collections that support:
- **Positive indexing**: `array[0]` (first element), `array[1]` (second element), etc.
- **Negative indexing**: `array[-1]` (last element), `array[-2]` (second-to-last), etc.
- **Method operations**: `.length()`, `.filter()`, `.map_each()`, `.sort()`, etc.
- **Heterogeneous values**: Elements can be of different types

Example:
```bloblang
$items = [1, "text", true, null, {"key": "value"}]
output.first = $items[0]      # 1
output.last = $items[-1]      # {"key": "value"}
output.count = $items.length() # 5
```
