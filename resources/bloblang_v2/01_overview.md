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
  .map(user -> user.name)
  .sort()

# Pattern matching
output.category = match input.score as s {
  s >= 80 => "high",
  s >= 50 => "medium",
  _ => "low",
}

# Named transformation (isolated function)
map normalize_user(data) {
  {
    "id": data.user_id,
    "name": data.full_name
  }
}
output.user = normalize_user(input.user_data)
```

## 1.3 Lexical Structure

**Keywords:** `input`, `output`, `if`, `else`, `match`, `as`, `map`, `import`, `true`, `false`, `null`, `_`

**Reserved function names:** `deleted`, `throw` — these parse as regular function calls but have special semantics (see Sections 8.4, 9.2, and 12.3). User-defined maps cannot shadow these names.

`_` has context-dependent roles: it serves as the wildcard in match cases (Section 4.2) and as a **discard parameter** in map and lambda parameter lists (Sections 3.4, 5.1).

**Operators:** `.`, `?.`, `@`, `::`, `=`, `+`, `-`, `*`, `/`, `%`, `!`, `>`, `>=`, `==`, `!=`, `<`, `<=`, `&&`, `||`, `=>`, `->` (`?.` applies to field access, indexing, and method calls)

**Delimiters:** `(`, `)`, `{`, `}`, `[`, `]`, `?[`, `,`, `:`

**Variables:** `$name` (declaration and reference)

**Metadata:** `input@.key` (read), `output@.key` (write)

**Literals:**
- Numbers: `42`, `3.14` (negative numbers use unary minus: `-10`). Integer literals are int64; float literals are float64. Float literals require digits on both sides of the decimal point — `.5` and `5.` are invalid, write `0.5` and `5.0` instead. Exponent notation (e.g., `1e3`) is not supported in literals — use `.parse_json()` or explicit arithmetic instead. Literals that exceed the range of their type are a compile-time error. **Note:** `-10` is not a single token — it is unary minus applied to `10`. Since method calls bind tighter than unary minus, `-10.string()` parses as `-(10.string())` which is an error. Use `(-10).string()` instead.
- Strings: `"hello"`, `"escape\n"`, `"\u{1F600}"`, or `` `raw multiline` ``
- Booleans: `true`, `false`
- Null: `null`
- Arrays: `[1, 2, 3]`, `["a", input.field, uuid_v4()]`
- Objects: `{"name": "value", "count": 42}`

**Comments:** `#` to end-of-line

**Identifiers:** `[a-zA-Z_][a-zA-Z0-9_]*` excluding keywords (notably `_` alone is not a valid identifier). Used for variable names, map names, and parameter names — these cannot be keywords. The exception is `_`, which is permitted as a discard parameter in map and lambda parameter lists (Sections 3.4, 5.1).

**Field names:** Field names after `.` and `?.` accept any word (`[a-zA-Z_][a-zA-Z0-9_]*` including keywords) — `input.map`, `output.if`, `data.match` are all valid. Use `."quoted"` for names with special characters or spaces:
```bloblang
input.map                   # Valid: keyword as field name
input."field with spaces"   # Quoting needed: spaces
output."special.field"      # Quoting needed: contains dot
```
