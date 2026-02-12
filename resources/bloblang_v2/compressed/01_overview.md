# 1. Overview & Lexical Structure

**Bloblang V2** is a domain-specific mapping language for stream processing with explicit context management and predictable behavior.

## 1.1 Design Principles

1. **Radical Explicitness** - No implicit context shifting, all references explicit
2. **One Clear Way** - Single obvious approach for each operation
3. **Consistent Syntax** - Symmetrical keywords (`input`/`output`), consistent prefixes
4. **Fail Loudly** - Errors are explicit, not silent

## 1.2 Quick Start

```bloblang
# Basic assignment
output.user_id = input.user.id
output.email = input.user.email.lowercase()

# Null-safe navigation
output.city = input.user?.address?.city.or("Unknown")

# Functional pipeline
output.active_users = input.users
  .filter(user -> user.active)
  .map_each(user -> user.name)
  .sort()

# Pattern matching
output.category = match input.score as s {
  s >= 80 => "high"
  s >= 50 => "medium"
  _ => "low"
}

# Named transformation
map normalize_user(data) {
  output.id = data.user_id
  output.name = data.full_name
}
output.user = normalize_user(input.user_data)
```

## 1.3 Lexical Structure

**Keywords:** `input`, `output`, `if`, `else`, `match`, `as`, `map`, `deleted`, `import`

**Operators:** `.`, `?.`, `@.`, `=`, `+`, `-`, `*`, `/`, `%`, `!`, `>`, `>=`, `==`, `<`, `<=`, `&&`, `||`, `=>`, `->`

**Delimiters:** `(`, `)`, `{`, `}`, `[`, `]`, `?[`, `,`, `:`

**Variables:** `$name` (declaration and reference)

**Metadata:** `input@.key` (read), `output@.key` (write)

**Literals:**
- Numbers: `42`, `3.14`, `-10`
- Strings: `"hello"` or `"""multiline"""`
- Booleans: `true`, `false`
- Null: `null`
- Arrays: `[1, 2, 3]`, `["a", input.field, uuid_v4()]`
- Objects: `{"name": "value", "count": 42}`

**Comments:** `#` to end-of-line

**Identifiers:** `[a-zA-Z_][a-zA-Z0-9_]*`, or quoted for special characters: `."field with spaces"`
