# Bloblang V2 — Condensed Language Specification

A mapping language for stream processing. Input is immutable, output is built incrementally. Maps are isolated functions. Errors propagate until caught.

---

## 1. Lexical Structure

**Keywords:** `input output if else match as map import true false null _`
**Reserved function names (cannot be identifiers):** `deleted throw`
**Operators:** `. ?. @ :: = + - * / % ! > >= == != < <= && || => ->`
**Delimiters:** `( ) { } [ ] ?[ , :`
**Variables:** `$name`
**Metadata:** `input@.key` (read), `output@.key` (write)
**Comments:** `#` to end-of-line

**Identifiers:** `[a-zA-Z_][a-zA-Z0-9_]*` excluding keywords and reserved names. `_` is only valid as a discard parameter.
**Field names:** After `.`/`?.` accept any word including keywords. Use `."quoted"` for special chars/spaces.

**Literals:**
- Int64: `42` (no leading `.`, no `5.`, no `.5`, no exponent notation). Overflow = compile error. `-10` is unary minus applied to `10`.
- Float64: `3.14` (digits required both sides of `.`). Overflow = compile error.
- String: `"hello\n"` with escapes `\\ \" \n \t \r \uXXXX \u{X...}` (surrogate codepoints U+D800-U+DFFF invalid in both Unicode forms). Raw: `` `verbatim` `` (no escapes, no backtick inside).
- Bool: `true false`
- Null: `null`
- Array: `[1, 2, 3]` (trailing comma OK)
- Object: `{"k": v}` (trailing comma OK, keys are expressions that must eval to string at runtime)

**Statement separation:** Newlines separate statements. NL suppressed inside `()` and `[]`. Inside `{}` NL is significant (separates statements in blocks), but object literal entries are comma-separated so NL between them is whitespace. NL suppressed when next token is `.` `?.` `[` `?[` `else`, or when preceding token is a binary/unary op, `=`, `=>`, `->`, `:`.

---

## 2. Type System

Dynamically typed. Runtime types: `string` (UTF-8, codepoint-based ops), `int32`, `int64`, `uint32`, `uint64`, `float32`, `float64`, `bool`, `null`, `bytes` (byte-based ops), `array`, `object` (key order not preserved), `timestamp` (nanosecond precision).

**Void** is not a type — it's the absence of a value (from if-without-else when false, match-without-`_` when no case matches, `.find()` with no match). Cannot be stored, passed, or used in expressions. Only meaningful in assignments (skips the assignment) and rescued by `.or()`. `.catch()` passes void through (not an error). All other uses are errors.

**`deleted()`** is a deletion marker (not a type). Removes fields/elements when assigned. `output = deleted()` drops the message and exits mapping immediately. Variable assignment `$v = deleted()` is error. `.or()` rescues it; `.catch()` passes it through. All other operations on it are errors.

### Numeric Promotion

When binary ops mix numeric types, both promoted to common type:
- Same type → no promotion
- Same signedness, different width → wider (`int32 + int64 → int64`)
- Signed + unsigned → int64 (error if uint64 > 2^63-1)
- Any int + any float → float64 (error if int magnitude > 2^53)

Promotion is **checked** — lossy promotions error at runtime.

### Operators on Types

- `+`: both strings → concat; both bytes → concat; both numeric → add (with promotion); cross-family → error
- `-` `*` `%`: numeric only (with promotion).
- Integer overflow → error (all integer arithmetic: `+` `-` `*` `%`).
- `/`: always produces float (float32/float32 → float32, otherwise float64). Division by zero → error. Mod by zero → error.
- `%`: follows standard promotion (not division rule). Float mod uses truncated division (C `fmod`).
- `> >= < <=`: same comparable type only (numeric with promotion, strings lexicographic, bytes lexicographic, timestamps chronological). Null → error. Cross-type → error.
- `== !=`: numeric → promote then compare; non-numeric → same type+value required; cross-family → `false` (not error). `null == null` is `true`.
- `&& || !`: boolean only. `5 && true` → error.
- `timestamp - timestamp` → int64 (nanoseconds). All other timestamp arithmetic → error. Use `.ts_add(nanos)`.

