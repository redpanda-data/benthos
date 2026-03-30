# Bloblang V2 — Condensed Language Specification

A domain-specific mapping language for stream processing. Dynamically typed, explicit context, functional core with imperative escape hatches.

---

## 1. Lexical Structure

**Keywords:** `input`, `output`, `if`, `else`, `match`, `as`, `map`, `import`, `true`, `false`, `null`, `_`

**Reserved function names:** `deleted`, `throw` — parsed as function calls with special semantics; cannot be used as identifiers (variable/map/parameter names). Valid as field names (`input.deleted`).

**Identifiers:** `[a-zA-Z_][a-zA-Z0-9_]*` excluding keywords and reserved names. `_` alone is not a valid identifier but is allowed as a discard parameter.

**Field names:** After `.`/`?.`, any `[a-zA-Z_][a-zA-Z0-9_]*` including keywords works without quoting. Use `."quoted"` for special characters/spaces/digits-first.

**Variables:** `$name`

**Metadata:** `input@.key` (read), `output@.key` (write)

**Operators:** `.` `?.` `@` `::` `=` `+` `-` `*` `/` `%` `!` `>` `>=` `==` `!=` `<` `<=` `&&` `||` `=>` `->`

**Delimiters:** `( ) { } [ ] ?[ , :`

**Literals:**
- Integers: `42` (int64). No exponent notation. `-10` is unary minus applied to `10`. Overflow is compile error.
- Floats: `3.14` (float64). Requires digits on both sides of `.` — no `.5` or `5.`.
- Strings: `"hello\n"` with escapes `\\ \" \n \t \r \uXXXX \u{X...}`, or `` `raw` `` (verbatim, no escapes, cannot contain backtick). `\uXXXX` is 4 hex digits, BMP only (U+0000-U+FFFF). `\u{X...}` is 1-6 hex digits, any plane. Surrogate codepoints (U+D800-U+DFFF) are invalid in both forms.
- Booleans: `true`, `false`
- Null: `null`
- Arrays: `[1, 2, 3]` (trailing comma OK)
- Objects: `{"key": value}` (trailing comma OK, keys must evaluate to string at runtime, key order not preserved)

**Comments:** `#` to end-of-line

**Statement separation:** Newlines separate statements. Newlines suppressed inside `()` and `[]`. Inside `{}` newlines are significant (separate statements in blocks; object literal entries use commas). Newlines suppressed when next line starts with postfix token (`. ?. [ ?[ else`) or preceding token is a binary/unary operator, `=`, `=>`, `->`, or `:`.

---

## 2. Type System

### Runtime Types

| Type | Examples |
|------|---------|
| `string` | `"hello"` (codepoint-based ops) |
| `int32` | `42.int32()` |
| `int64` | `42` (default integer literal) |
| `uint32` | `42.uint32()` |
| `uint64` | `"18446744073709551615".uint64()` |
| `float32` | `3.14.float32()` |
| `float64` | `3.14` (default float literal) |
| `bool` | `true`, `false` |
| `null` | `null` |
| `bytes` | `"hello".bytes()` (byte-based ops) |
| `array` | `[1, "two", true]` |
| `object` | `{"key": "value"}` (key order not preserved) |
| `timestamp` | `now()`, `"2024-03-01".ts_parse("%Y-%m-%d")` |

**Void** is not a runtime type — it is the absence of a value, produced by if-without-else (false condition), match-without-`_` (no match), and `.find()` (no match). Cannot be stored, used in expressions, or included in collections. Only meaningful in assignments (causes no-op) and rescued by `.or()`. See Section 6 for full rules.

### Numeric Promotion

When binary operators mix numeric types, both operands are promoted:

| Mix | Promoted to | Error condition |
|-----|-------------|-----------------|
| Same type | No promotion | — |
| Same sign, different width | Wider | — |
| Signed + unsigned int | int64 | uint64 > 2^63-1 |
| Any int + any float | float64 | int magnitude > 2^53 |

