# Bloblang V2 — Condensed Language Specification

Dynamically typed DSL for stream processing. Explicit context (`input`/`output`), no implicit coercion, fail-loudly semantics.

---

## 1. Lexical Structure

**Keywords:** `input output if else match as map import true false null _`
**Reserved functions:** `deleted` `throw` — parse as calls, cannot be identifiers. Valid as field names (`input.deleted`).
**Operators:** `. ?. @ :: = + - * / % ! > >= == != < <= && || => ->`
**Delimiters:** `( ) { } [ ] ?[ , :`
**Variables:** `$name`  **Metadata:** `input@.key` `output@.key`
**Comments:** `#` to EOL

**Literals:**
- Int64: `42` (overflow = compile error; `-10` is unary minus on `10`)
- Float64: `3.14` (digits required both sides of `.`; no `.5`, no `5.`, no `1e3`)
- String: `"escape\n\t\r\\\"\u00E9\u{1F600}"` or `` `raw verbatim` `` (no backtick escape)
- Bool: `true` `false`  Null: `null`  Array: `[1, 2,]`  Object: `{"k": v,}` (trailing commas OK)

**Identifiers:** `[a-zA-Z_][a-zA-Z0-9_]*` minus keywords/reserved. `_` only valid as discard param.
**Field names:** after `.`/`?.` accept any word incl keywords; `."quoted"` for special chars.

**Newline rules:** Statements separated by newlines. NL suppressed inside `()` and `[]`. Inside `{}` NL is significant (separates stmts) but object literal entries are comma-separated so NL ignored between them. NL suppressed when next token is postfix (`. ?. [ ?[ else`) or preceding token is an incomplete operator (`+ - * / % == != > >= < <= && || ! = => -> :`).

---

## 2. Types

| Type | Notes |
|------|-------|
| `string` | UTF-8, codepoint-based ops |
| `int32` `int64` `uint32` `uint64` | Default literal: int64 |
| `float32` `float64` | Default literal: float64 |
| `bool` | `true`/`false` |
| `null` | |
| `bytes` | Byte-based ops, no implicit JSON serialization |
| `array` | Ordered, heterogeneous |
| `object` | Key-value map, key order NOT preserved |
| `timestamp` | Nanosecond precision |

**Void** is not a type — it is absence of value (from if-without-else when false, match-without-`_` when no case matches, `.find()` on no match). Cannot be stored, passed, or used in expressions. In output field assignments and variable reassignments, void skips the assignment (no-op); in variable declarations (first assign), void is a runtime error. Only `.or()` (rescues void) and `.catch()` (passes void through unchanged) can be called on void. All other uses error. See §4 void table.

**Large uint64:** Literals are int64, so `"18446744073709551615".uint64()` for values > int64 max.
**String ops:** Codepoint-based, no normalization. Different Unicode representations ≠ equal.
**Object keys:** Order not preserved; equality ignores order; JSON output sorted lexicographically.

### Numeric Promotion

Applied to arithmetic/comparison/equality with mixed numeric operands:

| Operands | Promoted to | Error |
|----------|------------|-------|
| Same type | None | |
| Same sign, diff width | Wider | |
| Signed + unsigned int | int64 | uint64 > 2^63-1 |
| Any int + any float | float64 | int magnitude > 2^53 |

Promotion is **checked** — lossy conversions error at runtime. Integer overflow is always an error (no wrapping).

### Operator Type Rules

- `+`: same-family only (string+string, bytes+bytes, numeric+numeric). Cross-family = error.
- `-` `*` `%`: numeric only, null errors. Modulo by zero = error. Modulo uses truncated division remainder (C `fmod` for floats).
- `/`: always returns float (f32/f32→f32, all else→f64). Division by zero = error. No IEEE Inf/NaN from division.
- Comparison (`> >= < <=`): same sortable type required (numeric w/ promotion, timestamp, string lexicographic, bytes lexicographic). Null = error.
- Equality (`== !=`): numeric uses promotion; non-numeric requires same type+value; cross-family = `false`. Structural equality for arrays/objects.
- Logical (`&& || !`): booleans only, no truthy coercion.

