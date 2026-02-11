# 2. Lexical Structure

## 2.1 Tokens

- **Identifiers**: `[a-zA-Z_][a-zA-Z0-9_]*`
- **Keywords**: `output`, `input`, `if`, `else`, `match`, `as`, `map`, `deleted`, `import`, `from`
- **Operators**: `.`, `?.`, `=`, `+`, `-`, `*`, `/`, `%`, `!`, `>`, `>=`, `==`, `<`, `<=`, `&&`, `||`, `=>`, `->`
- **Delimiters**: `(`, `)`, `{`, `}`, `[`, `]`, `?[`, `,`, `:`
- **Literals**: Numbers, strings, booleans, null, arrays, objects
- **Comments**: `#` to end-of-line

## 2.2 Identifiers

Field names follow dot-separated path notation. Special characters (spaces, dots, symbols) require double-quote escaping:
```
output.user.name          # Standard identifier
output."foo.bar".baz      # Quoted identifier with dot
output."field with spaces" # Quoted identifier with spaces
```

## 2.3 Literals

**Numbers**: Integer or floating-point decimal notation.
```
42
3.14
-10
```

**Strings**: Single-line (double quotes) or multi-line (triple quotes).
```
"hello world"
"""
multi-line
string
"""
```

**Booleans**: `true`, `false`

**Null**: `null`

**Arrays**: Comma-separated values in square brackets. May contain dynamic query expressions.
```
[1, 2, 3]
["a", input.field, uuid_v4()]
```

**Objects**: Comma-separated key-value pairs in curly braces. Keys are strings; values may be dynamic.
```
{"name": "value", "count": 42}
{"id": input.id, "timestamp": now()}
```