**NaN:** `NaN == NaN` false; `NaN != NaN` true; all comparisons with NaN false; arithmetic with NaN → NaN. `.sort()` uses total ordering (NaN after all values). `.bool()` on NaN → error.

**Negative zero:** `-0.0 == 0.0` true. `.string()` → `"0.0"`.

### Equality Details

Arrays: element-by-element (order matters). Objects: key-value pairs (order irrelevant). `5 == 5.0` true (promotion). `5 == "5"` false (cross-family).

---

## 3. Expressions

### Precedence (high to low)
1. Postfix: `.field` `?.field` `[i]` `?[i]` `.method()` `?.method()`
2. Unary: `!` `-` (thus `-10.string()` parses as `-(10.string())` → error; use `(-10).string()`)
3. Multiplicative: `* / %`
4. Additive: `+ -`
5. Comparison: `> >= < <=` (non-associative: `a < b < c` → parse error)
6. Equality: `== !=` (non-associative)
7. Logical AND: `&&`
8. Logical OR: `||`

Arithmetic and logical are left-associative. Lambda `->` is not an operator — it consumes the entire RHS as body.

### Path Expressions

**Roots:** `input` `output` `$variable` (top-level). In maps/lambdas: bare parameter names (read-only). In `match...as x`: `x` (read-only).

**Indexing:** `obj["key"]` (string index on object), `arr[0]` (numeric on array, whole numbers only, `1.5` → error), `str[0]` (codepoint position → int64), `bytes[0]` (byte → int64). Negative indices count from end. Out-of-bounds → error.

**Null-safe:** `?.` `?[` `?.method()` short-circuit to `null` only on null receiver. Type errors still throw (`5?.uppercase()` → error).

**Name resolution:** Every bare identifier must resolve to: parameter > map name > stdlib function. Unresolved → compile error. Map/function names only valid as calls `f()` or higher-order args `.map(f)`, not as values.

### Functions & Methods

Functions: `uuid_v4()`, `now()`, etc. Methods: `value.method()`. Chaining: `x.a().b().c()`.

**Named args:** `f(a: 1, b: 2)`. Cannot mix positional and named. Duplicate names → compile error.
**Default params:** Must follow required params. Values must be literals. `_` params cannot have defaults.

Method names resolved at compile time (unknown → compile error). Type compat checked at runtime.

### Lambdas

Inline syntax for method arguments only. Not values — `$fn = x -> x` is invalid.

```
x -> expr
(a, b) -> expr
x -> {                         # block: var assignments + final expression
  $v = x + 1
  $v * 2
}
```

- Params are read-only bare identifiers
- `_` discards: `(_, v) -> v * 2`. Multiple allowed. Not bound.
- Default params: `(x, y = 0) -> x + y`
- Cannot assign to `output`/`output@`
- Inherit read permissions of enclosing context (top-level lambda can read input/output; lambda inside map cannot)
- In expression contexts, assigning to outer variable name → shadow (new inner variable)

### Conditional Expressions

**If expression** (in assignment RHS / expression context):
```
if cond { expr } else if cond2 { expr2 } else { expr3 }
```
Without `else`: false → void. Body is `(var_assignment NL)* expression`. Cannot contain output assignments.

**If statement** (top-level / statement context):
```
if cond {
  output.x = val
}
```
Body is statements. Cannot end with bare expression. Empty body OK (no-op).

Context determines form: RHS of `=`, lambda body, map body → expression. Top-level, inside statement body → statement.

**Match** — three forms:

1. **Equality:** `match expr { val => result, _ => default }` — cases evaluated in order; first `==` match wins, subsequent cases not evaluated. Boolean case values → error (compile-time for literals, runtime for dynamic). Use `as` for boolean conditions.
2. **Boolean with `as`:** `match expr as x { x >= 10 => "high", _ => "low" }` — each case must be boolean. `x` is block-scoped read-only binding.
3. **Boolean without expr:** `match { cond1 => r1, _ => r2 }` — each case must be boolean.

`_` is unconditional catch-all in all forms. Without `_`, no match → void (expression) or no-op (statement).

Statement match cases require braces: `"cat" => { output.sound = "meow" }`. Expression cases allow bare: `"cat" => "meow"` or braced blocks.

### Void Behavior Summary

| Context | Behavior |
|---------|----------|
| `output.x = void` | Skip assignment, prior value preserved |
| `$x = void` (declaration) | Runtime error |
| `$x = void` (reassignment) | Skip, prior value preserved |
| `[1, void, 3]` | Error |
| `{"a": void}` | Error |
| `f(void)` | Error |
| `.map()` lambda returns void | Error |
| `.filter()` lambda returns void | Error |
| `void.or(x)` | Returns `x` |
| `void.catch(fn)` | Void passes through |
| `void.anything_else()` | Error |
| `void + 1` | Error |

---

## 4. Statements & Assignments

```
output.field = expr          # field assignment (auto-creates intermediates)
output = expr                # replace entire output
output@.key = expr           # metadata assignment
$var = expr                  # variable declaration/reassignment
$var.field = expr            # variable path assignment
```

**Auto-creation:** `output.a.b.c = 1` creates `a`, `a.b` as objects. `output.items[0] = "x"` creates array. Index type determines: string → object, int → array. Collision with incompatible type → error.

**Array gaps:** `$arr[5] = 30` on `[10,20]` → `[10,20,null,null,null,30]`.

**Copy-on-write:** `$x = input.data` creates logical copy. Mutations to `$x` never affect input, and vice versa. Same for `output` reads into variables.

### Variable Scoping

Variables are block-scoped. Two contexts:

**Statement context** (top-level, if/match statements): assigning to existing outer variable → **modifies** it. New declarations are block-scoped (not visible outside).

**Expression context** (if/match expressions, lambdas, map bodies): assigning to existing outer variable name → **shadows** (new inner variable, outer unchanged).

Same-scope reassignment is always mutation in both contexts.

### Metadata

`output@` is always an object. `output@ = deleted()` → error. `output@ = "string"` → error. `output@ = {}` clears all. `input@` and `output@` without path → whole metadata object. Values can be any type. Undefined keys → null.

---

## 5. Maps (User-Defined Functions)

```
map name(param1, param2 = "default") {
  $temp = param1 + param2
  $temp * 2                    # final expression = return value
}
```

- **Isolated:** Cannot access `input`, `output`, or top-level `$variables`. Only parameters + locally declared variables.
- Parameters are **read-only**.
- `_` discard params allowed. Maps with `_` → positional calls only.
- Positional or named args (not mixed). Arity checked.
- **Hoisted:** Can call before declaration.
- Duplicate names in same file → compile error.
- **Recursion:** Self and mutual (same file). Min 1000 depth. Exceeding limit → uncatchable error.
- Map names shadow stdlib functions of same name.
- Map names can be passed to higher-order methods: `.map(my_map)` = `.map(x -> my_map(x))`. Compile-time sugar, not a value.

---

## 6. Imports

```
import "./path.blobl" as namespace
namespace::map_name(args)
```

- Relative to importing file. Absolute paths supported.
- Imported files may only contain map declarations and imports (no statements).
- All maps exported automatically.
- Circular imports → compile error. Duplicate namespace names → compile error.
- `namespace::name` can be used in calls and as higher-order args.

---

## 7. Execution Model

