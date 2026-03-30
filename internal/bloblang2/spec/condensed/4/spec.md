# Bloblang V2 — Condensed Specification

A mapping language for stream processing. Input is immutable; output is built incrementally. Maps (user functions) are isolated. Errors propagate until caught.

---

## 1. Lexical Structure

**Keywords:** `input`, `output`, `if`, `else`, `match`, `as`, `map`, `import`, `true`, `false`, `null`, `_`

**Reserved function names:** `deleted`, `throw` — parsed as function calls, cannot be used as identifiers. Valid as field names (`input.deleted`).

**Operators:** `.` `?.` `@` `::` `=` `+` `-` `*` `/` `%` `!` `>` `>=` `==` `!=` `<` `<=` `&&` `||` `=>` `->`

**Delimiters:** `( ) { } [ ] ?[ , :`

**Variables:** `$name`
**Metadata:** `input@.key` (read), `output@.key` (write)

**Identifiers:** `[a-zA-Z_][a-zA-Z0-9_]*` excluding keywords and reserved names. `_` is not a valid identifier (it is a keyword), but is allowed as a discard parameter in maps/lambdas and as a match wildcard.

**Field names** after `.`/`?.`: any `[a-zA-Z_][a-zA-Z0-9_]*` including keywords. Use `."quoted"` for special characters/spaces/digits-first.

**Literals:**
- Int64: `42` (no leading `.`, no exponent notation). Negative via unary minus: `-10`. Overflow is compile error.
- Float64: `3.14` (digits required on both sides of `.`). No exponent notation in literals.
- String: `"hello\n"` with escapes `\\` `\"` `\n` `\t` `\r` `\uXXXX` `\u{X...}` (surrogate codepoints U+D800-U+DFFF invalid in both forms). Raw: `` `verbatim` `` (no escapes, no backticks inside).
- Bool: `true`, `false`
- Null: `null`
- Array: `[1, 2, 3]` (trailing comma OK)
- Object: `{"key": expr}` (trailing comma OK, keys must evaluate to string at runtime)

**Comments:** `#` to end of line.

**Statement separation:** Newlines separate statements. Newlines suppressed inside `()` and `[]`. Inside `{}`, newlines are significant (separate statements) but object literal entries use commas. Newlines suppressed when next line starts with `.` `?.` `[` `?[` `else`, or when previous token is a binary/unary operator, `=`, `=>`, `->`, or `:`.

---

## 2. Type System

Dynamically typed. Runtime types:

| Type | Default literal | Notes |
|------|----------------|-------|
| `string` | `"text"` | UTF-8, codepoint-based operations |
| `int64` | `42` | Default integer literal type |
| `int32` | `42.int32()` | |
| `uint32` | `42.uint32()` | |
| `uint64` | `"N".uint64()` | Large values must be parsed from string |
| `float64` | `3.14` | Default float literal type |
| `float32` | `3.14.float32()` | |
| `bool` | `true`/`false` | |
| `null` | `null` | |
| `bytes` | `"hi".bytes()` | Byte-based operations |
| `array` | `[1, 2]` | |
| `object` | `{"a": 1}` | Key order not preserved |
| `timestamp` | `now()` | Nanosecond precision |

**Void** is not a type — it is the absence of a value (see Section 5).

### Numeric Promotion

When binary operators mix numeric types, both are promoted:
1. Same type → no promotion
2. Same signedness, different width → wider type
3. Signed + unsigned integer → int64 (error if uint64 > 2^63-1)
4. Any integer + any float → float64 (error if integer magnitude > 2^53)

Promotions are checked — lossy conversions error at runtime.

### Operator Semantics

- `+`: same-family only. strings→concat, bytes→concat, numeric→add. Cross-family = error.
- `-` `*` `%`: numeric only. Integer overflow = error.
- `/`: always produces float (float32/float32→float32, else float64). Division by zero = error.
- `%`: standard promotion (not division rule). Truncated remainder for floats.
- `>` `>=` `<` `<=`: same sortable type only (numeric with promotion, strings lexicographic, bytes lexicographic, timestamps). null = error.
- `==` `!=`: numeric uses promotion then compare. Non-numeric: same type+value. Cross-family always `false`.
- `&&` `||` `!`: require booleans. No truthy/falsy.