Promotion is **checked** — lossy promotions error at runtime. Integer overflow is always a runtime error (no wrapping).

### Operators

**`+`**: string+string=concat, bytes+bytes=concat, numeric+numeric=addition (promoted). Cross-family mix is an error.

**Arithmetic** (`- * %`): numeric only (with promotion). **Division** (`/`): always returns float (float32/float32→float32, all else→float64). Division/modulo by zero is an error.

**Comparison** (`> >= < <=`): same sortable type family only (numeric with promotion, strings lexicographic, timestamps chronological, bytes lexicographic). Null errors.

**Equality** (`== !=`): numeric types promoted then compared by value. Non-numeric: same type and value. Cross-family always `false`.

**Logical** (`! && ||`): require booleans. No truthy/falsy coercion.

**Precedence** (high→low): postfix (`. ?. [] ?[] .method()`) > unary (`! -`) > multiplicative (`* / %`) > additive (`+ -`) > comparison > equality > `&&` > `||`

**Associativity:** arithmetic and logical are left-associative. Comparison and equality are **non-associative** (chaining like `a < b < c` is a parse error).

**Unary minus trap:** `-10.string()` parses as `-(10.string())` (error). Use `(-10).string()`.

### Special Float Values

NaN/Infinity may enter via input data. IEEE 754 semantics: `NaN == NaN` is false. Division by zero is always an error (no Infinity production). `.sort()` uses total ordering for NaN (sorts after all others). `.bool()` on NaN is an error.

### Null Handling

Null errors in arithmetic, comparison, and most methods. Equality works (`null == null` is true). Null-safe operators (`?.`, `?[]`, `?.method()`) short-circuit to null on null receivers only (not on type errors). `.or()` provides defaults for null/void/deleted.

---

## 3. Expressions

### Path Expressions

**Roots:** `input` (immutable), `output` (mutable), `$variable`, bare identifiers (parameters, `as` bindings — read-only, expression context only).

**Name resolution:** Every bare identifier must resolve to: parameter > map name > stdlib function. Unresolved = compile error. Map/function names only valid as calls `f(x)` or higher-order arguments `.map(f)`, not as values.

### Indexing

- Array: `[n]` by number (whole numbers only, negative counts from end)
- Object: `["key"]` by string (dynamic field access)
- String: `[n]` returns int64 codepoint value. Use `.char()` to convert back.
- Bytes: `[n]` returns int64 byte value (0-255)

Out-of-bounds is a runtime error. Float index like `1.5` is an error; `2.0` is OK.

### Null-Safe Navigation

`?.field`, `?[index]`, `?.method()` — short-circuit to null if receiver is null. Type errors still throw.

### Function Calls

```
func(arg1, arg2)           # positional
func(name1: v1, name2: v2) # named (order doesn't matter)
# Cannot mix positional and named. Duplicate names = compile error.
```

### Method Calls

```
value.method(args)   # regular
value?.method(args)  # null-safe (skips if value is null)
```

Methods are resolved at compile time (unknown method = compile error). Type compatibility checked at runtime.

### Lambda Expressions

Inline syntax for method arguments only — cannot be stored in variables.

```
x -> x * 2                         # single param
(a, b) -> a + b                    # multi param
_ -> "constant"                    # discard param
(_, v) -> v * 2                    # partial discard (multiple _ OK)
x -> { $t = x * 2; $t + 1 }       # block body (must end with expression)
(x, y = 0) -> x + y               # default params (must be literals, after required)
```

Parameters are read-only, shadow map names. Lambdas cannot assign to `output`/`output@`. In expression contexts, assigning to outer variable name creates a shadow.

### Literals

**Object keys** must evaluate to string at runtime (error otherwise). **Trailing commas** allowed in arrays and objects.

---

## 4. Control Flow

### If Expression (returns value)

```
output.x = if cond { expr } else { expr }
output.x = if cond { expr }  # void if false → assignment skipped
output.x = if c1 { e1 } else if c2 { e2 } else { e3 }
```

Body is an expression context: variable assignments + final expression. No `output` assignments.