**NaN:** Follows IEEE 754 (`NaN==NaN` is false, etc). `.sort()` uses total ordering (NaN after all). `.unique()` treats NaN as equal. `.bool()` on NaN = error.
**Negative zero:** `-0.0 == 0.0` true; `.string()` → `"0.0"`.
**Timestamps:** `ts - ts` → int64 nanos. All other ts arithmetic = error. Use `.ts_add(nanos)`.

### Null Handling

Null errors in arithmetic, comparison (except equality), and most methods. `null == null` is true. Cross-type equality with null returns false. Use `?.`/`?[]`/`?.method()` for null-safe navigation (short-circuits on null only, NOT on type errors). `.or(default)` provides fallback for null/void/deleted.

### Type Conversions

`.string()` `.int32()` `.int64()` `.uint32()` `.uint64()` `.float32()` `.float64()` `.bool()` `.bytes()` `.char()` `.type()`

Float→int conversions truncate toward zero. String parsing for numeric conversions. `.bool()`: string "true"/"false", numeric 0=false/nonzero=true (NaN=error). `.char()`: int codepoint → single-char string.

---

## 3. Expressions & Statements

### Path Expressions

Roots: `input`, `output`, `$variable`. In maps/lambdas: parameters as bare identifiers (read-only). In `match ... as x`: `x` as bare identifier (read-only).

**Name resolution:** Every bare identifier must resolve (compile-time): params > maps > stdlib functions. Unresolved = compile error. Map/function names valid only as calls `f()` or higher-order args `.map(f)` — not as values.

### Indexing

- Object: string key → value. Non-string = error.
- Array: integer → element. Must be whole number (2.0 OK, 1.5 error). Negative = from end. OOB = error.
- String: integer → int64 codepoint value. Use `.char()` to convert back.
- Bytes: integer → int64 byte value (0-255).
- All other types: error.

### Null-Safe Navigation

`?.` `?[]` `?.method()` — short-circuit to null when receiver is null. Type errors still throw.

### Operators

**Precedence** (high→low): postfix (`. ?. [] ?[] .method()`) → unary (`! -`) → `* / %` → `+ -` → `> >= < <=` → `== !=` → `&&` → `||`

Arithmetic/logical: left-associative. Comparison/equality: non-associative (chaining = parse error).
Method calls bind tighter than unary minus: `-10.string()` = `-(10.string())` = error. Use `(-10).string()`.
Lambda `->` is not in precedence; consumes entire RHS.

### Functions & Methods

Functions: `uuid_v4()`, `now()`, `random_int(1, 6)`. Methods: `input.text.uppercase()`.
**Named args:** `f(param1: v1, param2: v2)`. Cannot mix positional and named. Duplicate names = compile error.
**Method resolution:** compile-time against known methods. Unknown = compile error. Type compat = runtime check.

### Lambdas

Inline syntax for method args only — NOT values (`$fn = x -> x` is invalid). Use maps for reuse.

```
x -> expr                    # single param
(a, b) -> expr               # multi param
x -> { $v = x + 1           # block: stmts separated by newlines
       $v * 2 }             # must end with expression
_ -> expr                    # discard param
(_, v) -> v * 2              # partial discard
(x, y = 0) -> x + y         # defaults
```

Params: read-only, bare identifiers. Shadow map names. Purity: no output assignments. Context: inherits read permissions from enclosing scope (top-level lambda can read input/output; lambda inside map cannot).

### Statements

**Assignment:** `output.field = expr` (auto-creates intermediates). `output."quoted" = v`. `output = expr` replaces entire doc. `output@ = expr` (must be object). `output@.key = expr`.
**Variables:** `$var = expr`. Mutable, block-scoped. `$var.field = expr` (deep mutation with auto-creation). Assignment always creates logical copy (COW).
**Array gaps:** `$arr[5] = 30` on len-2 array fills nulls: `[v0, v1, null, null, null, 30]`.
**Deletion:** `output.f = deleted()` removes field. `output = deleted()` drops message + exits immediately. `$var = deleted()` = error. `output@ = deleted()` = error.