**Timestamps:** `ts1 - ts2` → int64 nanoseconds. All other arithmetic with timestamps = error. Use `.ts_add(nanos)`.

**NaN/Infinity:** Follow IEEE 754 (`NaN == NaN` is false, etc.). Division by zero is error (no Inf produced). Sort uses total order: NaN after everything. **Negative zero:** `-0.0 == 0.0` is `true`, `-0.0 < 0.0` is `false`. `.string()` normalizes to `"0.0"`.

**Integer overflow:** Always a runtime error for all integer types and operators.

### Null Handling

Null errors in most operations. Works with `==`/`!=`. Use `?.`/`?[]`/`?.method()` for null-safe access (short-circuits on null only, not type errors). `.or(default)` provides defaults for null.

---

## 3. Expressions & Statements

### Operator Precedence (high to low)
1. Postfix: `.field` `?.field` `[i]` `?[i]` `.method()` `?.method()`
2. Unary: `!` `-`
3. Multiplicative: `*` `/` `%`
4. Additive: `+` `-`
5. Comparison: `>` `>=` `<` `<=` (non-associative)
6. Equality: `==` `!=` (non-associative)
7. Logical AND: `&&`
8. Logical OR: `||`

Arithmetic and logical operators are left-associative. Chaining non-associative operators (`a < b < c`) is a parse error.

**Lambda arrow** (`->`) is not in the precedence table — it consumes the entire RHS as the body.

Note: `-10.string()` parses as `-(10.string())` which errors. Use `(-10).string()`.

### Path Expressions

Roots: `input`, `output`, `$variable`, bare identifiers (parameters, match `as` bindings).

**Name resolution:** Every bare identifier must resolve to: parameter > map name > stdlib function. Unresolved = compile error. Map/function names only valid as calls `f(x)` or higher-order arguments `.map(f)`, never as values.

**Indexing:**
- Object: `obj["key"]` or `obj[$var]` — string index, non-string = error
- Array: `arr[0]`, `arr[-1]` — numeric index (must be whole number), negative counts from end
- String: `str[0]` → int64 codepoint value. Use `.char()` to convert back.
- Bytes: `b[0]` → int64 byte value (0-255)
- Out-of-bounds = error. Other types = error.

**Null-safe:** `?.field`, `?[index]`, `?.method()` — short-circuit to null when receiver is null **only**. Type errors still throw. `deleted()` is not null — `deleted()?.field` is an error.

### Functions & Methods

```
output.id = uuid_v4()                    # function call
output.up = input.text.uppercase()       # method call
output.r = func(a: 1, b: 2)             # named args (cannot mix with positional)
```

Methods resolved at compile time (unknown method = compile error). Type compatibility checked at runtime. Duplicate named arguments = compile error.

### Lambdas

Inline syntax for method arguments only — not storable in variables.

```
x -> x * 2                              # single param
(a, b) -> a + b                         # multi param
_ -> "constant"                         # discard param
(_, v) -> v * 2                         # partial discard
x -> { $t = x * 2                      # block body (newline-separated,
  $t + 1 }                               #   must end with expression)
(x, y = 0) -> x + y                     # default params
```

Parameters are read-only. Lambdas cannot assign to `output`/`output@`. Variable assignments in lambda bodies shadow outer variables (expression context rules). Lambdas inherit read permissions of enclosing context. Parameter names shadow map names of the same name within the body.

### Statements

**Assignment:** `output.field = expr`, `output@.key = expr`
**Variable:** `$var = expr` (mutable, block-scoped)
**Variable path assignment:** `$var.field = expr`, `$var[0] = expr` (auto-creates intermediates, same as output)

Assigning to a variable creates a logical copy (COW). Mutations never affect the source.

**Auto-creation:** Assigning nested paths creates intermediate objects/arrays. Array gaps fill with null. Collision with incompatible type = error.

### Variable Scoping