### If Statement (standalone, side effects)

```
if cond {
  output.x = value
  output.y = value
}
```

Body is a statement context: output/variable assignments. **Cannot** end with a bare expression (parse error). Empty body `{}` is valid (no-op).

### Match Expression (returns value)

Three forms:

**1. Equality match:** `match expr { value => result, _ => default }` — cases compared by `==`. Cases evaluated in order; first match wins and subsequent case expressions are **not evaluated**. Boolean case values are an error (catches bug of writing conditions in equality match). Boolean literals (`true`/`false`) as cases are a **compile-time** error; dynamic expressions that evaluate to boolean at runtime are **runtime** errors.

**2. Boolean match with `as`:** `match expr as x { x >= 80 => "high", _ => "low" }` — expression evaluated once, bound to variable. `as` binding available in cases, results, and statement bodies. Block-scoped to the match — cannot be referenced after match closes. Cases must be boolean (non-boolean case = error). `_` is exempt (always matches).

**3. Boolean match (no expression):** `match { cond1 => r1, _ => default }` — cases must be boolean.

`_` is unconditional catch-all in all forms — it is a syntactic form, not an expression (can only appear as a match case pattern). Without `_`, no match produces void.

### Match Statement (standalone)

Same three forms but case bodies **always require braces**: `"cat" => { output.sound = "meow" }` (unlike expression cases which allow bare expressions). Empty case bodies `=> { }` are valid (no-op).

### Parsing Disambiguation

Context determines form: top-level/inside statement body → statement form. Inside assignment RHS/variable decl/lambda/map → expression form.

---

## 5. Maps (User-Defined Functions)

```
map name(param1, param2, optional = "default") {
  $temp = param1 + param2
  $temp + optional  # final expression is return value
}
```

**Isolation:** Maps cannot access `input`, `output`, or top-level `$variables`. Only parameters and locally-declared variables.

**Parameters:** read-only, available as bare identifiers. `_` discards (not bound, multiple OK, no defaults). Defaults must be literals, come after required params.

**Calling:** positional or named args (not mixed). Maps with `_` params → positional only.

**Recursion:** Self and mutual recursion supported. Min 1000 depth. Exceeding limit is uncatchable error. Map declarations are **hoisted** — usable before declaration. Duplicate names in same file = compile error.

**As higher-order arguments:** `input.items.map(double)` is sugar for `map(x -> double(x))`. Compile-time references only, cannot be stored in variables.

---

## 6. Void & Deleted

### Void (absence of value)

Produced by: if-without-else (false), match-without-`_` (no match), `.find()` (no match).

| Context | Behavior |
|---------|----------|
| `output.x = void` | Assignment skipped, prior value preserved |
| `$x = void` (declaration — first assignment to name in scope) | **Runtime error** |
| `$x = void` (reassignment) | Skipped, prior value preserved |
| Collection literal `[1, void, 3]` | **Error** |
| Function/map argument | **Error** |
| `.map()` lambda return | **Error** |
| `.filter()` lambda return | **Error** |
| `.or(default)` receiver | Returns default (rescued) |
| `.catch()` receiver | Passes through (not an error) |
| Other method/operator | **Error** |

### Deleted (intentional removal marker)

`deleted()` returns a deletion marker.

**Triggers deletion (no error):**
- `output.field = deleted()` — removes field
- `output = deleted()` — drops message, **immediately exits mapping**
- `output@.key = deleted()` — removes metadata key
- `$var.field = deleted()` — removes field from variable's value
- `$arr[i] = deleted()` — removes element, shifts remaining down
- Array/object literals: `[1, deleted(), 3]` → `[1, 3]`
- `.map()` / `.map_values()` / `.map_keys()` / `.map_entries()` lambda return — omits element/entry

**Causes error:**
- `$var = deleted()` — variables must hold a value
- `output@ = deleted()` — metadata cannot be deleted
- Any operator: `deleted() + 5`, `deleted() == deleted()`
- Any method except `.or()` and `.catch()`
- As function argument