### Variable Scope & Shadowing

- **Expression contexts** (if/match exprs, lambdas, map bodies): assigning to outer var name **shadows** (new inner var, outer unchanged).
- **Statement contexts** (if/match stmts at top-level): assigning to existing outer var **modifies** it. New declarations are block-scoped.
- Same-scope reassignment is always mutation, not shadowing.

---

## 4. Control Flow

### If

**Expression** (in assignment RHS, var decl, lambda, map): contains expressions only, no output assignments.
**Statement** (top-level/inside stmt bodies): contains output assignments, cannot end with expression. Empty body OK.

Without `else`: produces **void** when false. Else-if without final else: void if no branch matches.

### Match

Three forms:
1. **Equality** `match expr { val => result, _ => default }` — evaluates expr once, compares each case by `==` in order; first match wins. Boolean case values = error (compile-time for literals, runtime for dynamic).
2. **Boolean with as** `match expr as x { x >= 50 => ..., _ => ... }` — cases must be boolean expressions, evaluated in order; first `true` wins.
3. **Boolean** `match { cond => ..., _ => ... }` — cases must be boolean, evaluated in order; first `true` wins.

`_` is unconditional catch-all in all forms. Not an expression — only valid as match case pattern.
Without `_`: produces void (expression) or is no-op (statement).

**Case bodies:** Expression match: bare expr or `{ expr_body }`. Statement match: always `{ stmt_body }`. Empty stmt bodies OK.

### Void Behavior Summary

| Context | Behavior |
|---------|----------|
| `output.x = void` | Skip assignment, preserve prior value |
| `$x = void` (first assign) | Runtime error |
| `$x = void` (reassign) | Skip, keep prior value |
| `[1, void, 3]` | Error |
| `{"a": void}` | Error |
| `f(void)` | Error |
| `void + 1` | Error |
| `void.type()` | Error |
| `void.or(x)` | Returns x |
| `void.catch(fn)` | Void passes through |
| `.map()` lambda returns void | Error |
| `.filter()` lambda returns void | Error |
| Other lambda returns void | Propagates to consuming context |

---

## 5. Maps (User-Defined Functions)

```
map name(param1, param2 = "default") {
  $local = param1.field
  param2 + $local  # final expression = return value
}
```

- **Isolated:** No access to `input`, `output`, or top-level `$vars`. Only params + local vars.
- **Params:** Read-only. Bare identifiers. Defaults must be literals, after required params. `_` discards (cannot have defaults).
- **Calling:** Positional or named args (not mixed). `_` params restrict to positional only.
- **Arity:** Positional: [required, total]. Named: must provide all required; missing defaults filled. Mismatch = error.
- **Shadowing:** Param names shadow map names within body. `namespace::name` unambiguous.
- **Recursion:** Self and mutual (same file). Min 1000 depth. Exceeding = uncatchable error.
- **As higher-order args:** `input.items.map(double)` = sugar for `map(x -> double(x))`. Compile-time resolution, not runtime values. Cannot store in vars.
- **Void from body:** If final expr is if-without-else or match-without-`_`, map can produce void.

---

## 6. Imports

```
import "./path.blobl" as ns
ns::map_name(arg)
```

- Relative to importing file. Absolute paths used as-is.
- Imported files: only map decls and imports. No top-level statements.
- All maps auto-exported. Duplicate namespace = error. Circular imports = compile error.
- Mutual recursion across files impossible (no circular imports).

---

## 7. Execution Model

**Input:** Immutable (any type). **Output:** Mutable, starts as `{}`. **Metadata:** `input@`/`output@` always objects, start as `{}`.