- **Expression contexts** (if/match expressions, lambdas, map bodies): assigning to outer variable name **shadows** (creates new inner variable).
- **Statement contexts** (if/match statements at top-level): assigning to existing outer variable **modifies** it. New declarations are block-scoped.
- Same-scope reassignment is always mutation, not shadowing.

---

## 4. Control Flow

### If

**Expression** (in assignment RHS — no output assignments inside):
```
output.x = if cond { val } else { other }
output.x = if cond { val }                    # void if false
output.x = if a { 1 } else if b { 2 } else { 3 }
```

**Statement** (standalone — contains output assignments, cannot end with bare expression, empty bodies are valid no-ops):
```
if cond {
  output.x = val
}
```

### Match

Three forms:

**1. Equality match** — `match expr { value => result }`: cases compared by `==`. Boolean case values are an error (catches common mistake — use `as` or `if/else` instead). Boolean literals rejected at compile time; dynamic booleans at runtime.

**2. Boolean match with `as`** — `match expr as x { bool_expr => result }`: expression evaluated once and bound to `x`. Cases must be boolean expressions (non-boolean = runtime error). `as` binding available in case conditions, result expressions, and statement bodies. Block-scoped to the match.

**3. Boolean match** — `match { bool_expr => result }`: no matched expression. Cases must be boolean (non-boolean = runtime error).

In all forms: `_` is unconditional catch-all. Cases evaluated in order, first match wins. Trailing commas OK.

**Expression** (in assignment, `expr_body`):
```
output.x = match input.v { "a" => 1, _ => 0 }
```

**Statement** (standalone, `stmt_body` — cases require braces):
```
match input.type {
  "a" => { output.x = 1 },
  _ => { output.x = 0 },
}
```

---

## 5. Void

Void is the absence of a value, produced by:
- `if` without `else` when condition is false
- `match` without `_` when no case matches
- `.find()` when no element matches

| Context | Behavior |
|---------|----------|
| `output.x = void` | Assignment skipped (prior value preserved) |
| `$x = void` (first assignment) | **Runtime error** |
| `$x = void` (reassignment) | Assignment skipped (prior value preserved) |
| Collection literal `[void]` | **Error** |
| Function/map argument | **Error** |
| `.map()` lambda return | **Error** |
| `.filter()` lambda return | **Error** |
| Expression operand `void + 1` | **Error** |
| `.or(default)` | Returns default (rescues void) |
| `.catch(fn)` | Void passes through (not an error) |
| Other method on void | **Error** |

---

## 6. Maps (User-Defined Functions)

Isolated, reusable transformations.

```
map name(param1, param2, opt = "default") {
  # variable declarations (optional)
  $temp = param1 * 2
  # final expression (return value — required)
  $temp + param2
}
```

- **Isolated**: no access to `input`, `output`, or top-level `$variables`. Only parameters and locally declared `$variables`.
- Parameters are **read-only**.
- Default parameters must come after required params. Defaults must be literals.
- `_` as parameter: accepts and ignores argument. Not bound. Multiple allowed. Forces positional-only calls.
- Call: `name(arg1, arg2)` or `name(param1: arg1, param2: arg2)`. Cannot mix positional and named.
- **Hoisted**: can be called before declaration. Duplicate names in same file = compile error.
- Recursion supported including mutual recursion within the same file (min 1000 depth, shared limit). Exceeding limit = uncatchable error.
- Can be passed as higher-order arguments: `.map(my_func)` (compile-time sugar, not a runtime value).
- If a map body's final expression is an if-without-else or match-without-`_`, the map can produce void. Void follows standard propagation rules in the calling context.
- Parameter names shadow map names of the same name within the body. `namespace::name` is unaffected.

---

## 7. Imports

```
import "./path.blobl" as namespace
namespace::map_name(args)
```

- Relative paths resolve from importing file's directory. Absolute paths used as-is.
- Imported files may only contain `map` declarations and `import` statements (no top-level statements).
- All maps are automatically exported.
- Circular imports = compile error (mutual recursion across files is not possible).
- Duplicate namespace names = error.

---

## 8. Execution Model