- `input` (document + metadata): immutable. Can be any type.
- `output` starts as `{}`. `output@` starts as `{}`.
- Statements execute top-to-bottom. Later statements can read earlier output.
- Non-existent fields → `null` (not error). JSON output: absent fields omitted, `null` fields serialized.
- `output = deleted()` drops message, exits mapping immediately.
- `output = input` logical copy (COW). `output@ = input@` likewise.

### Scoping Permissions

| Context | Read input | Read output | Write output | Variables |
|---------|-----------|-------------|-------------|-----------|
| Top-level | yes | yes | yes | yes |
| Map body | no | no | no | local only |
| Expr (top-level) | yes | yes | no | yes |
| Expr (in map) | no | no | no | map-local only |

---

## 8. Error Handling

### `.catch(fn)`

Catches errors from entire left-hand expression chain. If no error, returns value unchanged. Error object: `{"what": "message string"}`. Void and `deleted()` pass through (not errors). Handler lambda may return `deleted()` or void — these flow to the calling context with normal semantics. All runtime errors catchable except recursion limit.

```
expr.catch(err -> fallback)
input.date.ts_parse("%Y-%m-%d").catch(err -> null)
```

### `.or(default)`

Rescues `null`, void, and `deleted()`. Short-circuit: default only evaluated if needed. Does NOT catch errors (errors propagate through).

```
input.name.or("Anonymous")
(if false { "x" }).or("default")     # rescues void
input.name.or(throw("required"))     # throw only evaluated if null
```

### `throw(message)`

Produces error. Non-string literal arg → compile error; dynamic non-string arg → runtime error. Catchable with `.catch()`. Uncaught → halts mapping.

### `.not_null(message = "unexpected null value")`

Returns value if not null, throws error if null.

### Null-safe vs Error-safe

- `?.` `?[` `?.method()`: handle null only (type errors still throw)
- `.catch()`: handle errors only (null passes through as value)
- `.or()`: handle null/void/deleted only (errors propagate)

---

## 9. Deletion & Filtering

`deleted()` returns deletion marker.

**Triggers deletion (not error):**
- `output.field = deleted()` → removes field
- `output = deleted()` → drops message, exits mapping
- `output@.key = deleted()` → removes key
- `$var.field = deleted()` → removes field from var's value
- `$arr[i] = deleted()` → removes element, shifts remaining down
- In collections: `[1, deleted(), 3]` → `[1, 3]`; `{"a": deleted()}` → `{}`
- Lambda return in `.map()`, `.map_values()`, `.map_keys()`, `.map_entries()` → omit element/entry