- Non-existent fields return null. `output@ = deleted()` = error. `output@ = {}` clears metadata.
- `output = input` is logical copy (COW). Mutations independent.
- Root `output = value` replaces entire doc (any type).
- Statements execute top-to-bottom. Maps hoisted (usable before declaration). Duplicate map names = compile error.

### Scoping

| Context | Read input | Read output | Write output/@ | Vars |
|---------|-----------|-------------|----------------|------|
| Top-level | ✅ | ✅ | ✅ | ✅ |
| Map body | ❌ | ❌ | ❌ | local only |
| Expr ctx (top-level) | ✅ | ✅ | ❌ | ✅ |
| Expr ctx (in map) | ❌ | ❌ | ❌ | map's local |
| Match `as` binding | ← enclosing expr ctx | ← enclosing expr ctx | ❌ | read-only `as` var + enclosing |

---

## 8. Error Handling

**Propagation:** Errors skip subsequent postfix ops until `.catch()`. The scope of `.catch()` is the entire postfix chain to its left — if any method, field access, or index in the chain errors, all subsequent postfix ops are skipped and the error flows to `.catch()`. Use parentheses to narrow scope: `(expr).catch(fn)`.

### `.catch(fn)`
Catches errors from its full receiver expression (the entire postfix chain to the left). `fn` receives `{"what": "message"}`; handler may return `deleted()`/void (flow to calling context normally). Void and `deleted()` pass through unchanged (not errors). All runtime errors catchable except recursion limit.

### `.or(default)`
Rescues null, void, and `deleted()`. Exactly 1 arg (0 or 2+ = compile error). Short-circuit: default only evaluated if receiver is null/void/deleted. Does NOT catch errors (errors propagate through).

### `throw(message)`
String arg required (non-string literal = compile error; dynamic non-string = runtime error). Produces catchable error. Uncaught = halts mapping.

### Null-Safe vs Error-Safe
- `?.`/`?[]`/`?.method()`: handle null, not errors or type mismatches
- `.catch()`: handles errors, not null
- `.or()`: handles null/void/deleted, not errors

---

## 9. Special Features

### Dynamic Fields
`input[$var]` `output[$key] = v` — string keys for objects, integer for arrays.

### Deletion (`deleted()`)

**Triggers deletion (no error):** field assignment, root output (drops message + exits), metadata key, variable field (`$v.f = deleted()`), array index (`$arr[0] = deleted()` removes + shifts), collection literals, `.map()`/`.map_values()`/`.map_keys()`/`.map_entries()`/`.catch()` lambda return.

**Causes error:** variable assignment (`$var = deleted()`), `output@ = deleted()`, operators, method calls (except `.or()` `.catch()`), function args.

`deleted()` vs void: `deleted()` is intentional removal (accepted in collections/map lambdas); void is missing code path (error in those contexts).
Array index deletion: negative indices from end, OOB = error, shifts remaining elements.

---

## 10. Grammar