`.or()` rescues deleted (returns its argument). `.catch()` passes deleted through.

**Void vs deleted:** Void = missing code path (likely bug if in collection). Deleted = intentional removal. Collections accept deleted (removes element) but error on void.

---

## 7. Execution Model

### Input/Output

- `input` (document + metadata): immutable, any type
- `output`: starts as `{}`, built incrementally. `output = expr` replaces entire document.
- `output@`: starts as `{}`, always an object. `output@ = {}` clears. `output@ = deleted()` is error. Assigning a non-object to `output@` is error (`output@ = "string"`, `output@ = 42`, `output@ = [1,2]` all error).
- Non-existent fields return `null`. Explicit `null` vs absent: differ in JSON serialization.
- Assignment to nested paths auto-creates intermediate objects/arrays. Index gaps filled with `null`.
- `output = input` is a logical copy (COW). Variable assignment is always a logical copy.

### Metadata

- `input@.key` read, `output@.key` write. `input@`/`output@` without path = entire metadata object.
- Values can be any type. Nested paths auto-create intermediates.
- `output@ = input@` copies all metadata (COW).

### Scoping

| Context | Read input | Read output | Write output/output@ | Variables |
|---------|-----------|------------|---------------------|-----------|
| Top-level | Yes | Yes | Yes | Yes |
| Map body | No | No | No | Local only |
| Expression (top-level) | Yes | Yes | No | Yes |
| Expression (in map) | No | No | No | Map's local only |

**Variable scoping:** Block-scoped. In **statement** contexts (top-level if/match): assigning to existing outer var modifies it; new vars are block-scoped. In **expression** contexts (if/match expression, lambda, map): assigning to outer var name **shadows** it (new inner var). Same-scope reassignment is always mutation.

### Evaluation Order

Statements execute top-to-bottom. Maps are hoisted. Variables must be declared before use. Later statements can read earlier `output` fields (returns null if not yet assigned).

---

## 8. Error Handling

### `.catch(fn)`

Catches errors from entire left-hand expression chain. Normal methods are skipped on error; `.catch()` activates only on errors. Void and deleted pass through unchanged.

```
expr.catch(err -> fallback)        # err is {"what": "message"}
input.date.ts_parse("%Y-%m-%d")
  .catch(err -> input.date.ts_parse("%Y/%m/%d"))
  .catch(err -> null)
```

All runtime errors catchable except recursion limit.

### `.or(default)`

Returns default for null, void, or deleted. **Short-circuit:** argument only evaluated when needed. Errors propagate through `.or()` uncaught.

```
input.name.or("Anonymous")
input.name.or(throw("required"))    # throw only evaluated if null
(if false { "x" }).or("default")    # void rescued
```

### `throw(message)`

Produces a catchable error. Requires exactly one string argument. Uncaught = halts mapping.

---

## 9. Imports

```
import "./path.blobl" as namespace
namespace::map_name(args)
```

- Relative to importing file's directory. Absolute paths used as-is.
- Imported files may only contain map declarations and import statements (no top-level statements).
- All maps automatically exported.
- Circular imports: compile-time error.
- Duplicate namespace names: error.

---

## 10. Grammar