**Input:** Immutable document + metadata. Can be any type (commonly object from JSON).
**Output:** Starts as `{}`. Built incrementally. Metadata (`output@`) starts as `{}`.

```
output = input                          # copy entire document (COW)
output@ = input@                        # copy all metadata (COW)
output.field = deleted()                # remove field
output = deleted()                      # drop message, exit mapping immediately
output@.key = deleted()                 # remove metadata key
output@ = {}                            # clear all metadata
output@ = deleted()                     # ERROR: cannot delete metadata
output@ = "string"                      # ERROR: metadata must be object
```

Non-existent fields return `null`. Explicit `null` vs absent: both read as null, but differ in JSON serialization (null serialized, absent omitted).

Root assignment `output = expr` replaces entire document (any type).

**Evaluation order:** Top-to-bottom. Variables must be declared before use. Maps are hoisted. Later statements can reference earlier output fields. Reading an unassigned output field returns `null` (reordering statements can silently change behavior).

### Scoping Rules

| Context | Read input | Read output | Write output | Variables |
|---------|-----------|-------------|-------------|-----------|
| Top-level | yes | yes | yes | yes |
| Map body | no | no | no | local only |
| Expression (top-level) | yes | yes | no | yes |
| Expression (in map) | no | no | no | local only |
| Match `as` binding (top-level) | yes | yes | no | yes |
| Match `as` binding (in map) | no | no | no | local only |

---

## 9. Error Handling

### `.catch(fn)`
Catches errors from its receiver expression chain. Non-errors pass through. Void and `deleted()` pass through (not errors).

```
expr.catch(err -> fallback)             # err is {"what": "message string"}
```

All runtime errors are catchable except recursion limit exceeded.

**Scope:** catches any error from the entire postfix chain to its left. Errors skip subsequent postfix operations until reaching `.catch()`.

### `.or(default)`
Returns default for null, void, or `deleted()`. Short-circuit: argument only evaluated if needed. Does NOT catch errors — errors propagate through.

```
input.name.or("Anonymous")
(if false { 1 }).or(0)                  # rescues void
deleted().or("fallback")                # rescues deleted
(5 / 0).or(0)                           # ERROR propagates (not caught)
```

`.or()` and `.catch()` are the only methods callable on void/`deleted()`.

### `throw(message)`
Throws error with string message. Propagates like any error, catchable by `.catch()`. Uncaught = halts mapping.

---

## 10. Deletion (`deleted()`)

Returns a deletion marker.

**Triggers deletion (no error):**
- `output.field = deleted()` — removes field
- `output = deleted()` — drops message, exits mapping immediately
- `output@.key = deleted()` — removes metadata key
- `$var.field = deleted()` — removes field from variable's value
- `$arr[i] = deleted()` — removes element, shifts remaining down
- Collection literals: `[1, deleted(), 3]` → `[1, 3]`, `{"a": deleted()}` → `{}`
- Lambda returns in `.map()`, `.map_values()`, `.map_keys()`, `.map_entries()` — entry omitted; `.catch()` — flows to calling context normally

**Causes error:**
- `$var = deleted()` — variables must hold a value
- `output@ = deleted()` — metadata cannot be deleted
- Any operator: `deleted() + 5`, `deleted() == deleted()`
- Any method except `.or()` and `.catch()`
- As function/map argument

**deleted() vs void:** `deleted()` is intentional removal (accepted in collections/map lambdas). Void is missing code path (error in those contexts).

---

## 11. Grammar (Simplified)

