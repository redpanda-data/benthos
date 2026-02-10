# 3. Type System

Bloblang is dynamically typed with the following runtime types:
- **Number**: 64-bit integer or floating-point
- **String**: UTF-8 encoded character sequence
- **Boolean**: `true` or `false`
- **Null**: Represents absence of value
- **Bytes**: Raw byte sequence (via `content()` function)
- **Array**: Ordered collection of heterogeneous values
- **Object**: Unordered collection of key-value pairs

Type introspection via `.type()` method returns: `"number"`, `"string"`, `"bool"`, `"null"`, `"bytes"`, `"array"`, `"object"`.