```
program         := NL? (top_level_statement (NL top_level_statement)*)? NL?
top_level_statement := statement | map_decl | import_stmt
statement       := assignment | if_stmt | match_stmt
assignment      := assign_target '=' expression
assign_target   := 'output' '@'? path_component* | var_ref path_component*
var_assignment  := var_ref path_component* '=' expression

map_decl        := 'map' identifier '(' [param_list] ')' '{' NL? (var_assignment NL)* expression NL? '}'
param_list      := param (',' param)*
param           := identifier | identifier '=' literal | '_'
import_stmt     := 'import' string_literal 'as' identifier

expression      := postfix_expr | binary_expr | unary_expr | lambda_expr
postfix_expr    := primary_expr postfix_op*
primary_expr    := literal | context_root | call_expr | if_expr | match_expr | '(' expression ')'
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

binary_expr     := expression binary_op expression
binary_op       := '+' | '-' | '*' | '/' | '%' | '==' | '!=' | '>' | '>=' | '<' | '<=' | '&&' | '||'
unary_expr      := ('!' | '-') expression
lambda_expr     := lambda_params '->' (expression | '{' NL? (var_assignment NL)* expression NL? '}')
lambda_params   := identifier | '_' | '(' param (',' param)* ')'

literal         := float_literal | int_literal | string_literal | 'true' | 'false' | 'null' | array | object
int_literal     := [0-9]+
float_literal   := [0-9]+ '.' [0-9]+
string_literal  := '"' string_char* '"' | '`' [^`]* '`'
string_char     := [^"\\\n] | '\\' ('"' | '\\' | 'n' | 't' | 'r' | 'u' hex{4} | 'u{' hex+ '}')
                   # Surrogate codepoints (U+D800–U+DFFF) invalid in both \u forms
array           := '[' (expression (',' expression)* ','?)? ']'
object          := '{' NL? (expression ':' expression (',' NL? expression ':' expression)* ','?)? NL? '}'
arg_list        := expression (',' expression)* ','? | identifier ':' expression (',' identifier ':' expression)* ','?
word            := [a-zA-Z_][a-zA-Z0-9_]*
identifier      := word - keyword - reserved_name
reserved_name   := 'deleted' | 'throw'
```

**Key disambiguation:**
- After `match`, `{` is always match body, not object literal.
- `identifier (` = call; `identifier` alone = context_root. Reserved names (`deleted`/`throw`) always require `(`.
- `{}` in expression context = empty object (blocks need ≥1 expression).
- Field/method names use `word` (incl keywords); declarations use `identifier` (excl keywords).
- Semantic pass enforces: equality match boolean cases = error; `as` match cases must be boolean.
- Object keys must evaluate to string at runtime.
- Map decls and imports: top-level only. `_` params restrict map/lambda to positional calls only.
- Bare identifiers must resolve (compile-time): params > maps > stdlib. Map/function names only valid as calls or higher-order args.

---

## 11. Standard Library

Named args supported for all stdlib using documented param names. Parameters accept promoted numeric types (int params accept whole-number floats). Regex uses RE2 syntax. Unless stated, void/`deleted()` as lambda returns = error.

### Functions

| Function | Params | Returns | Notes |
|----------|--------|---------|-------|
| `uuid_v4()` | — | string | Random UUID v4 |
| `now()` | — | timestamp | Fresh each call |
| `random_int(min, max)` | int64, int64 | int64 | Inclusive [min,max]. min>max = error |
| `range(start, stop, step?)` | int64, int64, int64? | [int64] | [start,stop). Step inferred: 1 if start≤stop, -1 if start>stop. step=0 or contradicts direction = error. start==stop → `[]`. |
| `timestamp(year, month, day, hour=0, minute=0, second=0, nano=0, timezone="UTC")` | ... | timestamp | Component ranges validated |
| `second()` `minute()` `hour()` `day()` | — | int64 | Nanosecond constants (1000000000, 60000000000, 3600000000000, 86400000000000) |
| `throw(message)` | string | never | Non-string literal = compile error; dynamic non-string = runtime error |
| `deleted()` | — | deletion marker | See §9 |

### Type Conversion Methods

| Method | Receiver | Returns | Notes |
|--------|----------|---------|-------|
| `.string()` | any | string | int→decimal, float→shortest roundtrip (incl `.` or `e`), bool→"true"/"false", null→"null", ts→RFC3339, bytes→UTF-8 decode (error if invalid), array/object→compact JSON (keys sorted lexicographically by codepoint). Containers with bytes = error. |
| `.int32()` `.int64()` `.uint32()` `.uint64()` | numeric, string | respective type | Float truncates toward zero. Range check. |
| `.float32()` `.float64()` | numeric, string | respective type | Explicit = unchecked (caller accepts precision loss) |
| `.bool()` | bool, string, numeric | bool | "true"/"false", 0=false/nonzero=true, NaN=error, -0.0=false, ±Inf=true |
| `.bytes()` | any | bytes | String→UTF-8 encoding, others→`.string().bytes()`. Containers with bytes = error. |
| `.char()` | integer | string | Codepoint → single-char string |
| `.type()` | any incl null | string | Returns type name |