```
program         := NL? (top_level_stmt (NL top_level_stmt)*)? NL?
top_level_stmt  := statement | map_decl | import_stmt
statement       := assignment | if_stmt | match_stmt
assignment      := assign_target '=' expression
assign_target   := 'output' '@'? path* | '$' ident path*

map_decl        := 'map' ident '(' params? ')' '{' NL? (var_asgn NL)* expression NL? '}'
params          := param (',' param)*
param           := ident | ident '=' literal | '_'
import_stmt     := 'import' string 'as' ident

expression      := postfix | binary | unary | lambda
postfix         := primary postfix_op*
primary         := literal | context_root | call | if_expr | match_expr | '(' expression ')'
context_root    := ('output'|'input') '@'? | '$' ident | qualified | ident

postfix_op      := '.' field | '?.' field | '[' expr ']' | '?[' expr ']'
                 | '.' word '(' args? ')' | '?.' word '(' args? ')'
field           := word | string
call            := (ident | qualified | reserved) '(' args? ')'
qualified       := ident '::' ident
reserved        := 'deleted' | 'throw'

if_expr         := 'if' expr '{' expr_body '}' ('else' 'if' expr '{' expr_body '}')* ('else' '{' expr_body '}')?
if_stmt         := 'if' expr '{' stmt_body '}' ('else' 'if' expr '{' stmt_body '}')* ('else' '{' stmt_body '}')?
expr_body       := (var_asgn NL)* expression
stmt_body       := (statement (NL statement)*)?
var_asgn        := '$' ident path* '=' expression

match_expr      := 'match' expr ('as' ident)? '{' expr_case (',' expr_case)* ','? '}'
                 | 'match' '{' expr_case (',' expr_case)* ','? '}'
match_stmt      := 'match' expr ('as' ident)? '{' stmt_case (',' stmt_case)* ','? '}'
                 | 'match' '{' stmt_case (',' stmt_case)* ','? '}'
expr_case       := (expr | '_') '=>' (expression | '{' expr_body '}')
stmt_case       := (expr | '_') '=>' '{' stmt_body '}'

binary          := expr op expr    # apply precedence/associativity from Section 3
unary           := ('!' | '-') expr
lambda          := lambda_params '->' (expression | '{' NL? (var_asgn NL)* expression NL? '}')
lambda_params   := ident | '_' | '(' param (',' param)* ')'
args            := positional | named
positional      := expr (',' expr)* ','?
named           := ident ':' expr (',' ident ':' expr)* ','?

literal         := float | int | string | bool | null | array | object
int             := [0-9]+
float           := [0-9]+ '.' [0-9]+
array           := '[' (expr (',' expr)* ','?)? ']'
object          := '{' NL? (expr ':' expr (',' NL? expr ':' expr)* ','?)? NL? '}'
word            := [a-zA-Z_][a-zA-Z0-9_]*
ident           := word - keywords - reserved
```

**Disambiguation:**
- After `match`, `{` is always the match body (not object literal).
- After `ident`, `(` means function call; otherwise context_root.
- Empty `{}` in expression context = empty object literal (blocks need at least one expression).

---

## 12. Standard Library

**Conventions:** All support named arguments. Numeric params accept any numeric type with promotion (whole-number check for integer params). Regex uses RE2. Unless noted, void/`deleted()` from lambdas = error.

### Functions

| Signature | Returns | Description |
|-----------|---------|-------------|
| `uuid_v4()` | string | Random UUID v4 |
| `now()` | timestamp | Current time (fresh each call) |
| `random_int(min, max)` | int64 | Random int in [min, max] |
| `range(start, stop, step?)` | array\<int64\> | start inclusive, stop exclusive. Step inferred if omitted (1 if start<=stop, -1 if start>stop). Zero step = error. Step contradicting direction = error. start==stop → empty array. |
| `timestamp(year, month, day, hour=0, minute=0, second=0, nano=0, timezone="UTC")` | timestamp | Construct from components |
| `second()` | int64 | 1000000000 (ns in 1s) |
| `minute()` | int64 | 60000000000 |
| `hour()` | int64 | 3600000000000 |
| `day()` | int64 | 86400000000000 |
| `throw(message)` | never | Throw error (string required) |
| `deleted()` | deletion marker | See Section 10 |

### Type Conversion Methods