```
program         := NL? (top_level_statement (NL top_level_statement)*)? NL?
top_level_statement := statement | map_decl | import_stmt
statement       := assignment | if_stmt | match_stmt
assignment      := assign_target '=' expression
assign_target   := 'output' '@'? path_component* | '$' identifier path_component*
var_assignment  := '$' identifier path_component* '=' expression

map_decl        := 'map' identifier '(' param_list? ')' '{' NL? (var_assignment NL)* expression NL? '}'
param_list      := param (',' param)*
param           := identifier | identifier '=' literal | '_'
import_stmt     := 'import' string_literal 'as' identifier

expression      := postfix_expr | binary_expr | unary_expr | lambda_expr
postfix_expr    := primary_expr postfix_op*
primary_expr    := literal | context_root | call_expr | if_expr | match_expr | '(' expression ')'
context_root    := ('output' | 'input') '@'? | '$' identifier | qualified_name | identifier
postfix_op      := path_component | '.' word '(' arg_list? ')' | '?.' word '(' arg_list? ')'
path_component  := '.' field_name | '?.' field_name | '[' expression ']' | '?[' expression ']'
field_name      := word | string_literal
call_expr       := (identifier | qualified_name | reserved_name) '(' arg_list? ')'
qualified_name  := identifier '::' identifier

if_expr         := 'if' expression '{' NL? expr_body NL? '}'
                   ('else' 'if' expression '{' NL? expr_body NL? '}')*
                   ('else' '{' NL? expr_body NL? '}')?
if_stmt         := 'if' expression '{' NL? stmt_body NL? '}'
                   ('else' 'if' expression '{' NL? stmt_body NL? '}')*
                   ('else' '{' NL? stmt_body NL? '}')?
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
array           := '[' (expression (',' expression)* ','?)? ']'
object          := '{' NL? (expression ':' expression (',' NL? expression ':' expression)* ','?)? NL? '}'
arg_list        := expression (',' expression)* ','? | identifier ':' expression (',' identifier ':' expression)* ','?

word            := [a-zA-Z_][a-zA-Z0-9_]*
identifier      := word - keyword - reserved_name
keyword         := 'input' | 'output' | 'if' | 'else' | 'match' | 'as' | 'map' | 'import' | 'true' | 'false' | 'null' | '_'
reserved_name   := 'deleted' | 'throw'
```

**Disambiguation:** After `match`, `{` is always the match body (not an object literal). After `identifier`, `(` means call; otherwise context_root. `{}` in expression context = empty object (blocks require at least one expression).

---

## 11. Standard Library

All functions/methods support named arguments using parameter names shown below. Parameter type promotion: int params accept any whole number, float params accept any numeric type.

Regex: RE2 syntax (linear time, no backreferences/lookahead).

Lambda return values: void and `deleted()` are errors unless method explicitly supports them.

### Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `uuid_v4()` | string | Random UUID v4 |
| `now()` | timestamp | Current time (fresh each call) |
| `random_int(min, max)` | int64 | Random int in [min, max] |
| `range(start, stop, step?)` | array\<int64\> | Half-open range. Step inferred if omitted. Error if step=0 or contradicts direction |
| `timestamp(year, month, day, hour=0, minute=0, second=0, nano=0, timezone="UTC")` | timestamp | Construct from components |
| `second()` | int64 | 1000000000 (ns in 1s) |
| `minute()` | int64 | 60000000000 |
| `hour()` | int64 | 3600000000000 |
| `day()` | int64 | 86400000000000 |
| `throw(message)` | never | Throw error (string required). See Section 8 |
| `deleted()` | deletion marker | See Section 6 |

### Type Conversion Methods

| Method | Receiver | Returns | Notes |
|--------|----------|---------|-------|
| `.string()` | any | string | int→decimal, float→shortest round-trip repr (always has `.` or `e`), `-0.0`→`"0.0"`, NaN→`"NaN"`, Infinity→`"Infinity"`, `-Infinity`→`"-Infinity"`, bool→`"true"`/`"false"`, null→`"null"`, timestamp→RFC3339, bytes→UTF-8 decode (error if invalid), array/object→compact JSON (sorted keys). Containers with bytes error. |
| `.int32()` | numeric, string | int32 | Floats truncated toward zero |
| `.int64()` | numeric, string | int64 | Floats truncated toward zero |
| `.uint32()` | numeric, string | uint32 | Error if negative |
| `.uint64()` | numeric, string | uint64 | Error if negative |
| `.float32()` | numeric, string | float32 | Unchecked precision loss OK |
| `.float64()` | numeric, string | float64 | Unchecked precision loss OK |
| `.bool()` | bool, string, numeric | bool | string: `"true"`/`"false"` only. numeric: 0=false, non-zero=true. `-0.0`=false. Infinity/`-Infinity`=true. NaN=error |
| `.bytes()` | any | bytes | string→UTF-8. Others→`.string().bytes()`. Containers w/ bytes error |
| `.char()` | integer types | string | Codepoint → single character |
| `.type()` | any (incl null) | string | Returns type name: `"string"`, `"int64"`, `"float64"`, `"bool"`, `"null"`, `"bytes"`, `"timestamp"`, `"array"`, `"object"`, `"int32"`, `"uint32"`, `"uint64"`, `"float32"` |

