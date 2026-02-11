# 3. Type System

Bloblang is dynamically typed with the following runtime types:
- **Number**: 64-bit integer or floating-point
- **String**: UTF-8 encoded character sequence (supports indexing with `[n]` and `[-n]`)
- **Boolean**: `true` or `false`
- **Null**: Represents absence of value
- **Bytes**: Raw byte sequence (via `content()` function, supports indexing with `[n]` and `[-n]`)
- **Array**: Ordered collection of heterogeneous values (supports indexing with `[n]` and `[-n]`)
- **Object**: Unordered collection of key-value pairs

Type introspection via `.type()` method returns: `"number"`, `"string"`, `"bool"`, `"null"`, `"bytes"`, `"array"`, `"object"`.

## Indexable Type Details

### Arrays

Arrays are zero-indexed ordered collections that support:
- **Positive indexing**: `array[0]` (first element), `array[1]` (second element), etc.
- **Negative indexing**: `array[-1]` (last element), `array[-2]` (second-to-last), etc.
- **Method operations**: `.length()`, `.filter()`, `.map_each()`, `.sort()`, etc.
- **Heterogeneous values**: Elements can be of different types

Example:
```bloblang
$items = [1, "text", true, null, {"key": "value"}]
output.first = $items[0]       # 1
output.last = $items[-1]       # {"key": "value"}
output.count = $items.length() # 5
```

### Strings

Strings support byte-position indexing:
- **Positive indexing**: `string[0]` (first byte), `string[1]` (second byte), etc.
- **Negative indexing**: `string[-1]` (last byte), `string[-2]` (second-to-last), etc.
- **Returns**: Single-character string at the byte position
- **UTF-8 caveat**: Indexing is by byte position, not character position. Multi-byte UTF-8 characters may be split if indexed in the middle.

Example:
```bloblang
$text = "hello"
output.first_char = $text[0]   # "h"
output.last_char = $text[-1]   # "o"
output.length = $text.length() # 5

$emoji = "👋"  # 4-byte UTF-8 character
output.first_byte = $emoji[0]  # Returns first byte (may be invalid UTF-8)
```

### Bytes

Bytes support indexing that returns numeric byte values:
- **Positive indexing**: `bytes[0]` (first byte), `bytes[1]` (second byte), etc.
- **Negative indexing**: `bytes[-1]` (last byte), `bytes[-2]` (second-to-last), etc.
- **Returns**: Byte value as a number (0-255)

Example:
```bloblang
$data = content()  # Raw message bytes
output.first_byte = $data[0]     # Number 0-255
output.last_byte = $data[-1]     # Number 0-255
output.size = $data.length()     # Byte count
```