### Sequence Methods (string, array, bytes)

| Method | Returns | Notes |
|--------|---------|-------|
| `.length()` | int64 | Codepoints / elements / bytes / keys (also works on object) |
| `.contains(target)` | bool | Substring / element equality / byte subsequence |
| `.index_of(target)` | int64 | First occurrence index, -1 if not found |
| `.slice(low, high?)` | same type | [low,high), high defaults to end. Negative indices. Clamped to bounds. |
| `.reverse()` | same type | |

### String Methods

| Method | Params | Returns | Notes |
|--------|--------|---------|-------|
| `.uppercase()` `.lowercase()` | — | string | |
| `.trim()` | — | string | Unicode whitespace |
| `.trim_prefix(prefix)` `.trim_suffix(suffix)` | string | string | No-op if not present |
| `.has_prefix(p)` `.has_suffix(s)` | string | bool | |
| `.split(delim)` | string | [string] | `"".split("")` → `[]`; `"".split(",")` → `[""]` |
| `.replace_all(old, new)` | string, string | string | |
| `.repeat(count)` | int64 | string | count<0 = error |
| `.re_match(pattern)` | string | bool | Matches any part |
| `.re_find_all(pattern)` | string | [string] | Non-overlapping |
| `.re_replace_all(pattern, replacement)` | string, string | string | RE2 expansion: `$0` `$1` `${name}` `$$` |

### Array Methods

| Method | Params | Returns | Notes |
|--------|--------|---------|-------|
| `.filter(fn)` | elem→bool | array | Must return bool |
| `.map(fn)` | elem→any | array | void=error, `deleted()`=omit element |
| `.sort()` | — | array | Stable, ascending. Same sortable family required (numeric/string/timestamp). NaN sorts last. |
| `.sort_by(fn)` | elem→comparable | array | Stable, by key |
| `.append(v)` | any | array | |
| `.concat(other)` | array | array | |
| `.flatten()` | — | array | One level |
| `.unique(fn?)` | elem→any? | array | By equality (or key fn). NaN = equal. First occurrence kept. |
| `.without_index(i)` | int64 | array | Removes + shifts. Negative OK. OOB = error. |
| `.enumerate()` | — | [{"index":i,"value":v}] | |
| `.any(fn)` | elem→bool | bool | Short-circuits on first true. Empty = false. **Required semantic.** |
| `.all(fn)` | elem→bool | bool | Short-circuits on first false. Empty = true. **Required semantic.** |
| `.find(fn)` | elem→bool | any or void | No match = void. Use `.or()` for fallback. **Required short-circuit.** |
| `.join(delim)` | string | string | All elements must be strings |
| `.sum()` | — | numeric | Pairwise promotion. Empty = 0 (int64). Non-numeric = error. |
| `.min()` `.max()` | — | same type | Same sortable family. Empty = error. |
| `.fold(init, fn)` | any, (tally,elem)→any | any | |
| `.collect()` | — | object | Array of `{"key":str,"value":any}` → object. Last wins on dupes. Extra fields ignored. Error if element missing key/value or key not string. |

### Object Methods

| Method | Params | Returns | Notes |
|--------|--------|---------|-------|
| `.iter()` | — | [{"key":k,"value":v}] | Order not guaranteed |
| `.keys()` | — | [string] | Order not guaranteed |
| `.values()` | — | [any] | Order not guaranteed (same as `.keys()` within single call) |
| `.has_key(key)` | string | bool | |
| `.merge(other)` | object | object | `other` wins on conflict |
| `.without(keys)` | [string] | object | Missing keys ignored |
| `.map_values(fn)` | val→any | object | void=error, `deleted()`=omit entry |
| `.map_keys(fn)` | key→string\|deleted | object | void=error, `deleted()`=omit, non-string=error. Dupes: last wins. |
| `.map_entries(fn)` | (k,v)→{"key":s,"value":any} | object | void=error, `deleted()`=omit. Dupes: last wins. |
| `.filter_entries(fn)` | (k,v)→bool | object | |