**Causes error:**
- `$var = deleted()` (variable must hold value)
- `output@ = deleted()` (metadata can't be deleted)
- Binary ops, method calls (except `.or()` `.catch()`)
- Function arguments

**void vs deleted():** void = "no value produced" (missing code path), error in collections/map-lambdas. deleted() = "intentionally remove" (accepted in those contexts). Both rescued by `.or()`.

---

## 10. Grammar (Simplified)

```
program         := NL? (top_level_stmt (NL top_level_stmt)*)? NL?
top_level_stmt  := statement | map_decl | import_stmt

statement       := assignment | if_stmt | match_stmt
assignment      := assign_target '=' expression
assign_target   := 'output' '@'? path_component* | '$' identifier path_component*

map_decl        := 'map' identifier '(' param_list? ')' '{' NL? (var_assignment NL)* expression NL? '}'
param_list      := param (',' param)*
param           := identifier | identifier '=' literal | '_'
import_stmt     := 'import' string_literal 'as' identifier

var_assignment  := '$' identifier path_component* '=' expression

expression      := postfix_expr | binary_expr | unary_expr | lambda_expr
postfix_expr    := primary postfix_op*
primary         := literal | 'input' '@'? | 'output' '@'? | '$' identifier
                 | identifier | qualified_name | call_expr | if_expr | match_expr | '(' expression ')'
postfix_op      := '.' field_name | '?.' field_name | '[' expr ']' | '?[' expr ']'
                 | '.' word '(' args? ')' | '?.' word '(' args? ')'
call_expr       := (identifier | qualified_name | 'deleted' | 'throw') '(' args? ')'
qualified_name  := identifier '::' identifier

if_expr         := 'if' expr '{' expr_body '}' ('else' 'if' expr '{' expr_body '}')* ('else' '{' expr_body '}')?
if_stmt         := 'if' expr '{' stmt_body '}' ('else' 'if' expr '{' stmt_body '}')* ('else' '{' stmt_body '}')?
expr_body       := (var_assignment NL)* expression
stmt_body       := (statement (NL statement)*)?          # empty OK

match_expr      := 'match' expr? ('as' identifier)? '{' expr_case (',' expr_case)* ','? '}'
match_stmt      := 'match' expr? ('as' identifier)? '{' stmt_case (',' stmt_case)* ','? '}'
expr_case       := (expression | '_') '=>' (expression | '{' expr_body '}')
stmt_case       := (expression | '_') '=>' '{' stmt_body '}'

binary_expr     := expression binop expression    # apply precedence/associativity from Section 3
unary_expr      := ('!' | '-') expression
lambda_expr     := lambda_params '->' (expression | '{' NL? (var_assignment NL)* expression NL? '}')
lambda_params   := identifier | '_' | '(' param (',' param)* ')'

literal         := int | float | string | bool | null | array | object
args            := positional_args | named_args
positional_args := expression (',' expression)* ','?
named_args      := identifier ':' expression (',' identifier ':' expression)* ','?
field_name      := word | string_literal
word            := [a-zA-Z_][a-zA-Z0-9_]*
identifier      := word - keyword - reserved_name
```

**Key disambiguation:** After `match`, `{` is always the match body (not object literal). After identifier, `(` means call; otherwise context_root. `{}` in expression context = empty object (blocks need at least one expression).

---

## 11. Standard Library

### Functions

| Function | Params | Returns | Notes |
|----------|--------|---------|-------|
| `uuid_v4()` | none | string | Random UUID v4 |
| `now()` | none | timestamp | Current time (fresh each call) |
| `random_int(min, max)` | int64, int64 | int64 | Inclusive range. Error if min > max |
| `range(start, stop, step?)` | int64, int64, int64? | array[int64] | Half-open [start,stop). Step defaults to 1 (or -1 if start>stop). Zero step → error. Explicit step contradicting direction → error |
| `timestamp(year, month, day, hour=0, minute=0, second=0, nano=0, timezone="UTC")` | int64s + string | timestamp | Component construction |
| `second()` `minute()` `hour()` `day()` | none | int64 | Nanosecond constants (1e9, 6e10, 3.6e12, 8.64e13) |
| `throw(message)` | string | never | Custom error. Non-string → error |
| `deleted()` | none | deletion marker | See Section 9 |

### Type Conversion Methods

All accept numeric types and strings unless noted. Float→int truncates toward zero.

| Method | From | To | Notes |
|--------|------|----|-------|
| `.string()` | any | string | Int→decimal. Float→shortest round-trip repr with decimal/exponent. Bool→"true"/"false". Null→"null". Timestamp→RFC3339. Bytes→UTF-8 decode (error if invalid). Array/object→compact JSON (sorted keys). Containers with bytes→error. |
| `.int32()` | numeric, string | int32 | Out of range → error |
| `.int64()` | numeric, string | int64 | Out of range → error |
| `.uint32()` | numeric, string | uint32 | Negative → error |
| `.uint64()` | numeric, string | uint64 | Negative → error |
| `.float32()` | numeric, string | float32 | Unchecked (precision loss OK) |
| `.float64()` | numeric, string | float64 | Unchecked (precision loss OK) |
| `.bool()` | bool, string, numeric | bool | String: "true"/"false". Numeric: 0=false, nonzero=true. NaN→error |
| `.char()` | integer types | string | Codepoint → single-char string |
| `.bytes()` | any | bytes | String→UTF-8 encoding. Bytes→as-is. Others→`.string().bytes()` |
| `.type()` | any (incl null) | string | Returns type name |

### Sequence Methods (string, array, bytes)

| Method | Returns | Notes |
|--------|---------|-------|
| `.length()` | int64 | Strings: codepoints. Also works on objects (key count) |
| `.contains(target)` | bool | String: substring. Array: element equality. Bytes: subsequence |
| `.index_of(target)` | int64 | First occurrence index, or -1 |
| `.slice(low, high?)` | same type | Half-open. Negative indices from end. Clamped (no OOB error). low>=high → empty |
| `.reverse()` | same type | Reverse by unit (codepoint/element/byte) |

### String Methods

| Method | Params | Returns | Notes |
|--------|--------|---------|-------|
| `.uppercase()` | | string | |
| `.lowercase()` | | string | |
| `.trim()` | | string | Unicode whitespace |
| `.trim_prefix(p)` | string | string | No match → unchanged |
| `.trim_suffix(s)` | string | string | No match → unchanged |
| `.has_prefix(p)` | string | bool | |
| `.has_suffix(s)` | string | bool | |
| `.split(delim)` | string | array[string] | `"".split("")` → `[]`. `"".split(",")` → `[""]` |
| `.replace_all(old, new)` | string, string | string | |
| `.repeat(count)` | int64 | string | Negative → error |
| `.re_match(pattern)` | string (RE2) | bool | Matches any part |
| `.re_find_all(pattern)` | string (RE2) | array[string] | Non-overlapping |
| `.re_replace_all(pattern, repl)` | string, string | string | RE2 expansion: `$0` `$1` `${name}` |

### Array Methods

| Method | Params | Returns | Notes |
|--------|--------|---------|-------|
| `.filter(fn)` | elem→bool | array | Non-bool return → error |
| `.map(fn)` | elem→any | array | Void → error. `deleted()` → omit element |
| `.sort()` | | array | Stable. Single sortable type family (numeric/string/timestamp). NaN sorts last |
| `.sort_by(fn)` | elem→comparable | array | Stable. Key function |
| `.append(val)` | any | array | |
| `.concat(other)` | array | array | |
| `.flatten()` | | array | One level. Non-array elements kept as-is |
| `.unique(fn?)` | elem→any (optional) | array | First occurrence kept. NaN values considered equal |
| `.without_index(i)` | int64 | array | Remove + shift. Negative OK. OOB → error |
| `.enumerate()` | | array[{index,value}] | |
| `.any(fn)` | elem→bool | bool | Short-circuits on first true. Empty → false |
| `.all(fn)` | elem→bool | bool | Short-circuits on first false. Empty → true |
| `.find(fn)` | elem→bool | any or void | Short-circuits. No match → void (use `.or()`) |
| `.join(delim)` | string | string | All elements must be strings |
| `.sum()` | | numeric | All must be numeric (promoted). Empty → 0 (int64) |
| `.min()` | | same type | Single sortable family. Empty → error |
| `.max()` | | same type | Single sortable family. Empty → error |
| `.fold(init, fn)` | any, (tally,elem)→any | any | |
| `.collect()` | | object | Array of `{key:string, value:any}` → object. Last wins on dupes. Missing key/value or non-string key → error |

### Object Methods

| Method | Params | Returns | Notes |
|--------|--------|---------|-------|
| `.iter()` | | array[{key,value}] | Order not guaranteed |
| `.keys()` | | array[string] | Order not guaranteed |
| `.values()` | | array | Order not guaranteed, but same as `.keys()` within single call |
| `.has_key(key)` | string | bool | |
| `.merge(other)` | object | object | Other wins on conflicts |
| `.without(keys)` | array[string] | object | Missing keys ignored |
| `.map_values(fn)` | value→any | object | `deleted()` → omit. Void → error |
| `.map_keys(fn)` | key→string | object | `deleted()` → omit (checked before type check). Void → error. Non-string → error |
| `.map_entries(fn)` | (key,value)→{key,value} | object | `deleted()` → omit. Void → error |
| `.filter_entries(fn)` | (key,value)→bool | object | |

### Numeric Methods

| Method | Receiver | Returns | Notes |
|--------|----------|---------|-------|
| `.abs()` | any numeric | same type | Signed min value → overflow error |
| `.floor()` | float32/64 | same float type | |
| `.ceil()` | float32/64 | same float type | |
| `.round(n=0)` | float32/64 | same float type | Half-even (banker's). Negative n rounds to powers of 10 |

### Timestamp Methods

| Method | Params | Returns | Notes |
|--------|--------|---------|-------|
| `.ts_parse(fmt?)` | string (strftime) | timestamp | Default: `"%Y-%m-%dT%H:%M:%S%f%z"`. `%f`: optional on parse (consumes leading `.` + 1-9 digits if present, else matches nothing), shortest on format (omitted entirely when zero). `%z`: parse accepts `Z` `+HH:MM` `-HH:MM` `+HHMM` `-HHMM`; format emits `Z` for UTC, `±HH:MM` otherwise |
| `.ts_format(fmt?)` | string (strftime) | string | Default: RFC 3339 with shortest fractional seconds |
| `.ts_unix()` | | int64 | Seconds |
| `.ts_unix_milli()` | | int64 | Milliseconds |
| `.ts_unix_micro()` | | int64 | Microseconds |
| `.ts_unix_nano()` | | int64 | Nanoseconds |
| `.ts_from_unix()` | | timestamp | On any numeric. Float→sub-second (limited by float64 precision). uint64 > int64 range → error |
| `.ts_from_unix_milli()` | | timestamp | On int64 |
| `.ts_from_unix_micro()` | | timestamp | On int64 |
| `.ts_from_unix_nano()` | | timestamp | On int64. Lossless round-trip with `.ts_unix_nano()` |
| `.ts_add(nanos)` | int64 | timestamp | Negative subtracts. Use `second()` etc. |

**Strftime directives:** `%Y %m %d %H %M %S %f %z %Z %a %A %b %B %p %I %j %%`

### Parsing/Encoding Methods

| Method | Receiver | Params | Returns | Notes |
|--------|----------|--------|---------|-------|
| `.parse_json()` | string, bytes | | any | JSON ints → int64 (or float64 if > int64 range). JSON floats → float64 |
| `.format_json(indent="", no_indent=false, escape_html=true)` | any (not bytes) | string opts | string | Keys sorted lexicographically. Floats always include decimal or exponent (distinguishes from int). Bytes in value → error. NaN/Inf → error |
| `.encode(scheme)` | string, bytes | | string | "base64", "base64url", "base64rawurl", "hex" |
| `.decode(scheme)` | string | | bytes | Same schemes |

### Intrinsic Methods (special runtime handling)

- `.catch(fn)`: Intercepts errors (normal methods are skipped on error). Void and `deleted()` pass through.
- `.or(default)`: Short-circuit eval. Only method (with `.catch()`) callable on void/`deleted()`.
- `throw(msg)`, `deleted()`: Parsed as calls but need special tracking.

---

## 12. Implementation Notes

- Map declarations are hoisted (callable before declared).
- Variables must be declared before use.
- Object key ordering not preserved. JSON output sorts keys lexicographically.
- Timestamp JSON serialization: RFC 3339 with trimmed trailing fractional zeros.
- Recursion limit: min 1000. Exceeding → uncatchable error.
- Lazy evaluation of `.filter()` `.map()` is optional but must be semantically transparent.
- Regular expressions use RE2 syntax (linear time, no backreferences/lookahead).
- Float `.string()` uses shortest round-trip representation — cross-implementation variation acceptable.