### Sequence Methods (string/array/bytes)

| Method | Description |
|--------|-------------|
| `.length()` | Length (codepoints/elements/bytes/keys for objects) |
| `.contains(target)` | string: substring. array: element equality. bytes: byte subsequence |
| `.index_of(target)` | First index or -1 |
| `.slice(low, high?)` | Subsequence [low, high). Negative indices OK. Clamped to bounds. High defaults to length |
| `.reverse()` | Reverse sequence |

### String Methods

| Method | Description |
|--------|-------------|
| `.uppercase()` | To uppercase |
| `.lowercase()` | To lowercase |
| `.trim()` | Strip Unicode whitespace |
| `.trim_prefix(prefix)` | Remove prefix (no-op if absent) |
| `.trim_suffix(suffix)` | Remove suffix (no-op if absent) |
| `.has_prefix(prefix)` | bool |
| `.has_suffix(suffix)` | bool |
| `.split(delimiter)` | → array of strings. `"hello".split("")` → `["h","e","l","l","o"]` (splits by codepoint). `"".split("")` → `[]`. `"".split(",")` → `[""]` |
| `.replace_all(old, new)` | Replace all occurrences |
| `.repeat(count)` | Repeat string. Error if negative |
| `.re_match(pattern)` | bool — matches any part |
| `.re_find_all(pattern)` | All non-overlapping matches → array of strings |
| `.re_replace_all(pattern, replacement)` | Replace with RE2 expansion (`$0`, `$1`, `${name}`, `$$`) |

### Array Methods

| Method | Description |
|--------|-------------|
| `.filter(fn)` | Keep elements where `fn(elem)` → true. Must return bool |
| `.map(fn)` | Transform elements. Void = error. `deleted()` = omit element |
| `.sort()` | Ascending, stable. Same sortable family required (numeric/string/timestamp). NaN sorts last |
| `.sort_by(fn)` | Sort by key function. Stable |
| `.append(value)` | New array with value appended |
| `.concat(other)` | Concatenate two arrays |
| `.flatten()` | Flatten one level. Non-array elements kept as-is |
| `.unique(fn?)` | Deduplicate (first occurrence kept). Optional key function. NaN considered equal |
| `.without_index(index)` | Remove element at index, shift remaining. Out-of-bounds = error |
| `.enumerate()` | → `[{"index": i, "value": v}, ...]` |
| `.any(fn)` | **Short-circuits** on first true. Empty → false |
| `.all(fn)` | **Short-circuits** on first false. Empty → true |
| `.find(fn)` | First match, **short-circuits**. No match → **void**. Use `.or()` for fallback |
| `.join(delimiter)` | All elements must be strings → joined string |
| `.sum()` | Sum numeric elements (promoted). Empty → `0` (int64) |
| `.min()` | Minimum (same sortable family). Empty = error |
| `.max()` | Maximum (same sortable family). Empty = error |
| `.fold(initial, fn)` | `fn(tally, elem)` → accumulate |
| `.collect()` | `[{"key": k, "value": v}, ...]` → object. Last value wins on duplicates. Extra fields ignored. Error if element missing `"key"`/`"value"` or key is not string |

### Object Methods

