# Bloblang V2 — Condensed Language Specification

## 1. Overview

Bloblang V2 is a mapping language for stream processing. A program transforms an immutable `input` document+metadata into a mutable `output` document+metadata via sequential statements.

```bloblang
output.user_id = input.user.id
output.email = input.user.email.lowercase()
output.city = input.user?.address?.city.or("Unknown")
output.active = input.users.filter(u -> u.active).map(u -> u.name).sort()
output.tier = match input.score as s { s >= 80 => "high", s >= 50 => "med", _ => "low" }
```

## 2. Lexical Structure

**Keywords:** `input`, `output`, `if`, `else`, `match`, `as`, `map`, `import`, `true`, `false`, `null`, `_`

**Reserved function names:** `deleted`, `throw` — cannot be identifiers; valid as field names.

**Identifiers:** `[a-zA-Z_][a-zA-Z0-9_]*` excluding keywords and reserved names. `_` alone is not an identifier (it's a keyword used as wildcard/discard).

**Field names after `.`/`?.`:** Any `word` (`[a-zA-Z_][a-zA-Z0-9_]*` including keywords). Use `."quoted"` for special chars/spaces.

**Operators:** `.` `?.` `@` `::` `=` `+` `-` `*` `/` `%` `!` `>` `>=` `==` `!=` `<` `<=` `&&` `||` `=>` `->`

**Delimiters:** `( ) { } [ ] ?[ , :`

**Variables:** `$name` — declaration and reference.

**Metadata:** `input@.key` (read), `output@.key` (write).

**Literals:**
- Integers: `42` (always int64). Overflow is compile error. `-10` is unary minus on `10`.
- Floats: `3.14` (always float64). Must have digits both sides of `.`. No exponent notation.
- Strings: `"hello\n"` with escapes `\\`, `\"`, `\n`, `\t`, `\r`, `\uXXXX` (BMP), `\u{X...}` (1-6 hex, any codepoint). Surrogate codepoints (U+D800-U+DFFF) invalid in both forms. Raw: `` `verbatim` `` (no escapes, no backtick inside).
- Booleans: `true`, `false`
- Null: `null`
- Arrays: `[1, 2, 3]` (trailing comma OK)
- Objects: `{"key": expr}` (trailing comma OK; keys must eval to string at runtime)

**Comments:** `#` to end of line.

**Statement separation:** Newlines separate statements. Inside `()` and `[]`, newlines are whitespace. Inside `{}`, newlines are significant (separate statements in blocks); object literal entries are comma-separated so newlines between them are consumed.

**Continuation rules:**
- **Postfix continuation:** NL suppressed when next token is `.`, `?.`, `[`, `?[`, or `else`.
- **Operator continuation:** NL suppressed when preceding token is a binary op, unary op, `=`, `=>`, `->`, or `:`.

## 3. Type System

Dynamically typed. Runtime types:

| Type | Examples |
|------|---------|
| `string` | `"hello"` (codepoint-based ops) |
| `int32` | `42.int32()` |
| `int64` | `42` (default integer literal type) |
| `uint32` | `42.uint32()` |
| `uint64` | `"18446744073709551615".uint64()` |
| `float32` | `3.14.float32()` |
| `float64` | `3.14` (default float literal type) |
| `bool` | `true`, `false` |
| `null` | `null` |
| `bytes` | `"hello".bytes()` (byte-based ops) |
| `array` | `[1, "two", true]` |
| `object` | `{"key": "value"}` |
| `timestamp` | `now()`, `"2024-03-01".ts_parse("%Y-%m-%d")` |

String ops are **codepoint-based** (not grapheme). No Unicode normalization. Bytes ops are byte-based.

Object key order is **not preserved**. Programs must not depend on iteration order.

### Numeric Promotion

When binary ops mix numeric types, both are promoted:

| Mix | Promoted to | Error condition |
|-----|-------------|-----------------|
| Same type | No change | — |
| Same sign, different width | Wider | — |
| Signed + unsigned int | int64 | uint64 > 2^63-1 |
| Any int + any float | float64 | int magnitude > 2^53 |

Promotion is **checked** — lossy promotion is a runtime error.

**Division** always returns float (float32/float32 → float32; all others → float64). No integer division; use `(7 / 2).int64()` for truncation.

**Modulo** follows standard promotion (not the division rule). Float modulo uses truncated-division remainder (C `fmod`).

**Integer overflow** is always a runtime error (no wrapping).

**Division/modulo by zero** is always an error (no Infinity/NaN production). NaN/Infinity from input data follow IEEE 754 semantics: `NaN == NaN` is false, `NaN != NaN` is true, all NaN comparisons are false, arithmetic with NaN → NaN. `-0.0 == 0.0` is true, `.string()` on `-0.0` → `"0.0"`. `.bool()`: 0/`-0.0` → false, non-zero/Infinity → true, NaN → error.

### Coercion Rules

- `+`: same type family required. Both strings → concat. Both bytes → concat. Both numeric → add with promotion. Cross-family → error.
- `-`, `*`, `%`: numeric only (with promotion). Null → error.
- Comparison (`>`, `<`, `>=`, `<=`): same comparable type. Comparable: numeric (with promotion), timestamps, strings (lexicographic), bytes (lexicographic). Null → error.
- Equality (`==`, `!=`): numeric types use promotion. Non-numeric: same type+value required, cross-type → `false`. Cross-family (numeric vs non-numeric) → `false`.
- Logical (`&&`, `||`, `!`): boolean only. `5 && true` → error.

### Timestamp Semantics

- Compare with `==`, `!=`, `<`, `>`, `<=`, `>=`.
- `timestamp - timestamp` → int64 (nanoseconds). All other timestamp arithmetic → error.
- Use `.ts_add(nanos)` for offset.
- Serialized as RFC 3339, trailing fractional zeros trimmed.

### Null Handling

Null errors in arithmetic, most method calls, and ordering comparisons. Works in equality checks. Use `?.`/`?[]` for null-safe navigation, `.or()` for defaults.

### Void

Void is not a type — it's "no value produced." Sources: if-without-else when false, match-without-`_` when no case matches, `.find()` on no match.

| Context | Behavior |
|---------|----------|
| `output.x = void` | Assignment skipped (no-op) |
| `$x = void` (declaration) | **Runtime error** |
| `$x = void` (reassignment) | Skipped, keeps prior value |
| Collection literal | **Error** |
| Function/map argument | **Error** |
| `.or()` receiver | Returns argument (rescued) |
| `.catch()` receiver | Passes through unchanged |
| Other method/operator | **Error** |

### deleted()

`deleted()` is a deletion marker, distinct from void.

**Triggers deletion (no error):** field assignment, root output (`output = deleted()` drops message + exits mapping immediately — no subsequent statements execute), metadata key, variable field (`$var.field = deleted()`), array index (removes element + shifts remaining down; negative indices count from end; OOB → error), collection literals (element/field omitted). **Path suffix rule:** `output.items[0].name = deleted()` removes the `name` field from the object at index 0 — it does NOT remove the array element. `deleted()` applies to the final path component.

**Causes error:** variable assignment (`$var = deleted()`), metadata root (`output@ = deleted()`), binary ops, method calls (except `.or()` and `.catch()`), function arguments.

**Method chain propagation:** When `deleted()` hits an unsupported method, the method produces an error. That error propagates through subsequent methods (skipping them) until caught by `.catch()`. Example: `deleted().uppercase().catch(err -> "recovered")` → `"recovered"`.

`.or()` rescues deleted. `.catch()` passes deleted through (not an error — handler not invoked).

## 4. Expressions & Operators

### Precedence (high to low)

1. Postfix: `.`, `?.`, `[]`, `?[]`, `.method()`, `?.method()`
2. Unary: `!`, `-`
3. Multiplicative: `*`, `/`, `%`
4. Additive: `+`, `-`
5. Comparison: `>`, `>=`, `<`, `<=`
6. Equality: `==`, `!=`
7. Logical AND: `&&`
8. Logical OR: `||`

Left-associative: arithmetic, logical. Non-associative: comparison, equality (chaining like `a < b < c` is a **parse error**).

`->` is not a binary op; lambda prefix (`x ->` or `(params) ->`) consumes entire RHS as body.

### Path Expressions

Roots: `input`, `output`, `$variable`, or bare identifiers (parameters/match bindings — read-only, expression-only).

**Indexing:**
- Objects: `[string]` → field value. Non-string index → error.
- Arrays: `[number]` → element. Must be whole number (2.0 OK, 1.5 error). Negative counts from end.
- Strings: `[number]` → int64 codepoint value. Use `.char()` to convert back.
- Bytes: `[number]` → int64 byte value (0-255).
- Out of bounds → error. All other types → error.

**Null-safe:** `?.field`, `?[index]`, `?.method()` — short-circuit to null if receiver is null. Type errors still throw.

### Name Resolution

Every bare identifier in expressions must resolve to: parameter > map name > stdlib function name. Unresolved → **compile error**. Namespace-qualified: `ns::name`.

Map/function names are only valid: (1) called with parens, or (2) as higher-order method arguments (`.map(double)`). Bare use elsewhere → compile error. Cannot be stored in variables.

### Method Resolution

Methods resolved at compile time (unknown method → compile error). Type compatibility checked at runtime.

## 5. Control Flow

### If Expression (returns value)

```bloblang
output.x = if cond { expr } else if cond2 { expr2 } else { expr3 }
```

Without `else`: void when false (assignment skipped). Body is expression context: variable assignments + final expression. No `output` assignments.

### If Statement (standalone, side effects)

```bloblang
if cond {
  output.x = value
  output.y = value2
} else if cond2 {
  output.z = value3
}
```

Body is statement context: can assign to `output`/`output@`. **Cannot** end with a trailing expression (parse error). Empty body is valid (no-op).

### Match — Three Forms

**1. Equality match** (`match expr { value => result, ... }`): Cases compared by `==`. If a case evaluates to boolean → **error** (catches common mistake of writing conditions without `as`). Boolean literals as cases → compile error; dynamic booleans → runtime error.

**2. Boolean match with `as`** (`match expr as x { bool_cond => result, ... }`): Expression evaluated once, bound to `x` (read-only, block-scoped). Cases must be boolean. First `true` wins. Non-boolean case → error.

**3. Boolean match** (`match { bool_cond => result, ... }`): No expression. Cases must be boolean. First `true` wins.

`_` is wildcard catch-all in all forms — always matches, exempt from type requirements.

**Non-exhaustive match** (no `_`): produces void in expression context (same rules as if-without-else). In statement context: no-op.

**Expression match** cases: bare expr or braced body `=> { vars; expr }`. **Statement match** cases: always braced `=> { statements }`. Empty statement case body is valid.

### Block-Scoped Variables

**Expression contexts** (if/match expressions, lambdas, map bodies): assigning to outer variable name **shadows** (new inner variable). Outer unchanged.

**Statement contexts** (if/match statements at top-level): assigning to existing outer variable **modifies** it. New variables are block-scoped (not visible outside).

Same-scope reassignment is always mutation, not shadowing.

## 6. Maps (User-Defined Functions)

```bloblang
map name(param1, param2 = "default") {
  $temp = param1.field
  $temp + param2        # final expression is return value
}
output.result = name(input.data, "override")
output.result = name(param1: input.data)
```

- **Isolated:** Cannot access `input`, `output`, or top-level `$variables`. Only parameters + local variables.
- Parameters are **read-only**.
- Default params: must come after required; must be literals. `_` discard params allowed (no binding, can't reference).
- Positional or named args (not mixed). `_` params → positional only.
- Arity: positional must provide [required, total]. Named must include all required; defaults used for missing optional. Errors on extra/unknown.
- **Hoisted:** Can be called before declaration. Duplicate names → compile error.
- **Recursion:** Supported (self and mutual within same file only — mutual recursion across files impossible due to circular import ban). Min 1000 depth. Exceeding limit → uncatchable error.
- Parameter names shadow map names of same name within body.

### Maps as Method Arguments

```bloblang
map double(x) { x * 2 }
output.doubled = input.items.map(double)  # sugar for .map(x -> double(x))
```

Compile-time references, not runtime values. Works for map names, `ns::name`, and stdlib function names.

## 7. Lambdas

Inline syntax for method arguments — not storable values.

```bloblang
x -> x * 2                          # single param
(a, b) -> a + b                     # multi param
(a, b = 0) -> a + b                 # with default
_ -> "constant"                     # discard
x -> { $t = x * 2; $t + 1 }        # block body (vars + final expr)
```

Parameters are read-only, available as bare identifiers. `_` not bound. Expression context: cannot assign to `output`. Inherit read permissions of enclosing context (top-level lambda can read `input`/`output`; lambda inside map cannot).

## 8. Imports

```bloblang
import "./file.blobl" as ns
output.result = ns::transform(input.data)
```

- Path: relative to importing file, or absolute.
- Imported files: **only** map declarations and import statements. Top-level statements → compile error.
- All top-level maps exported automatically.
- Duplicate namespace → error. Circular imports → compile error. File not found → error.

## 9. Execution Model

- `input` (doc + metadata): immutable.
- `output` doc: starts as `{}`. `output@` metadata: starts as `{}`.
- Statements execute top-to-bottom. Variables must be declared before use.
- Reading non-existent field → `null`. JSON: non-existent fields omitted; `null` fields serialized.
- `output = expr` replaces entire output (any type). Prior assignments discarded.
- `output@ = expr` must be object (error otherwise). `output@ = deleted()` → error. `output@ = {}` clears.
- Assignment to variable creates logical copy (COW). Mutations never affect source.

### Auto-creation

Assigning to nested paths auto-creates intermediate objects/arrays:
```bloblang
output.user.address.city = "London"  # creates {user: {address: {city: "London"}}}
output.items[0].name = "first"       # creates items as array
$arr[5] = 30                         # fills gaps with null: [..., null, null, 30]
```

Collision with incompatible type → error.

### Scoping Rules

| Context | Read input | Read output | Write output/@ | Variables |
|---------|-----------|-------------|-----------------|-----------|
| Top-level | yes | yes | yes | yes |
| Map body | **no** | **no** | **no** | local only |
| Expression (top-level) | yes | yes | **no** | yes |
| Expression (in map) | **no** | **no** | **no** | map-local only |

## 10. Error Handling

### .catch(fn)

Catches errors from entire receiver expression. Non-error values pass through unchanged. Error object: `{"what": "message string"}`.

```bloblang
input.date.ts_parse("%Y-%m-%d").catch(err -> null)
expr.method1().method2().catch(err -> fallback)  # catches error from any part of chain
```

Void and deleted pass through `.catch()` unchanged (not errors). Handler may return deleted/void.

All runtime errors are catchable **except** recursion limit.

### .or(default)

Short-circuit default for **null**, **void**, or **deleted()**. Argument only evaluated if needed. Does NOT catch errors.

```bloblang
input.name.or("Anonymous")
input.name.or(throw("required"))  # throw only if null
(if false { 1 }).or(0)            # rescues void
```

### throw(message)

Produces error. String arg required (non-string literal → compile error; non-string dynamic → runtime error). Catchable with `.catch()`. Uncaught → halts mapping.

### Null-safe vs Error-safe

- `?.`/`?[]`/`?.method()`: handle **null** only (type errors still throw)
- `.catch()`: handles **errors** only
- `.or()`: handles **null/void/deleted** only

## 11. Grammar

```
program         := NL? (top_level_stmt (NL top_level_stmt)*)? NL?
top_level_stmt  := statement | map_decl | import_stmt
statement       := assignment | if_stmt | match_stmt

assignment      := assign_target '=' expression
assign_target   := 'output' '@'? path_component* | var_ref path_component*

var_assignment  := var_ref path_component* '=' expression

map_decl        := 'map' identifier '(' [param_list] ')' '{' NL? (var_assignment NL)* expression NL? '}'
param_list      := param (',' param)*
param           := identifier | identifier '=' literal | '_'
import_stmt     := 'import' string_literal 'as' identifier

expression      := postfix_expr | binary_expr | unary_expr | lambda_expr
control_expr    := if_expr | match_expr

postfix_expr    := primary_expr postfix_op*
primary_expr    := literal | context_root | call_expr | control_expr | paren_expr
context_root    := ('output' | 'input') '@'? | var_ref | qualified_name | identifier

postfix_op      := path_component | method_call
path_component  := '.' field_name | '?.' field_name | '[' expression ']' | '?[' expression ']'
method_call     := '.' word '(' [arg_list] ')' | '?.' word '(' [arg_list] ')'
field_name      := word | string_literal
var_ref         := '$' identifier

call_expr       := (identifier | qualified_name | reserved_name) '(' [arg_list] ')'
qualified_name  := identifier '::' identifier

if_expr         := 'if' expression '{' NL? expr_body NL? '}'
                   (NL? 'else' 'if' expression '{' NL? expr_body NL? '}')*
                   (NL? 'else' '{' NL? expr_body NL? '}')?
if_stmt         := 'if' expression '{' NL? stmt_body NL? '}'
                   (NL? 'else' 'if' expression '{' NL? stmt_body NL? '}')*
                   (NL? 'else' '{' NL? stmt_body NL? '}')?
expr_body       := (var_assignment NL)* expression
stmt_body       := (statement (NL statement)*)?

match_expr      := 'match' expression ('as' identifier)? '{' NL? expr_match_case (',' NL? expr_match_case)* ','? NL? '}'
                 | 'match' '{' NL? expr_match_case (',' NL? expr_match_case)* ','? NL? '}'
match_stmt      := 'match' expression ('as' identifier)? '{' NL? stmt_match_case (',' NL? stmt_match_case)* ','? NL? '}'
                 | 'match' '{' NL? stmt_match_case (',' NL? stmt_match_case)* ','? NL? '}'
expr_match_case := (expression | '_') '=>' (expression | '{' NL? expr_body NL? '}')
stmt_match_case := (expression | '_') '=>' '{' NL? stmt_body NL? '}'

binary_expr     := expression binary_op expression   # see precedence table
binary_op       := '+' | '-' | '*' | '/' | '%' | '==' | '!=' | '>' | '>=' | '<' | '<=' | '&&' | '||'
unary_expr      := ('!' | '-') expression

lambda_expr     := lambda_params '->' (expression | '{' NL? (var_assignment NL)* expression NL? '}')
lambda_params   := identifier | '_' | '(' param (',' param)* ')'
paren_expr      := '(' expression ')'

literal         := float_literal | int_literal | string_literal | boolean | null | array | object
int_literal     := [0-9]+
float_literal   := [0-9]+ '.' [0-9]+
string_literal  := '"' string_char* '"' | '`' raw_char* '`'
array           := '[' [expression (',' expression)* ','?] ']'
object          := '{' NL? [key_value (',' NL? key_value)* ','?] NL? '}'
key_value       := expression ':' expression

arg_list        := positional_args | named_args
positional_args := expression (',' expression)* ','?
named_args      := identifier ':' expression (',' identifier ':' expression)* ','?
word            := [a-zA-Z_][a-zA-Z0-9_]*
identifier      := word - keyword - reserved_name
keyword         := 'input' | 'output' | 'if' | 'else' | 'match' | 'as' | 'map' | 'import' | 'true' | 'false' | 'null' | '_'
reserved_name   := 'deleted' | 'throw'
```

**Disambiguation notes:**
- After `match`, `{` is always match body (not object literal).
- `identifier (` → call_expr; `identifier` without `(` → context_root.
- `{}` in expression context (map/lambda body) → empty object literal (blocks need >= 1 expression).

## 12. Standard Library

All functions/methods support named arguments using documented parameter names. Parameter type promotion applies: integer params accept whole-number floats; float params accept integers.

Unless documented otherwise, void and deleted() as lambda returns are runtime errors. Methods supporting deleted() returns: `.map()`, `.map_values()`, `.map_keys()`, `.map_entries()`, `.catch()`.

Regex parameters use RE2 syntax.

### 12.1 Functions

| Function | Params | Returns | Description |
|----------|--------|---------|-------------|
| `uuid_v4()` | — | string | Random UUID v4 |
| `now()` | — | timestamp | Current time (fresh each call) |
| `random_int(min, max)` | int64, int64 | int64 | Random in [min, max]. Error if min > max |
| `range(start, stop, step?)` | int64, int64, int64? | array[int64] | [start, stop) with step. Inferred if omitted. Error: zero step, contradicting direction |
| `timestamp(year, month, day, hour=0, minute=0, second=0, nano=0, timezone="UTC")` | int64s + string | timestamp | Construct from components |
| `second()` | — | int64 | 1000000000 (ns) |
| `minute()` | — | int64 | 60000000000 (ns) |
| `hour()` | — | int64 | 3600000000000 (ns) |
| `day()` | — | int64 | 86400000000000 (ns) |
| `throw(message)` | string | never | Throw error (Section 10) |
| `deleted()` | — | deletion marker | Deletion marker (Section 3) |

### 12.2 Type Conversion Methods

| Method | Receiver | Returns | Notes |
|--------|----------|---------|-------|
| `.string()` | any | string | int→decimal, float→shortest round-trip (always includes `.` or `e` to distinguish from int), `-0.0`→`"0.0"`, NaN→`"NaN"`, Inf→`"Infinity"`, bool→`"true"`/`"false"`, null→`"null"`, timestamp→RFC3339, bytes→UTF-8 decode (error if invalid), array/object→compact JSON (sorted keys). Containers with bytes → error. |
| `.int32()` | numeric, string | int32 | Floats truncated toward zero. Out of range → error |
| `.int64()` | numeric, string | int64 | Floats truncated toward zero |
| `.uint32()` | numeric, string | uint32 | Negative → error. Floats truncated |
| `.uint64()` | numeric, string | uint64 | Negative → error. Floats truncated |
| `.float32()` | numeric, string | float32 | Unchecked precision loss (opt-in) |
| `.float64()` | numeric, string | float64 | Unchecked precision loss (opt-in) |
| `.bool()` | bool, string, numeric | bool | `"true"`/`"false"`, 0→false, non-zero→true. NaN→error |
| `.char()` | int types | string | Codepoint to single-char string. Invalid codepoint → error |
| `.bytes()` | any | bytes | String→UTF-8; bytes→as-is; others→`.string().bytes()`. Containers with bytes → error |
| `.type()` | any (incl. null) | string | Returns type name: `"string"`, `"int64"`, `"float64"`, `"bool"`, `"null"`, `"bytes"`, `"timestamp"`, `"array"`, `"object"`, etc. |

### 12.3 Sequence Methods (string, array, bytes)

| Method | Returns | Description |
|--------|---------|-------------|
| `.length()` | int64 | String: codepoints. Array: elements. Bytes: bytes. Object: keys. |
| `.contains(target)` | bool | String: substring. Array: element by `==`. Bytes: subsequence (bytes target). |
| `.index_of(target)` | int64 | First occurrence index, or -1. Same target types as `.contains()`. |
| `.slice(low, high?)` | same type | [low, high). Negative indices. Clamped to bounds. `low >= high` → empty. |
| `.reverse()` | same type | Reverse by unit (codepoint/element/byte). |

### 12.4 String Methods

| Method | Params | Returns | Description |
|--------|--------|---------|-------------|
| `.uppercase()` | — | string | To uppercase |
| `.lowercase()` | — | string | To lowercase |
| `.trim()` | — | string | Strip Unicode whitespace |
| `.trim_prefix(prefix)` | string | string | Remove prefix if present |
| `.trim_suffix(suffix)` | string | string | Remove suffix if present |
| `.has_prefix(prefix)` | string | bool | Starts with prefix |
| `.has_suffix(suffix)` | string | bool | Ends with suffix |
| `.split(delimiter)` | string | array[string] | Split. `"".split("")` → `[]`. `"".split(",")` → `[""]`. |
| `.replace_all(old, new)` | string, string | string | Replace all occurrences |
| `.repeat(count)` | int64 | string | Repeat count times. Negative → error |
| `.re_match(pattern)` | string | bool | RE2 match (any part of string) |
| `.re_find_all(pattern)` | string | array[string] | All non-overlapping matches |
| `.re_replace_all(pattern, replacement)` | string, string | string | Replace matches. `$0`, `$1`, `${name}` expansion |

### 12.5 Array Methods

| Method | Params | Returns | Description |
|--------|--------|---------|-------------|
| `.filter(fn)` | elem→bool | array | Keep elements where fn returns true. Non-bool → error. |
| `.map(fn)` | elem→any | array | Transform elements. Void → error. `deleted()` → omit element. |
| `.sort()` | — | array | Stable ascending sort. Same sortable family required (numeric/string/timestamp). NaN sorts last. |
| `.sort_by(fn)` | elem→comparable | array | Stable sort by key function |
| `.append(value)` | any | array | New array with value appended |
| `.concat(other)` | array | array | Concatenate two arrays |
| `.flatten()` | — | array | Flatten one level. Non-arrays kept as-is. |
| `.unique(fn?)` | (elem→any)? | array | Deduplicate (first occurrence kept). Optional key fn. NaN values considered equal. |
| `.without_index(index)` | int64 | array | Remove at index, shift down. Negative OK. OOB → error. |
| `.enumerate()` | — | array[{index, value}] | Convert to `[{"index": i, "value": v}, ...]` |
| `.any(fn)` | elem→bool | bool | Short-circuits on first true. Empty → false. |
| `.all(fn)` | elem→bool | bool | Short-circuits on first false. Empty → true. |
| `.find(fn)` | elem→bool | any or void | First match. Short-circuits. No match → **void**. Use `.or()`. |
| `.join(delimiter)` | string | string | Join string elements. Non-string → error. |
| `.sum()` | — | numeric | Sum with promotion. Empty → 0 (int64). Non-numeric → error. |
| `.min()` | — | comparable | Min element. Same sortable family. Empty → error. |
| `.max()` | — | comparable | Max element. Same sortable family. Empty → error. |
| `.fold(initial, fn)` | any, (tally,elem)→any | any | Reduce with accumulator |
| `.collect()` | — | object | Array of `{"key":k,"value":v}` → object. Extra fields ignored. Last wins on dup keys. Error if element not object, missing key/value fields, or key not string. |

### 12.6 Object Methods

| Method | Params | Returns | Description |
|--------|--------|---------|-------------|
| `.iter()` | — | array[{key,value}] | To `[{"key":k,"value":v}, ...]`. Order not guaranteed. |
| `.keys()` | — | array[string] | Key names. Order not guaranteed. |
| `.values()` | — | array | Values. Order not guaranteed but corresponds to `.keys()` within same call. |
| `.has_key(key)` | string | bool | Key exists check |
| `.merge(other)` | object | object | Merge. `other` wins on conflict. |
| `.without(keys)` | array[string] | object | Remove specified keys. Missing keys ignored. |
| `.map_values(fn)` | value→any | object | Transform values. Void → error. `deleted()` → omit entry. |
| `.map_keys(fn)` | key→string | object | Transform keys. `deleted()` → omit. Non-string → error. Dup keys: last wins. |
| `.map_entries(fn)` | (key,value)→{key,value} | object | Transform both. `deleted()` → omit. |
| `.filter_entries(fn)` | (key,value)→bool | object | Keep entries where fn returns true |

### 12.7 Numeric Methods

| Method | Receiver | Returns | Description |
|--------|----------|---------|-------------|
| `.abs()` | numeric | same type | Absolute value. Signed min → overflow error. |
| `.floor()` | float | same float | Largest int <= value |
| `.ceil()` | float | same float | Smallest int >= value |
| `.round(n=0)` | float | same float | Half-even rounding to n decimal places. Negative n rounds to powers of 10. |

### 12.8 Time Methods

| Method | Receiver | Params | Returns | Description |
|--------|----------|--------|---------|-------------|
| `.ts_parse(format?)` | string | strftime format (default RFC3339) | timestamp | Parse string to timestamp |
| `.ts_format(format?)` | timestamp | strftime format (default RFC3339) | string | Format timestamp |
| `.ts_unix()` | timestamp | — | int64 | Seconds since epoch |
| `.ts_unix_milli()` | timestamp | — | int64 | Milliseconds |
| `.ts_unix_micro()` | timestamp | — | int64 | Microseconds |
| `.ts_unix_nano()` | timestamp | — | int64 | Nanoseconds |
| `.ts_from_unix()` | numeric | — | timestamp | From seconds (ints→int64, float32→float64). Float for sub-second (~us precision). uint64 > int64 range → error. |
| `.ts_from_unix_milli()` | int64 | — | timestamp | From milliseconds |
| `.ts_from_unix_micro()` | int64 | — | timestamp | From microseconds |
| `.ts_from_unix_nano()` | int64 | — | timestamp | From nanoseconds (lossless) |
| `.ts_add(nanos)` | timestamp | int64 | timestamp | Add duration. Negative subtracts. |

**Required strftime directives:** `%Y` (4-digit year), `%m` (month 01-12), `%d` (day 01-31), `%H` (hour 00-23), `%M` (minute 00-59), `%S` (second 00-59), `%f` (fractional seconds, optional on parse, shortest on format), `%z` (UTC offset or Z), `%Z` (timezone name), `%a`/`%A` (weekday), `%b`/`%B` (month name), `%p` (AM/PM), `%I` (12-hour), `%j` (day of year), `%%` (literal %).

### 12.9 Error Handling Methods

| Method | Receiver | Params | Returns | Description |
|--------|----------|--------|---------|-------------|
| `.catch(fn)` | any | err→any | any | Handle errors. err is `{"what": "msg"}`. Handler may return deleted/void. |
| `.or(default)` | any (incl. void/deleted) | any (lazy) | any | Default for null/void/deleted. Short-circuit. |
| `.not_null(message?)` | any (incl. null, but NOT void/deleted) | string (default "unexpected null value") | same | Assert non-null. Throws on null. Regular method — void/deleted error before reaching it. Use `.or(throw(...))` to catch all three. |

### 12.10 Parsing Methods

| Method | Receiver | Params | Returns | Description |
|--------|----------|--------|---------|-------------|
| `.parse_json()` | string, bytes | — | any | Parse JSON. Ints → int64 (or float64 if > int64 range). Floats/exponents → float64. |
| `.format_json(indent="", no_indent=false, escape_html=true)` | any (not bytes) | — | string | Serialize to JSON. Keys sorted lexicographically. Bytes in value → error. NaN/Inf → error. Ints as JSON ints (no `.`); floats as shortest round-trip (with `.` or `e`). Large uint64 (> 2^53) serialized as-is. |
| `.encode(scheme)` | string, bytes | `"base64"`, `"base64url"`, `"base64rawurl"`, `"hex"` | string | Encode to string |
| `.decode(scheme)` | string | same schemes | bytes | Decode to bytes |

## 13. Intrinsics

`.catch()`, `.or()`, `throw()`, and `deleted()` parse as regular calls/methods but require special runtime handling:

- `.catch()`: activates on errors (opposite of normal methods which skip on error)
- `.or()`: short-circuit evaluation; works on void/deleted (other methods error on these)
- `throw()`: produces error; must support lazy eval in `.or(throw(...))`
- `deleted()`: produces deletion marker tracked through assignments/collections