| Method | Receiver | Returns | Notes |
|--------|----------|---------|-------|
| `.string()` | any | string | int→decimal, float→shortest round-trip (always has `.` or `e`), bool→"true"/"false", null→"null", timestamp→RFC3339, bytes→UTF-8 decode (error if invalid), array/object→compact JSON (keys sorted lexicographically by Unicode codepoint). Containers with bytes = error. |
| `.int32()` | numeric, string | int32 | Floats truncated toward zero. Out-of-range = error. |
| `.int64()` | numeric, string | int64 | Floats truncated toward zero. |
| `.uint32()` | numeric, string | uint32 | Negative = error. Floats truncated. |
| `.uint64()` | numeric, string | uint64 | Negative = error. Floats truncated. |
| `.float32()` | numeric, string | float32 | Unchecked precision loss. |
| `.float64()` | numeric, string | float64 | Unchecked precision loss. |
| `.bool()` | bool, string, numeric | bool | string: "true"/"false". numeric: 0=false, nonzero=true. NaN=error. |
| `.char()` | integer types | string | Codepoint → single-char string |
| `.bytes()` | any | bytes | String→UTF-8 bytes. Others→`.string()` then UTF-8. Containers with bytes = error. |
| `.type()` | any (incl null) | string | Returns type name |

### Sequence Methods (string, array, bytes)

| Method | Returns | Notes |
|--------|---------|-------|
| `.length()` | int64 | Codepoints / elements / bytes / keys(object) |
| `.contains(target)` | bool | String: substring (target: string). Array: element equality (target: any). Bytes: byte subsequence (target must be bytes). |
| `.index_of(target)` | int64 | First occurrence index, -1 if not found |
| `.slice(low, high?)` | same type | Inclusive low, exclusive high. Negative OK. Clamped to bounds. |
| `.reverse()` | same type | Reverses by unit (codepoint/element/byte) |

### String Methods

| Method | Signature | Returns |
|--------|-----------|---------|
| `.uppercase()` | | string |
| `.lowercase()` | | string |
| `.trim()` | | string (Unicode whitespace) |
| `.trim_prefix(p)` | `p`: string | string |
| `.trim_suffix(s)` | `s`: string | string |
| `.has_prefix(p)` | `p`: string | bool |
| `.has_suffix(s)` | `s`: string | bool |
| `.split(delim)` | `delim`: string | array\<string\>. `"".split("")`→`[]`, `"".split(",")`→`[""]` |
| `.replace_all(old, new)` | both string | string |
| `.repeat(count)` | `count`: int64 | string |
| `.re_match(pattern)` | RE2 | bool (matches any part) |
| `.re_find_all(pattern)` | RE2 | array\<string\> |
| `.re_replace_all(pattern, replacement)` | RE2 | string ($0, $1, ${name} expansion) |

### Array Methods

| Method | Signature | Returns | Notes |
|--------|-----------|---------|-------|
| `.filter(fn)` | elem→bool | array | Non-bool return = error |
| `.map(fn)` | elem→any | array | `deleted()`→omit element. Void = error. |
| `.sort()` | | array | Stable. Same sortable family required (numeric/string/timestamp only — bool, null, bytes, array, object are NOT sortable). |
| `.sort_by(fn)` | elem→comparable | array | Stable. Key extraction. |
| `.append(v)` | any | array | |
| `.concat(other)` | array | array | |
| `.flatten()` | | array | One level only |
| `.unique(fn?)` | elem→any (opt) | array | First occurrence kept. NaN treated as equal. |
| `.without_index(i)` | int64 | array | Removes at index, shifts. Negative counts from end. OOB = error. |
| `.enumerate()` | | array\<{index, value}\> | |
| `.any(fn)` | elem→bool | bool | Short-circuits on first true. Empty→false. |
| `.all(fn)` | elem→bool | bool | Short-circuits on first false. Empty→true. |
| `.find(fn)` | elem→bool | any or void | First match. No match→void. Short-circuits. |
| `.join(delim)` | string | string | All elements must be strings |
| `.sum()` | | numeric | All numeric, promoted. Empty→0 (int64) |
| `.min()` | | same type | Same sortable family. Empty = error. |
| `.max()` | | same type | Same sortable family. Empty = error. |
| `.fold(init, fn)` | (tally, elem)→any | any | |
| `.collect()` | | object | Array of `{"key": k, "value": v}` objects → object. `key` must be string. Missing key/value fields = error. Extra fields ignored. Last key wins. |