| Method | Description |
|--------|-------------|
| `.iter()` | → `[{"key": k, "value": v}, ...]` (order not guaranteed) |
| `.keys()` | → array of strings (order not guaranteed) |
| `.values()` | → array (order not guaranteed, but corresponds to `.keys()` order within single call) |
| `.has_key(key)` | bool |
| `.merge(other)` | Merge objects, `other` wins on conflict |
| `.without(keys)` | Remove keys (array of strings). Missing keys ignored |
| `.map_values(fn)` | Transform values. `deleted()` = omit entry |
| `.map_keys(fn)` | Transform keys (must return string). `deleted()` = omit entry. Duplicate new keys: last wins |
| `.map_entries(fn)` | `fn(key, value)` → `{"key": k, "value": v}`. `deleted()` = omit. Duplicate keys: last wins |
| `.filter_entries(fn)` | `fn(key, value)` → bool. Keep entries where true |

### Numeric Methods

| Method | Description |
|--------|-------------|
| `.abs()` | Absolute value. Same type. Overflow errors for most-negative signed int |
| `.floor()` | Float only → same float type |
| `.ceil()` | Float only → same float type |
| `.round(n=0)` | Half-even rounding to n decimal places. Negative n rounds to powers of 10 |

### Time Methods

| Method | Description |
|--------|-------------|
| `.ts_parse(format?)` | string → timestamp. Default format: `"%Y-%m-%dT%H:%M:%S%f%z"` (RFC 3339) |
| `.ts_format(format?)` | timestamp → string. Same default format |
| `.ts_unix()` | → int64 seconds |
| `.ts_unix_milli()` | → int64 milliseconds |
| `.ts_unix_micro()` | → int64 microseconds |
| `.ts_unix_nano()` | → int64 nanoseconds |
| `.ts_from_unix()` | any numeric → timestamp (int for seconds, float for sub-second). uint64 > int64 range = error |
| `.ts_from_unix_milli()` | int64 → timestamp |
| `.ts_from_unix_micro()` | int64 → timestamp |
| `.ts_from_unix_nano()` | int64 → timestamp |
| `.ts_add(nanos)` | Add int64 nanoseconds to timestamp |

**strftime directives:** `%Y %m %d %H %M %S %f %z %Z %a %A %b %B %p %I %j %%`. `%f` is optional on parse (consumes `.` + 1-9 digits if present). `%z` accepts `Z` and `±HH:MM`/`±HHMM`.

**Timestamp arithmetic:** `timestamp - timestamp` → int64 nanoseconds. All other arithmetic with timestamps is an error. Use `.ts_add(nanos)` to offset.

### Error Handling Methods

| Method | Description |
|--------|-------------|
| `.catch(fn)` | `fn(err)` where err is `{"what": "..."}`. Void/deleted pass through. Handler may return `deleted()` or void — these flow to calling context with normal semantics |
| `.or(default)` | Default for null/void/deleted. Short-circuit evaluation. Errors pass through. Exactly one argument required (0 or 2+ = compile error) |
| `.not_null(message?)` | Assert non-null (error with message if null). Default: `"unexpected null value"`. Regular method — cannot be called on void/deleted (use `.or(throw(...))` for those) |

### Parsing Methods

| Method | Description |
|--------|-------------|
| `.parse_json()` | string/bytes → value (bytes must be valid UTF-8). Integers w/o decimal → int64 (float64 if exceeds range). Decimals/exponents → float64 |
| `.format_json(indent="", no_indent=false, escape_html=true)` | value → JSON string. Keys sorted lexicographically. Bytes/NaN/Infinity error. Timestamps → RFC 3339 strings |
| `.encode(scheme)` | string/bytes → string. Schemes: `"base64"`, `"base64url"`, `"base64rawurl"`, `"hex"` |
| `.decode(scheme)` | string → bytes. Same schemes |

---

## 12. Implementation Notes

- `.catch()` and `.or()` are intrinsics requiring special runtime handling (error interception, short-circuit evaluation). `throw()` and `deleted()` similarly require special handling.
- Lazy evaluation of `.filter()` and `.map()` is an optional optimization. Results must be identical to eager evaluation. Variables always hold materialized values.
- `.any()`, `.all()`, `.find()` short-circuiting is **required semantics**, not optional.
- Recursion limit: min 1000 depth. Exceeding is uncatchable.
- Object key order is not preserved. JSON output sorts keys lexicographically.