### Numeric Methods

| Method | Receiver | Returns | Notes |
|--------|----------|---------|-------|
| `.abs()` | numeric | same type | Most-negative signed int = overflow error |
| `.floor()` | float | same float | |
| `.ceil()` | float | same float | |
| `.round(n=0)` | float | same float | Half-even (banker's). Negative n rounds to powers of 10. |

### Timestamp Methods

| Method | Params | Returns | Notes |
|--------|--------|---------|-------|
| `.ts_parse(fmt="%Y-%m-%dT%H:%M:%S%f%z")` | string (strftime) | timestamp | Receiver: string. |
| `.ts_format(fmt="%Y-%m-%dT%H:%M:%S%f%z")` | string (strftime) | string | `%f`: shortest fractional, omitted if zero. `%z`: `Z` for UTC, `±HH:MM` otherwise. |
| `.ts_unix()` | — | int64 | Seconds |
| `.ts_unix_milli()` | — | int64 | Milliseconds |
| `.ts_unix_micro()` | — | int64 | Microseconds |
| `.ts_unix_nano()` | — | int64 | Nanoseconds |
| `.ts_from_unix()` | — | timestamp | Receiver: numeric. Float for sub-second (~μs precision). |
| `.ts_from_unix_milli()` | — | timestamp | Receiver: int64. Exact ms. |
| `.ts_from_unix_micro()` | — | timestamp | Receiver: int64. Exact μs. |
| `.ts_from_unix_nano()` | — | timestamp | Receiver: int64. Exact ns. Lossless roundtrip with `.ts_unix_nano()`. |
| `.ts_add(nanos)` | int64 | timestamp | Negative = subtract. Use `second()` etc. |

**Required strftime directives:** `%Y %m %d %H %M %S %f %z %Z %a %A %b %B %p %I %j %%`

### Error Handling Methods

| Method | Notes |
|--------|-------|
| `.catch(fn)` | fn receives `{"what": msg}`. void/deleted pass through. Handler may return deleted/void. |
| `.or(default)` | Short-circuit. Rescues null/void/deleted. Does NOT catch errors. Exactly 1 arg. |
| `.not_null(message="unexpected null value")` | Returns value if non-null, throws message if null. Cannot be called on void/deleted (regular method). |

### Parsing Methods

| Method | Receiver | Returns | Notes |
|--------|----------|---------|-------|
| `.parse_json()` | string, bytes | any | JSON int→int64 (or float64 if >int64 range), JSON float→float64 |
| `.format_json(indent="", no_indent=false, escape_html=true)` | any (not bytes) | string | Keys sorted. Bytes at any depth = error. NaN/Inf = error. Ints→JSON int (no decimal), floats→shortest roundtrip (always incl `.` or `e`). Large uint64>2^53 serialized as-is. |
| `.encode(scheme)` | string, bytes | string | `"base64"` `"base64url"` `"base64rawurl"` `"hex"` |
| `.decode(scheme)` | string | bytes | Same schemes |

---

## 12. Implementation Notes

- `.catch()`, `.or()`, `throw()`, `deleted()` are **intrinsics** — parsed as calls/methods but require special runtime handling (error interception, short-circuit eval, deletion markers).
- **Lazy evaluation** optional: `.filter()` `.map()` may use iterators. Lazy `.map()` must handle `deleted()` returns (omit element). Must materialize at variable/output assignment, indexing, terminal methods. Observable behavior must match eager eval.
- **Early termination** for `.any()`, `.all()`, `.find()` is **required**, not optional.
- Error messages should include file:line:col, description, suggested fix.
- Float `.string()`/`.format_json()` may vary across implementations (different shortest-repr algorithms). Compare parsed values, not strings, in conformance tests.