### Object Methods

| Method | Signature | Returns | Notes |
|--------|-----------|---------|-------|
| `.iter()` | | array\<{key, value}\> | Order not guaranteed |
| `.keys()` | | array\<string\> | Order not guaranteed |
| `.values()` | | array | Order not guaranteed |
| `.has_key(k)` | string | bool | |
| `.merge(other)` | object | object | `other` wins on conflict |
| `.without(keys)` | array\<string\> | object | Remove listed keys |
| `.map_values(fn)` | value→any | object | `deleted()`→omit. Void = error. |
| `.map_keys(fn)` | key→string | object | `deleted()`→omit. Must return string. Duplicate→last wins. |
| `.map_entries(fn)` | (k,v)→object with `"key"` (string) and `"value"` fields | object | `deleted()`→omit. Void = error. Duplicate keys→last wins. |
| `.filter_entries(fn)` | (k,v)→bool | object | |

### Numeric Methods

| Method | Receiver | Returns |
|--------|----------|---------|
| `.abs()` | any numeric | same type. Signed min overflow = error. |
| `.floor()` | float32/64 | same float type |
| `.ceil()` | float32/64 | same float type |
| `.round(n=0)` | float32/64 | same float type. Half-even rounding. Negative n = powers of 10. |

### Timestamp Methods

| Method | Signature | Returns | Notes |
|--------|-----------|---------|-------|
| `.ts_parse(fmt?)` | string receiver, fmt default RFC3339 | timestamp | Strftime directives: `%Y %m %d %H %M %S %f %z %Z %a %A %b %B %p %I %j %%`. `%f` optional fractional (1-9 digits, pads to ns). `%z` accepts Z or ±HH:MM. |
| `.ts_format(fmt?)` | timestamp receiver | string | Same directives. Default = RFC3339. |
| `.ts_unix()` | timestamp | int64 | Seconds |
| `.ts_unix_milli()` | timestamp | int64 | Milliseconds |
| `.ts_unix_micro()` | timestamp | int64 | Microseconds |
| `.ts_unix_nano()` | timestamp | int64 | Nanoseconds |
| `.ts_from_unix()` | any numeric | timestamp | Float for sub-second (~us precision). Integers widened to int64, float32 to float64. uint64 > int64 range = error. |
| `.ts_from_unix_milli()` | int64 | timestamp | Exact ms precision |
| `.ts_from_unix_micro()` | int64 | timestamp | Exact us precision |
| `.ts_from_unix_nano()` | int64 | timestamp | Exact ns precision |
| `.ts_add(nanos)` | timestamp, int64 | timestamp | Negative subtracts |

### Error Handling Methods

| Method | Notes |
|--------|-------|
| `.catch(fn)` | `fn`: (err → any). `err` is `{"what": "msg"}`. `deleted()`/void from handler flow normally. |
| `.or(default)` | Short-circuit. Rescues null, void, `deleted()`. Errors propagate through. |
| `.not_null(msg?)` | Returns value if not null, throws if null. Default msg: "unexpected null value". Regular method — cannot be called on void/`deleted()`. |

### Parsing Methods

| Method | Receiver | Returns | Notes |
|--------|----------|---------|-------|
| `.parse_json()` | string, bytes | any | JSON int→int64 (or float64 if > int64 range). JSON float→float64. |
| `.format_json(indent="", no_indent=false, escape_html=true)` | any (not bytes) | string | Keys sorted lexicographically by Unicode codepoint. Bytes anywhere in value = error. NaN/Inf = error. |
| `.encode(scheme)` | string, bytes | string | Schemes: "base64", "base64url", "base64rawurl", "hex" |
| `.decode(scheme)` | string | bytes | Same schemes |

---

## 13. Intrinsics

`.catch()`, `.or()`, `throw()`, and `deleted()` parse as regular calls but require special runtime handling:
- `.catch()`: activates on error (opposite of normal methods which skip on error)
- `.or()`: short-circuit evaluation; works on void/`deleted()`
- `throw()`: produces error
- `deleted()`: produces deletion marker tracked through assignments and collections
