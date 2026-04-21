# Bloblang V1 — Migration Reference Specification

This document is a complete, self-contained description of Bloblang V1 as implemented in `internal/bloblang/` of `redpanda-data/benthos`. It is intended as the source of truth for tooling that reads, analyses, or rewrites V1 mappings — in particular, V1 → V2 migration. It is deliberately descriptive (what V1 *does*), not prescriptive (what V1 *should* do): every accepted construct, quirk, and legacy form should be documented here even when it is undesirable, because a migration tool must recognise it.

Sources reconciled for this spec:

- Official documentation: the `about/`, `arithmetic/`, `walkthrough/`, `advanced/` pages under `docs.redpanda.com/redpanda-connect/guides/bloblang/`, plus `configuration/interpolation/`.
- Reference implementation: `../bloblang/` (parser, query, mapping, field packages) and `public/bloblang/` (host API surface).
- Conformance-ish corpus: `../../../config/test/bloblang/*.yaml` and `*.blobl`, plus inline `_test.go` files alongside each parser and query package.

Where the implementation and docs disagree, the implementation wins and the docs claim is called out. See `README.md` in this directory for the full source list.

---

## 1. Overview

A Bloblang V1 source file is a **mapping**: an ordered sequence of statements that are executed once per input message to produce an output message. The output has two channels:

- **`root`** — the message payload (structured data: any JSON-compatible value, or raw bytes).
- **`meta`** — the message metadata (a map of keys to values).

Mappings read from:

- **`this`** — the (structured) input payload.
- **`@` / `meta(...)`** — input metadata.
- **`content()`** — the raw byte content of the input.
- **`$var`** — locally bound variables (`let name = expr`).
- Environment (`env`), clock (`now`, `timestamp_*`), batch (`batch_index`, `batch_size`), etc.

A mapping is **the whole file**. There is no module system beyond `import`/`from` for named-map reuse.

### 1.1 Two dialects: full mapping vs. field interpolation

Bloblang appears in two grammatically distinct contexts:

1. **Full mapping**, used in the `bloblang` processor, `mapping` fields, test files, and `.blobl` files. Multiple statements, assignments, `let`, `map`, `import`.
2. **Field interpolation**, used inside string config fields as `${! expression }`. A single query expression only — no statements. Literal `$` is kept verbatim unless followed by `{!`; the escape for a literal `${!` is `${{!...}}`.

Both share the same expression grammar. Everything below is in the full-mapping grammar unless explicitly marked *"interpolation only"*.

---

## 2. Lexical Structure

### 2.1 Encoding and whitespace

- Source is UTF-8 (runes). The parser operates on `[]rune`.
- **Whitespace** is spaces and tabs.
- **Newlines** are `\n` or `\r\n`.
- Newlines **are significant as statement separators** in mappings (see §7).
- Newlines are **conditionally significant inside expressions** — the parser is not uniformly newline-tolerant. The rules:

  **Positions that accept newlines freely** (between tokens, effectively ignored):
  - Inside array literals between elements (`[a,\n b,\n c]`).
  - Inside object literals between members (`{"a": 1,\n "b": 2}`).
  - Inside function/method argument lists between arguments and around commas (`fn(\n a,\n b\n)`).
  - After a binary operator in an arithmetic/comparison/logical chain (`a +\n b`).

  **Positions that reject newlines**:
  - **Immediately before a binary operator** (`a\n + b` is a parse error; the arithmetic chain delimiter uses `SpacesAndTabs` before the operator and `DiscardedWhitespaceNewlineComments` only after).
  - **Around the `->` in a lambda** (`x\n-> body` and `x ->\n body` are parse errors; both sides of `->` use `SpacesAndTabs` only — `parser/query_expression_parser.go:222-228`).
  - **Immediately before a `.` in a path or method chain** (`a\n.b` is a parse error; `parseWithTails` in `parser/query_function_parser.go:95-137` expects the `.` with no leading whitespace). Newlines AFTER the `.` are fine: `a.\n  b` and `a.\n  method()` both work.
  - **Any whitespace before a `.`** (`a .b` also fails — the parser requires the dot to immediately follow the preceding expression). Space/newline after the `.` is fine.

  **Migration implications**:
  - Long method chains must break *after* the `.`, never before. `root = items.\n  filter(...).\n  map_each(...)` is idiomatic V1; `root = items\n  .filter(...)` is a parse error.
  - Lambda bodies must stay on the same line as `->`. Wrap long bodies in parens + named capture (`foo.(name -> body)`) or use a `let` binding to split work.
  - Long expressions break *after* the binary operator. `a +\n b` works; `a\n + b` does not.

### 2.2 Comments

Line comments start with `#` and run to the end of the line.

```blobl
# top-level comment
root.foo = this.bar   # trailing comment
```

Comments are allowed wherever whitespace is allowed, including between the tokens of a single expression and between arguments.

A leading `#!` (shebang-style) on line 1 is simply a line comment — the parser does not treat it specially, but `.blobl` files in the wild use it for tooling.

### 2.3 Identifiers

Two overlapping lexical classes are used for different positions:

```
IDENT        = [A-Za-z0-9_]+           ; lenient (path segments, lambda params, @meta shortforms)
SNAKE_CASE   = [a-z0-9_]+              ; lowercase-only (function/method names, named args, meta bare keys)
```

- **`IDENT`** — any run of ASCII letters, digits, and underscores (`fieldReferencePattern` in `parser/query_function_parser.go:152`; `contextNameParser` at `parser/query_expression_parser.go:207-215`). Permits digits as the leading character — this is how `this.0` works for array indexing (see §6.3), and it also allows uppercase letters in path segments.
- **`SNAKE_CASE`** — lowercase letters, digits, underscores (`parser/combinators.go:417`). Used for function/method names, named-arg names, and bare meta keys. Note: a source comment claims "very strict: no double underscores, prefix or suffix underscores" but the actual parser does not enforce these — `_foo`, `foo__bar`, `foo_` all match. Migration tools should treat the lenient pattern as authoritative.

### 2.4 Reserved keywords

The following identifiers are recognised as keywords in at least one position:

```
true  false  null        # literals
this  root                # context references
if  else                  # conditionals (else if is two tokens)
match                     # pattern expression
let                       # variable binding
map                       # named-map definition
import  from              # module loading
meta                      # metadata keyword + function
_                         # wildcard in match cases and lambda params
```

Keywords are not reserved from being *field names*: `this.match` and `root.if` are valid path accesses because the path segment parser accepts any identifier.

### 2.5 Operator and punctuation tokens

```
+  -  *  /  %                   ; arithmetic
== != <  <= >  >=               ; comparison
&& ||                           ; logical (word-level not; !expr for prefix)
!                               ; prefix logical not (see §5.3)
|                               ; coalesce (high precedence; see §5.2)
=                               ; assignment (statement level only)
->                              ; lambda arrow
=>                              ; match-case arrow
(  )  [  ]  {  }                ; grouping / literals / blocks
.  ,  :                         ; selector / separator / named-arg
$  @                            ; variable / metadata sigils
#                               ; comment
```

There is no `;` statement separator — use a newline.

---

## 3. Types

Bloblang values at runtime are one of:

| Type       | Notes                                                                 |
|------------|-----------------------------------------------------------------------|
| `null`     | The literal `null`.                                                   |
| `bool`     | `true` / `false`.                                                     |
| `number`   | Either integer (`int64`) or floating (`float64`) at runtime. A single runtime type in user-facing sense; internally degrades between `int64` / `uint64` / `float64` (`query/arithmetic.go`). JSON numbers land as `json.Number` and are coerced as needed. **`.number()` as a coercion method always returns `float64`** — even for `"42".number()` — so use `.floor()` / `.round()` if an integer is required. Arithmetic preserves integer-ness (`2 + 3 → 5` as int64), but integer `/` always promotes to `float64` (see §5.3). |
| `string`   | Go `string`, arbitrary UTF-8.                                         |
| `bytes`    | Raw `[]byte`. Returned by `content()` and some decode methods; assignable to `root`. Implicit coercions to/from `string` occur in several methods. |
| `timestamp`| `time.Time`. Produced by `now()`, `ts_parse()`, `ts_*` methods.       |
| `array`    | Ordered heterogeneous list.                                           |
| `object`   | Map from string keys to values (sometimes called "structured" data).  |
| `delete`   | Sentinel from `deleted()` (see §9.4).                                 |
| `nothing`  | Sentinel from `nothing()` / omitted match arms (see §9.4).            |
| `error`    | Not a first-class value, but a separate runtime channel (see §11).    |

The type test method `.type()` returns one of `"null"`, `"bool"`, `"number"`, `"string"`, `"bytes"`, `"timestamp"`, `"array"`, `"object"`. There is no user-level distinction between int and float for `.type()`; both report `"number"`.

---

## 4. Literals

### 4.1 Null, booleans

```blobl
null   true   false
```

Lowercase only.

### 4.2 Numbers

```blobl
0   42   -7
3.14   -0.5
```

- Integer literals (no sign) parse as `int64`. A leading `-` is parsed not as part of the literal but as a unary minus (`parser/query_arithmetic_parser.go:76`), applied to the operand it precedes.
- Float literals require a decimal point (`.`) **with digits on both sides** — `.5` and `5.` are both parse errors (`parser/combinators.go:233`; digits are consumed, then `.` + digits is optional but *both* must be present if either is).
- **No hex (`0x...`), octal, or binary literals.**
- **No scientific/exponent notation** (`1e10`, `1.5E-3`) — the number parser only consumes `[0-9]+` optionally followed by `.[0-9]+`.
- `NaN` and `Infinity` are not literals; they can only arise from arithmetic (and division by zero is an error, not an `Inf` — see §12).

### 4.3 Strings

Two forms:

**Double-quoted** (escaped):
```blobl
"hello"   "line\nbreak"   "quote: \"x\""   "é"
```
Processed via `strconv.Unquote`: supports `\n \r \t \" \\ \/ \xFF ￿ \UFFFFFFFF \NNN`.

**Triple-quoted** (raw, multi-line):
```blobl
"""first line
second line with a literal \n in it"""
```
Everything between the opening `"""` and closing `"""` is taken verbatim — backslash escapes are **not** processed, and newlines are preserved. There is no escape for `"""` within a triple-quoted string; if you need that, use the double-quoted form.

There is no single-quote string form. There is no string interpolation — concatenate with `+` or use `format("...%v...", args...)` / `${!...}` (interpolation context only).

### 4.4 Arrays

```blobl
[]
[1, 2, 3]
["a", null, true, {"k": "v"}]
[ this.a, this.b + 1 ]         # elements are full expressions
```

Any expression is allowed as an element. Whitespace and newlines are permitted between elements. A trailing comma after the last element is tolerated by the parser.

### 4.5 Objects

```blobl
{}
{"k": "v"}
{
  "a": 1,
  "b": this.x,
  foo: this.y,                     # bare ident key — treated as `this.foo` at runtime
  ("dyn_" + this.suffix): this.y   # computed key (parens recommended for clarity)
}
```

Keys are parsed by `OneOf(QuotedString, queryParser)` in `parser/query_literal_parser.go:42-45`. At build time, `NewMapLiteral` (`query/literals.go:20-38`) classifies each key:

- **Quoted string literal** (`"foo"`) → static key, used verbatim.
- **Non-string literal** (`5`, `true`, `null`, array/object literals) → **parse error**: `object keys must be strings, received: int64` (or equivalent).
- **Any other expression** (bare ident, path access, function call, parenthesised expression, etc.) → **dynamic key**; evaluated at runtime, result must be a string (runtime error otherwise).

Consequences for migration tools:

- `{a: 1}` **parses** — `a` is the legacy bare-ident form for `this.a`, and if `this.a` is a string at runtime the key works. This is not idiomatic and should be rewritten to `{"a": 1}` (if the intent was a literal key `a`) or `{(this.a): 1}` (if truly dynamic).
- Computed keys **do not strictly require outer parens** in the object literal — `{foo.bar(): 1}` parses fine. Parens are only needed when the expression starts with a quoted string literal that would otherwise be consumed as the key alone — e.g. `{("foo".uppercase()): 1}` needs parens; without them, the parser commits to `"foo"` as the key and then fails on the `.uppercase()` tail.
- Values are any expression.
- Duplicate keys: last wins.
- Empty-string key (`""`) is permitted.

### 4.6 Literal composition in expressions

Array and object literals can appear anywhere an expression is valid — as method arguments, right-hand sides, match arms, lambda bodies, etc.

---

## 5. Operators

### 5.1 Unary operators

- **Prefix `!`** — logical not. Applies to an entire **method-chained term** and may appear anywhere that term can (left of `fn`, left of a lambda body, inside a `match` case expression, etc.). Parsed in `parseWithTails` at `parser/query_function_parser.go:98`. Operand must be `bool`; non-bool is a type error (`query/methods.go:388-398`, the `notMethod`).
- **Prefix `-`** — unary minus. Parsed as an optional prefix of **any operand** in an arithmetic chain (`parser/query_arithmetic_parser.go:74-77`, `Optional(charMinus)` is applied per term inside `Delimited`). `1 + -2`, `true && -this.x > 0`, and `-fn()` are all legal. Implemented as `0 - operand`.

There is no prefix `+` and no postfix operators.

### 5.2 Binary operators and precedence

V1 parses arithmetic-level expressions **flat** — a sequence of operands separated by any of the binary operators — and then resolves precedence in a four-pass reduction at build time (`query.NewArithmeticExpression` in `query/arithmetic.go:457`). The effective precedence table is:

| Level | Operators             | Associativity | Notes                                          |
|-------|-----------------------|---------------|------------------------------------------------|
| 1 (tightest) | `*`  `/`  `%`  `\|`  | left          | Coalesce `\|` binds as tightly as multiplicative ops — surprising |
| 2     | `+`  `-`              | left          | `+` also works for strings (concatenation)     |
| 3     | `<`  `<=`  `>`  `>=`  `==`  `!=` | left          | Chains (`a == b == c`) parse but rarely make sense — see note below |
| 4 (loosest) | `&&`  `\|\|`    | left (flat)   | `&&` and `\|\|` share a level — see warning below |

Warning on level 4: unlike almost every other language, `&&` does **not** bind tighter than `||`. They are resolved in a single left-to-right pass (`query/arithmetic.go:524-540`). `a || b && c` parses as `(a || b) && c`, not the conventional `a || (b && c)`. Migration tools must preserve original parenthesisation to avoid semantic drift.

Warning on level 1: the coalesce operator `|` is **high precedence**. `a + b | c` parses as `a + (b | c)`. This is the opposite of, for example, Kotlin's `?:` (low precedence) or Rust's `||` fallback pattern. Parenthesise when in doubt.

Comparison operators at level 3 are technically left-associative (`a == b == c` parses as `(a == b) == c`), but such chains rarely make semantic sense and migration tools should treat them as likely bugs.

### 5.3 Semantics per operator

| Operator | Accepts                                    | Result         |
|----------|--------------------------------------------|----------------|
| `+`      | number+number, string+string, string/bytes pair | number, or **string** (see note) |
| `-`      | number only                                | number         |
| `*`      | number only                                | number         |
| `/`      | number only                                | **always `float64`** |
| `%`      | any numeric pair — **floats are truncated to `int64`** silently via `IGetInt` (`value/type_helpers.go:175-178`). `7.5 % 2.5` evaluates as `7 % 2 == 1`, not a type error. | integer; divide-by-zero errors like `/` |
| `==`     | any two values                             | bool; **asymmetric in coercion** (see note); `5 == 5.0` is true; structural for object/array; `null == null` is true |
| `!=`     | any two values                             | bool; inverse of `==` |
| `<`, `<=`, `>`, `>=` | number–number or string–string pair | bool; non-numeric/non-string operands are type errors. **Booleans are not orderable.** Timestamps compare as RFC3339Nano strings (works for well-formed timestamps). Also asymmetric — see note. |
| `&&`, `\|\|`         | bool — or any **numeric** that coerces via `IGetBool` (non-zero → true). Strings / null / arrays / objects are type errors. | bool; **Short-circuit is guaranteed** (`boolAnd` / `boolOr` in `query/arithmetic.go:396-442` return before evaluating RHS). So `true && 1` → `true`, `0 \|\| true` → `true`, `true && "x"` → error. |
| `\|` (coalesce)      | any pair                        | left if not `null` and not error; otherwise right. The `deleted()` and `nothing()` sentinels register as null for this test, so `deleted() \| "x" == "x"`. |
| `!`      | bool                                       | bool           |

Note on `+` with bytes: when either operand is `[]byte`, both are coerced to `string` via `IGetString` and concatenated (`query/arithmetic.go:231-240`). The result is `string`, not `bytes`.

**Note on comparison asymmetry** (`query/arithmetic.go:346-392`, `compareOp`): all comparison operators (`==`, `!=`, `<`, `<=`, `>`, `>=`) dispatch on the **left operand's restricted type** and then attempt to coerce the right operand to that type before comparing. Only if that coercion fails do they fall back to a generic structural compare (which usually returns `false` for type mismatches). The key consequences:

- **`bool == number` is not symmetric**. Left side `bool`: the number is coerced to `bool` via `IGetBool` (non-zero → true), so `true == 1`, `false == 0`, `true == 3.14` all return **`true`**. Left side `number`: `IGetNumber` rejects bools, so `1 == true` returns **`false`**. Migration tools must preserve operand order.
- **`bool == string`**: no coercion either way, falls to generic compare → **`false`** (e.g. `true == "true"` is `false`).
- **`string == number`**: no coercion either way, returns **`false`** (e.g. `"5" == 5` is `false`).
- **`null == null`** is `true`; **`null == <anything-else>`** is `false` (including `null == false` and `null == 0`).

The asymmetry matters most for `==`/`!=` with bool-number pairs. For `<`, `<=`, `>`, `>=` the asymmetry is less visible because those operators don't admit bool coercion — a bool operand on either side errors.

Integer overflow in `+`, `-`, `*` is **not checked** — results wrap per Go int64 semantics. Division and modulo by zero return an `ErrDivideByZero` (`query/arithmetic.go:188`, `204`), not `Inf`/`NaN`.

**Compile-time vs. runtime errors**. When both operands of a binary operator are **literals** (including literal expressions), V1 evaluates the operation at parse time via constant folding. Any error produced — type mismatch, divide-by-zero, etc. — is raised as a **compile-time (fatal) parse error**, not a runtime error. Examples: `root = true + false`, `root = 5 + "foo"`, `root = 5 / 0`, `root = null < 3` all fail at parse time. The same operations with non-literal operands (e.g. `this.x + "foo"`) fail at runtime as a recoverable mapping error.

### 5.4 Path-scoped sub-expression: `foo.(expr)` and `foo.(name -> expr)`

Inside a path, `.(...)` introduces a **sub-expression with rebound context** (`parser/query_function_parser.go:53`, `query.NewMapMethod`). Two forms:

**Plain form** — `this` is rebound to the preceding value:

```blobl
root.type = this.thing.(article | comment | this).type
```

Within the parentheses, `this` refers to `this.thing`. Inside that scope, bare identifiers (`article`, `comment`) are the legacy shorthand for `this.article`, `this.comment` (§6.1). This is the canonical place to use `|` coalesce.

**Named-capture form** — a named context is introduced, but `this` is **unchanged**:

```blobl
root.sum = this.foo.bar.(thing -> thing.baz + thing.buz)
```

Here `thing` binds to `this.foo.bar`, and inside the body `this` is still the outer top-level value. Useful when you need both the capture and the outer `this` in the same expression. The lambda-like `name -> body` is a generic expression; it may appear anywhere `.(...)` accepts an expression, including nested chains. Names must not collide with `this`/`root` or an enclosing named context.

Outside of a path, `this.x | this.y` works identically to the plain form due to `|`'s high precedence.

### 5.5 Assignment operator `=`

`=` is a **statement-level** token only (§7). It never appears inside expressions. There are no compound assignments (`+=`, `||=`, etc.) and no destructuring.

---

## 6. Paths and References

### 6.1 Root context references

| Token | Meaning |
|-------|---------|
| `this` | Current query context. At the top level of a mapping this is the input document. Inside a lambda, match arm, `.(...)` scope, or `.apply(...)` body, `this` rebinds to the local value (see §6.5). |
| `root` | The output payload being constructed. Read-only in expressions (reads the partial result so far); write via assignment statements. |
| `$name` | Reference to a variable bound by `let name = ...`. |
| `@` (bare) | The whole metadata object as a value. |
| `@name` | Shorthand for `meta("name")`. Also written `@"quoted name"`. Parsed in `metadataReferenceParser`, `parser/query_function_parser.go:230`. |
| `meta("key")` | Function-call form for metadata; takes an expression key. |
| *any other ident at root* | **Legacy**: treated as `this.<ident>`. E.g. `foo` alone parses as `this.foo`. `parser/query_function_parser.go:271` — "TODO V5: Remove this and force this, root, or named contexts". Migration tools should normalise these to explicit `this.` references. |
| *named context identifier* | Introduced by any lambda (`x -> body`) or by `Environment.WithNamedContext`. E.g. inside `things.map_each(x -> x.name)`, `x` is a first-class root reference. Unlike plain `this`, named-context references survive further lambda nesting without being popped. |

### 6.2 Field path syntax

Paths are chains of `.`-separated segments after a root reference:

```blobl
this.foo.bar.baz
root."foo bar"."baz.qux"   # quoted segments for special chars
this.0                      # numeric segment — array index
this.foo."weird.key".0.name # mix freely
```

Segment grammar (`fieldReferenceParser` + `quotedPathSegmentParser`):

- **Unquoted segment** — one or more characters from `[A-Za-z0-9_]`. Leading digits are allowed. This enables `this.0` style indexing.
- **Quoted segment** — a standard double-quoted string literal. Internally `.` is encoded as `~1` and `~` as `~0` to survive the dot-joined JSON-pointer-ish representation; user code never sees this.

There is **no bracket-indexing syntax**: `this[0]` is a **parse error**. For dynamic indexing use `.index(i)` (`this.items.index(i)`) or build a path via `.get(path_expr)` style methods.

### 6.3 Path access on arrays and numeric-segment writes

**Reading**: an unquoted numeric segment on an array value is treated as an index:

```blobl
this.items.0        # first element
this.items.5        # sixth element; null if out of range
```

**Negative indices via path are not supported** — the identifier character class is `[A-Za-z0-9_]`, so `this.items.-1` is a parse error. Use `.index(-1)` for negative indices. `.index()` errors out-of-range rather than returning null — use `.or(null)` / `.catch(null)` for null-on-OOB.

**Writing a numeric-segment target creates an object**, not an array. `root.items.0 = "x"` produces `{"items": {"0": "x"}}` — the `"0"` key is a string in a JSON object, not an array index. There is **no automatic array creation** from numeric paths and **no array gap-filling**. To build arrays, seed explicitly (`root.items = []`) then append via `root.items = root.items.append(x)` (or similar methods), or construct the whole array in one expression and assign it.

### 6.4 Writable paths (assignment targets)

Targets in `target = expr` statements are a **restricted** subset of path expressions. The full grammar is in `parser/mapping_parser.go` (`assignmentTargetParser`); the accepted forms are:

```blobl
root                        # replace payload wholesale
root.<segment>(.<segment>)* # write a nested field (quoted segments allowed)
this                        # legacy: equivalent to root
this.<segments>             # legacy: equivalent to root.<segments>
meta                        # replace metadata wholesale
meta <key>                  # single metadata entry; bare identifier or quoted string ONLY
<segment>(.<segment>)*      # legacy bare-path form: equivalent to root.<segments>
```

**`meta(<expr>) = v` is NOT a valid assignment target** even though `meta("key")` reads work in expressions. The parser (`parser/mapping_parser.go` `metaStatementParser`) accepts only a bare identifier or a quoted string after `meta` at the target position. To write a dynamic metadata key, assemble an object and do a wholesale replacement: `meta = @.merge({(dynamic_key): value})` or `meta = {(key_expr): value}` (the latter wipes other entries).

Variable bindings (`let name = ...`) are a **separate statement form** (§7.2), not an assignment with a target — they are not interchangeable with `=`-targets.

Key constraints:

- **No dynamic fields in assignment paths**: `root.(expr)` and `root[expr]` are not assignment targets. To write a dynamic key, use `root = root.merge({(key_expr): value_expr})` or build an object literal.
- **No variable reassignment via `=`**: `$x = ...` is invalid. Use `let x = ...` to re-bind (which overwrites — see §7.2).
- `meta "foo" = deleted()` is the canonical way to remove a single metadata entry.
- `meta = deleted()` wipes all metadata.
- `root = deleted()` marks the whole message for deletion (the processor drops it).
- **Whole-meta assignment requires an object**: `meta = "string"`, `meta = 5`, `meta = [1,2]` all raise a runtime error `"setting root meta object requires object value, received: <type>"`. The only permitted right-hand sides are `deleted()` (clears meta) and an object value.

### 6.5 `this` rebinding

`this` is reassigned by many constructs. Lambdas interact with `this` in a surprising way (see the warning below).

- **Method with a non-lambda argument** — `items.map_each(this.value)`: the argument is evaluated with `this` **rebound to the current element**. This is the common, "obvious" semantics.
- **Method with a lambda argument** — `items.map_each(x -> body)`: the lambda is parsed as a `NamedContextFunction`. Its `Exec` (`query/expression.go:166-175`) pops the current value off the context stack and binds it to `x`, so `body` executes with **`this` reverted to the outer (parent) context** and `x` holding the element. Idiomatic code inside a lambda always references the named parameter, never `this`.
- **`_` lambda parameter** — `items.filter(_ -> expr)`: pops the value but binds no name. Inside `expr`, `this` has reverted to the outer context and there is no name for the element. This form is rarely useful; prefer naming the parameter.
- **`.apply("map_name")`** — inside the named map's body, `this` is the receiver value. Variables are cleared on entry (see §10.2).
- **`.(expr)` plain form** — inside `this.foo.(bar | baz)`, `this` is `this.foo` (no context pop; inner expression sees the new context directly).
- **`.(name -> expr)` named-capture form** — the inner lambda pops the just-rebound value, so `this` effectively reverts to the outer context and `name` is bound to `this.foo`. See §5.4.
- **`match subject { ... }`** — inside each case (the pattern *and* the result), `this` is `subject`. For subject-less `match { ... }`, `this` is unchanged.
- **`if cond { ... } else { ... }` blocks** (statement form) — `this` is unchanged; blocks execute with the outer context.
- **Method call arguments that are not lambdas** — e.g. `this.foo.bar(some_fn())`: inside `some_fn()`, `this` is the *outer* `this`, not the receiver. Receiver rebinding applies only to the body of the method itself, not to arguments.

**Warning for migration tools**: the lambda/non-lambda split means `items.map_each(this.value)` and `items.map_each(x -> this.value)` have **different** meanings. The first maps each element to the element's `value`; the second maps each element to `this.value` from the *outer* scope. When rewriting, never mechanically wrap a non-lambda method argument in `x -> ...` — the semantics change.

---

## 7. Statements

A mapping is a sequence of statements separated by newlines. Each statement is one of:

```
Statement
  = Assignment
  | LetBinding
  | MapDefinition
  | Import
  | FromImport
  | RootLevelIf
  | BareExpression
```

### 7.1 Assignment

```blobl
<target> = <expression>
```

Targets are the writable paths of §6.4. The expression is any query (§8). Multiple assignments to `root.a.b` across separate statements are **incremental**: each is a write to the (evolving) output document.

### 7.2 `let` bindings

```blobl
let <name> = <expression>
let "quoted name with spaces" = <expression>
```

Quoted names are permitted for the binding target (matching the metadata pattern). Access is always `$name` (dollar + identifier). Variables:

- Are stored in a **flat per-execution map** — `ctx.Vars` of type `map[string]any` (`query/package.go:50`). There is **no block scope**: `let` statements inside `if` or other blocks mutate the same map, and the re-binding remains visible after the block exits.
- Are **overwritten on re-binding**: `let x = 1` followed by `let x = 2` leaves `$x == 2`. There is no shadowing stack.
- Are **deleted by `let x = deleted()`** — `VarAssignment.Apply` in `mapping/assignment.go:57-61` explicitly checks for the delete sentinel and calls `delete(ctx.Vars, name)`. This is the way to remove a variable binding; a subsequent read of `$x` then errors as unset.
- Are **cleared at `apply` boundaries** — inside a `.apply("foo")` body, the variable environment is reset to an empty map (`query/methods.go:64`). Bindings set inside the apply do not leak out; bindings set outside are not visible inside.
- Are evaluated **eagerly at the `let` statement's position** — the right-hand side runs once at that point, not on each read.

### 7.3 Root-level `if` / `else if` / `else`

At the statement level an `if` block groups multiple statements:

```blobl
if this.type == "cat" {
  root.sound = "meow"
  meta category = "pet"
} else if this.type == "dog" {
  root.sound = "woof"
} else {
  root.sound = "?"
}
```

Semantics:

- Exactly one branch executes.
- Each block body is itself a sequence of mapping statements (recursive).
- Conditions must be boolean (non-boolean is a recoverable error).
- `else` is optional; `else if` chains freely.

Distinct from the expression form (§8.3) — statement `if` has `{...}` blocks of statements; expression `if` has `{...}` blocks of a single expression.

### 7.4 Bare expression statements

A standalone expression at the top level is treated as `root = <expr>`:

```blobl
this.foo.uppercase()    # equivalent to: root = this.foo.uppercase()
```

This is a **legacy convenience** and has one wart: it may only appear as the *sole* statement in the mapping. If any other statement follows, the bare form is rejected — the parser requires an explicit `root = ...`. Migration tools should normalise to explicit `root = <expr>`.

### 7.5 `map` definition

See §10.

### 7.6 `import` / `from`

See §10.4 and §10.5.

### 7.7 Statement separators

- Newlines separate statements.
- Multiple statements on one line are **not allowed** — there is no `;`.
- Blank lines are allowed anywhere.
- Trailing comments are allowed on any line.

---

## 8. Expressions

### 8.1 Primary expressions

```
Primary
  = Literal                          # §4
  | "this" | "root" | Ident          # root references (Ident legacy-captures this.ident)
  | "$" Ident                        # variable
  | "@" (Ident | QuotedString)?      # metadata (bare @ = whole meta object)
  | "(" Expression ")"               # grouping
  | ArrayLiteral | ObjectLiteral
  | FunctionCall                     # ident "(" args ")"
  | IfExpression                     # §8.3
  | MatchExpression                  # §8.4
  | LambdaExpression                 # §8.5
```

### 8.2 Tails (method chains, field access, map expression)

Any primary expression can be followed by a chain of:

```
Tail
  = "." Ident                        # field access
  | "." QuotedString                 # quoted field access
  | "." Ident "(" args ")"           # method call
  | "." "(" Expression ")"           # map expression (rebinds this; often used with |)
```

Combined, these compose the full expression grammar. `parseWithTails` (`parser/query_function_parser.go:95`) loops building a left-to-right chain.

### 8.3 `if` expression form

```blobl
root.label = if this.score > 50 { "high" } else { "low" }
```

- Each branch body is a **single expression** (not a statement list).
- `else if` chains work the same as in statement form.
- If there is no `else` and no branch matches, the expression produces the **`nothing` sentinel** (`value.Nothing`), not `null`. When this sentinel is assigned to a target, **the assignment is silently skipped** (prior value preserved, target absent if never set). See §9.4.
  - Example: `root.a = 1; root.a = if false { "x" }` leaves `root.a == 1` (the second assignment is elided).
  - This also means `.catch()` cannot rescue a non-matching `if` — there is no error to catch; the whole assignment just vanishes.
- A **null** condition is treated as falsy (branch doesn't fire) — V1 does not error on a null condition, only on non-bool non-null values.

### 8.4 `match` expression

**With subject**:
```blobl
root.kind = match this.thing {
  this.type == "article" => this.article
  this.type == "comment" => this.comment
  _ => this
}
```

**Without subject** — `this` context is unchanged:
```blobl
root.kind = match {
  this.doc.type == "article" => this.doc
  this.doc.type == "comment" => this.doc
}
```

**Literal equality pattern**:
```blobl
root.kind = match this.type {
  "doc" => "document"
  "art" => "article"
  _ => this
}
```

Matching rules (`parser/query_expression_parser.go:44-58` and the match executor):

- Each arm is `<pattern> => <result>`. Arms are separated by newlines **or** commas (both accepted, and they may be mixed). A trailing comma after the last arm is tolerated.
- The pattern is classified **at parse time**, not runtime, based on its AST shape:
  - `_` — wildcard, always matches.
  - A **literal** value (string, number, bool, null, literal object, literal array) — converted to an equality check against the subject via `value.ICompare` (representation-agnostic, so `5` matches `5.0`).
  - **Any other expression** — evaluated per-arm; the **result must be `bool`**, and a `true` result indicates a match. If a non-literal expression evaluates to a non-bool (e.g. a string or number), the match simply does not fire — no case-specific error is raised, and the next arm is tried.
- Evaluation is top-to-bottom; first match wins. **All arm patterns are evaluated eagerly** until one matches — an earlier `throw()` in a later arm still fires if a prior arm didn't match. V1 does not short-circuit arm evaluation at the pattern level.
- If no case matches, the expression produces the **`nothing` sentinel**, not `null`. When assigned, the assignment is silently skipped (same semantics as a non-matching `if` — see §8.3).
- Inside each arm (both pattern and result), `this` is the subject (when present) or the outer `this` (subject-less form). This is a common footgun: `match this.user { _ => this.user.name }` evaluates `this.user.name` as `<subject>.name`, which is `this.user.name` — same as intended here, but `match this.user { $target => ... }` evaluates `$target` in the subject context.
- A **null** subject (or null condition via boolean pattern) is tolerated, not an error. V1 does not raise on `match null { ... }`.

Note: the literal/expression classification is syntactic, so `match x { (5) => ... }` behaves the same as `match x { 5 => ... }` (the parenthesised literal still reaches the parser as a literal). But `match x { some_fn() => ... }` is treated as an expression pattern even if `some_fn()` always returns `5`.

### 8.5 Lambdas

```blobl
items.map_each(x -> x.value + 1)
items.filter(x -> x.active)
```

Lambda grammar:

```
Lambda = (Ident | "_") "->" Expression
```

- A lambda is a **first-class query expression** — the parser lists `lambdaExpressionParser` among the top-level alternatives in `queryParser` (`parser/query_parser.go:14`). In practice they are almost always used as method arguments, but they can appear anywhere a query can, and the `.(name -> body)` form (§5.4) exploits this.
- **Context handling** — a lambda compiles to a `NamedContextFunction`. At runtime, executing it *pops* the current value off the context stack and (if the name is not `_`) binds that value to the named parameter. The body therefore executes with `this` reverted to the **parent** context, not with `this` = the element. See §6.5 for the full rule and the migration warning.
- Named parameters cannot be `root` or `this` — the parser rejects those names (`parser/query_expression_parser.go:246-251`).
- Named parameters cannot **shadow** a parent lambda's parameter in the same chain — the parser tracks named contexts via `pCtx.HasNamedContext` and rejects collisions with a "would shadow a parent context" error.
- The `_` parameter is special: the context pop still happens, but no name is bound, so inside the body there is no way to reference the popped element. This is only useful when the body doesn't need the element (e.g. `items.map_each(_ -> uuid_v4())` generates a list of UUIDs with one per element, ignoring the element itself).
- Some methods pre-declare named parameters via their params spec rather than using user-named lambdas. `.sort(left > right)` and `.sort(left, right -> left > right)` forms pass a comparison query that references the implicit `left` and `right` identifiers. The exact syntax per method is declared in `query/docs.go` / the method's `Params`; migration tools should treat the argument as method-specific and not assume general lambda shape.

### 8.6 Named arguments

Most functions and methods accept **positional** arguments. Some also accept **named** arguments, distinguished syntactically by `name: value`:

```blobl
range(start: 0, stop: 10, step: 2)
range(0, 10, 2)                       # equivalent positional form
```

- Positional and named arguments **cannot be mixed** in a single call (`parser/query_function_parser.go:299`). It's all-positional or all-named.
- Argument names use the stricter `snake_case` identifier form.
- Named-arg availability per function/method is declared in its `params` spec (`query/params.go`).

---

## 9. Built-in Functions and Methods

The full, authoritative list is too long to inline and is subject to version drift. Migration tools should treat these catalogues as the source of truth:

- **Functions**: registered via `registerFunction` in `query/functions*.go`, aggregated in `query.AllFunctions`. Documented at `docs.redpanda.com/redpanda-connect/guides/bloblang/functions/`.
- **Methods**: registered via `registerMethod` in `query/methods*.go` (split by category — general, strings, numbers, structured, time, regexp, encoding, coercion, parsing, jwt, geoip). Aggregated in `query.AllMethods`. Documented at `docs.redpanda.com/redpanda-connect/guides/bloblang/methods/`.

### 9.0 Core vs. impure-package methods

V1 has **two tiers** of built-in methods, which confuses migration:

- **Core methods** are registered directly in `internal/bloblang/query/*.go` (`methods.go`, `methods_structured.go`, `methods_general.go`, etc.) and are available in every environment returned by `bloblang.NewEnvironment()` / `bloblang.GlobalEnvironment()`.
- **Extension methods** are registered in `internal/impl/pure/*.go` and only become available when that package is imported (e.g. via `_ "github.com/redpanda-data/benthos/v4/internal/impl/pure"` or through the full `cmd/benthos` binary). Notable extension-only entries include `abs`, `int64`, `int32`, `float64`, `float32`, `uint32`, `uint64`, `pow`, `ceil`, `sqrt` (most numeric-width coercers), `ts_parse`, `ts_format`, `ts_strptime`, `ts_strftime`, `ts_add_iso8601`, `ts_sub`, most `ts_*` formatters, `.round(N)` with a precision argument, `.char`, `.map_values`, `.map_entries`, `.filter_entries`, `.collect`, `.concat`, `.chunk`, `.reverse` on arrays (string-only reverse is core), and a handful of encoding / hashing additions.

A migration tool that parses a V1 mapping against the bare `public/bloblang` environment will report many of these as **unknown methods**. The migration tool should preserve those references verbatim and assume the host binary registers `internal/impl/pure` (as `cmd/benthos` and Redpanda Connect do).

Each function/method spec (`query/docs.go`) carries:

- A **status**: `StatusStable`, `StatusBeta`, `StatusExperimental`, or `StatusDeprecated`. Deprecated entries are the primary signal for migration.
- A **category**: used for documentation grouping (see below).
- **Impure** flag: whether the function has side effects or non-deterministic output (affects optimiser and pure-environment restrictions).

### 9.1 Function categories (`FunctionCategory*`)

```
General, Message, Environment, FakeData, Deprecated, Plugin
```

### 9.2 Method categories (`MethodCategory*`)

```
General, Strings, Numbers, Time, Regexp, Encoding, Coercion, Parsing,
ObjectAndArray, JWT, GeoIP, Deprecated
```

### 9.3 Migration-critical idioms

Rather than enumerate everything, the following idiomatic clusters appear almost universally in real mappings and should be recognisable to any migration tool:

- **Presence / defaults**: `.or(default)`, `.exists()`, `.not_null()`, `.catch(default)`.
- **Type checks**: `.type() == "array"`, `.type() == "object"`, etc. Also `.array()`, `.object()`, `.string()`, `.number()`, `.bool()` coercers.
- **Collections**: `.map_each(x -> ...)`, `.filter(x -> ...)`, `.fold(init, tally -> value -> ...)`, `.sort(a, b -> ...)`, `.sort_by(field_expr)`, `.unique()`, `.flatten()`, `.length()`, `.sum()`, `.min()`, `.max()`, `.keys()`, `.values()`, `.key_values()`, `.enumerated()`, `.index(i)`, `.slice(start, end)`.
- **Object manipulation**: `.merge(other)`, `.assign(other)`, `.without("a", "b.c", ...)`, `.get(path_expr)`, `.not_empty()`.
- **Strings**: `.uppercase()`, `.lowercase()`, `.capitalize()`, `.trim()`, `.split(sep)`, `.join(sep)`, `.replace_all(old, new)`, `.re_replace_all(pattern, repl)`, `.contains(s)`, `.has_prefix(s)`, `.has_suffix(s)`, `.quote()`, `.format(...)`, `.escape_html()` / `.unescape_html()`, `.escape_url_query()` / `.unescape_url_query()`.
- **Encoding/parsing**: `.parse_json()`, `.parse_yaml()`, `.parse_csv()`, `.format_json()`, `.format_yaml()`, `.encode("base64"|"hex"|...)`, `.decode(...)`, `.hash("sha256", key?)`.
- **Time**: `now()`, `ts_parse(format)`, `ts_format(format)`, `ts_unix()`, `ts_sub(t)`, `ts_add_duration(d)`.
- **Message / batch**: `content()`, `batch_index()`, `batch_size()`, `.from(idx)`, `.from_all()`. `.from(idx)` with negative or out-of-range `idx` is **not clearly defined** — behaviour depends on the `MsgBatch` implementation; tools should treat such calls as suspect.
- **State / stateful**: `counter()` (monotonic per-mapping), `count("name")` (named counter shared across messages — **impure**, tracks state externally). A mapping that relies on `count()` for ordering (e.g. "emit CSV header only on first row") is not re-runnable in isolation.
- **Env / identity / random**: `env("FOO")`, `hostname()`, `uuid_v4()`, `uuid_v7()`, `nanoid()`, `random_int()`.
- **Errors**: `error()` (last error in chain), `throw("msg")`.

### 9.4 Sentinel-returning functions

Two functions return special sentinel values, not regular data. Both are recognised as "null-like" by `value.IIsNull` (`query/type_helpers.go:307`), which drives the behaviour of `.or(...)` and `|` below.

- **`deleted()`** — the *delete* sentinel (`value.Delete`).
  - Assigned to `root` → drops the whole message.
  - Assigned to `root.path.to.x` → removes that field (or array element at that index).
  - Assigned to `meta` → clears all metadata.
  - Assigned to `meta key` → removes that metadata key.
  - Returned from a `.map_each` lambda → that **element is dropped from the resulting array/object** (`query/iterator.go:242-247`). The result does not contain a `null` hole.
  - Returned from a match arm → produces the `deleted()` value; the surrounding assignment then applies it according to the target rules above.
  - As operand to arithmetic or comparison operators → type error (the value is not a number, string, etc.).
  - As operand of `\|` or `.or(fallback)` → **replaced with fallback** (treated as null).
  - As operand of `.catch(fallback)` → **preserved** (not an error).
  - In a path tail (`deleted().foo`) → behaviour not covered by test corpus; migration tools should treat such chains as suspect rather than relying on any particular outcome.
- **`nothing()`** — the *no-op* sentinel (`value.Nothing`). **Sources** of this sentinel include:
  - The explicit `nothing()` function call.
  - `if <cond> { body }` with no `else`, when the condition is falsy (§8.3).
  - `match <subject> { ... }` with no matching arm and no wildcard (§8.4).
  - `if cond { body1 } else if cond2 { body2 }` where no arm's condition is truthy.

  **Behaviour of the sentinel**:
  - Assigned to anything → the assignment is **silently skipped** (`mapping/statement.go:64-67`); the target is left unchanged (prior value preserved, or the key is absent if never set). The field does **not** appear as `null` in the output.
  - Returned from a `.map_each` lambda → the **original element is preserved unchanged** (`query/iterator.go:243-244`).
  - As operand to arithmetic or comparison operators → type error (same as `deleted()`).
  - As operand of `\|` or `.or(fallback)` → replaced with fallback (treated as null).
  - As operand of `.catch(fallback)` → preserved (not an error — there is nothing to catch).
  - Assigned as the value of a `let` binding → the binding is **deleted**, not set to null. A subsequent `$name` read raises `"variable 'name' undefined"`. So `let x = nothing()` has the same effect as never declaring `x`.

The distinction matters for migration: a match arm returning `deleted()` vs. `nothing()` at the **same position** produces different outputs (the field is removed vs. left at its prior value). Do not collapse them.

### 9.5 Plugin-registered functions and methods

See §13.

---

## 10. Maps and Modules

### 10.1 Named maps

```blobl
map things {
  root.first  = this.thing_one
  root.second = this.thing_two
}

root.foo = this.value_one.apply("things")
root.bar = this.value_two.apply("things")
```

A **named map** is a reusable mapping body, defined at the **top level** of a source file. It cannot:

- Be defined inside another map, function, or block.
- Contain `meta` assignments (enforced by the parser — maps produce values, not metadata).
- Contain `import` or nested `map` statements (no nesting).
- Recurse without bound — `Environment.WithMaxMapRecursion(n)` configures a per-environment limit. The default behaviour (when the option is not set) depends on the host and is not documented at the API level; tools that evaluate mappings should set an explicit limit.

Within a map body:

- `this` is the **receiver** — the value passed to `.apply("name")`.
- `root` is a **fresh** value scoped to the map; the map's result is that fresh `root`.
- `$var`s are **reset** on entry (variables do not leak in or out).

### 10.2 `.apply(name)`

`.apply("name")` is the canonical invocation. The argument is an expression; a literal string is usual, but computed names (`.apply(this.kind)`) work and allow dynamic dispatch.

### 10.3 Built-in catalogue integration

Unlike functions and methods, maps are user-defined only. There is no built-in map library.

### 10.4 `import` statement

```blobl
import "./shared/common_maps.blobl"

root.foo = this.bar.apply("some_map_from_that_file")
```

- The path is resolved **relative to the importing file** when importing a file from disk, or **relative to the process working directory** for the outermost file.
- Imported files typically contain `map` definitions (and further `import`s); whether the parser allows top-level statements inside an imported file is not directly exercised by the test corpus — treat as undefined and restrict imported files to map definitions for portability.
- A map-name collision across imports is a parse error.
- Imports are static: the path must be a string literal.

### 10.5 `from "path"` — direct include

```blobl
from "./shared/base_mapping.blobl"
```

- The referenced file is a full mapping (top-level statements allowed), and it **replaces** the current mapping body entirely.
- `from` must be the **only** statement in the file using it; it cannot be mixed with other statements.
- Rarely used in modern corpora; most migration targets are `import` + `.apply(...)`.

### 10.6 Environment-level imports

`Environment.WithDisabledImports()` rejects all `import`/`from` at parse time. `Environment.WithCustomImporter(fn)` overrides the filesystem resolution — useful for embedded tools. A migration tool that receives a mapping out of context may not know what `import` paths resolve to.

---

## 11. Field Interpolation (`${! ... }`)

Field interpolation is the **expression-only** dialect used inside string configuration values in Redpanda Connect YAML configs:

```yaml
output:
  kafka:
    topic: "ingest.${! this.region }.${! meta(\"tenant\").or(\"default\") }"
    key: "${! this.id }"
```

Rules (`internal/bloblang/field/`):

- The substring between `${!` and the matching `}` is parsed as a **single query expression**. Statements, `let`, `map`, `import`, multi-statement `if` blocks, and assignments are not allowed.
- A single config string may contain **multiple interpolations**; each one is parsed independently. The surrounding literal text is emitted verbatim.
- To emit a literal `${!...}` without interpolation, use **double braces**: `${{! expression }}` is emitted as the literal `${! expression }`.
- Any other `$...` sequence (not `${!`) is left verbatim, including `$foo`, `${foo}` (environment-style braces), etc.
- Interpolation results are **coerced to string** for concatenation with the surrounding literal text. Exact coercion rules for each type (particularly null and structured values) depend on the implementation in `internal/bloblang/field/`; treat as defined by the reference impl.
- Errors in interpolation propagate to the enclosing component, which decides whether to fail the message or use a fallback.

---

## 12. Error and Nullability Model

### 12.1 Errors are out-of-band

At runtime, evaluating an expression returns either a value **or** an error. Errors propagate through the expression eagerly: the innermost operation that fails produces the error; outer operations pass it up unless explicitly caught.

### 12.2 Catch vs. null-default

| Construct | Catches errors | Catches nulls | Catches `deleted()` / `nothing()` sentinels | Notes |
|-----------|:--------------:|:-------------:|:-------------:|-------|
| `.catch(fallback)` | yes | no | no (passed through) | Preserves non-error nulls and sentinels. |
| `.or(fallback)`    | yes | yes | **yes** (sentinels register as null) | Fallback on null, error, or sentinel. |
| `\|` operator      | yes | yes | **yes** | Identical to `.or(...)` — `coalesce` at `query/arithmetic.go:444-452`. |

A **recoverable error** is one that does not crash the mapping: type mismatches in operators, missing metadata, out-of-range indexes, divide-by-zero, `throw(...)`, etc. The default behaviour when a mapping reaches completion with an unhandled error is to **reject the message** at the processor level. Configure processor-level error handling outside Bloblang.

### 12.3 `error()` function

Inside a mapping, after an error has been produced further up the chain *and caught*, `error()` returns the stringified error message. More commonly used in downstream processors (catching into a branch) than within a single mapping.

### 12.4 `throw(msg)`

`throw("something went wrong")` produces an error with the given message.

### 12.5 Null-safe path access

**Path access (`.field`, `.0`, `."quoted"`) is universally null-tolerant**: it returns `null` for every case where another language would raise a type error. Not just null — any non-object receiver. `5.foo`, `true.foo`, `"hello".foo`, `[1,2,3].foo`, and `null.foo` all yield `null`. The path machinery uses gabs which treats any non-object traversal as a missing key.

**Method calls (`.method()`) are NOT null-tolerant by default**. They have per-method type requirements and generally error on a wrong-type receiver:
- `null.length()` → error `"expected string, array or object value, got null"`.
- `null.uppercase()` → error `"expected string value, got null"`.
- `5.length()` → error `"expected string, array or object value, got number"`.

A migration tool should therefore:
- Treat any path access as returning null for missing/wrong-type data — never assume an error.
- Treat any method call as a potential type error if the receiver type is not guaranteed.
- Use `.catch(fallback)` to handle method-on-null errors, or `.or(fallback)` to handle both null results and errors uniformly.

**`.index(n)` on an array is a method (not a path)** and follows method semantics: out-of-range indexes are a **runtime error** (`"variable arr: index '5' was out of bounds for array size: 3"`), not `null`. Use `.index(n).catch(null)` or `.index(n).or(null)` to get null-on-OOB.

### 12.6 Sentinel interaction summary

The `deleted()` and `nothing()` sentinels are distinct from errors. Their exact interaction with `.or(...)`, `|`, and `.catch(...)` is covered in §9.4 and §12.2. In short:

- `.catch(x)` treats them as successful values and **passes them through** (they aren't errors).
- `.or(x)` and `|` treat them as null-like and **replace them with the fallback** (via `value.IIsNull`).

This asymmetry is load-bearing: a mapping relying on `.catch(deleted())` to keep a deletion intent will behave differently if naively rewritten to `.or(deleted())`.

---

## 13. Environment, Plugins, and Extensibility

The language is parameterised by an **Environment** (`internal/bloblang/environment.go`) that holds:

- The registered set of functions (`Environment.Functions`).
- The registered set of methods (`Environment.Methods`).
- The import resolver (filesystem by default; overridable).
- Optional restrictions: `WithoutFunctions(names...)`, `WithoutMethods(names...)`, `OnlyPure()`, `WithDisabledImports()`.
- Named contexts injected from the host (rare, but visible in the parser).

From Go, hosts extend the language via the public `public/bloblang/` package. The primary entry points are:

- `env.RegisterFunctionV2(name, spec, ctor)` / `env.RegisterMethodV2(name, spec, ctor)` — register a custom function/method with a `*PluginSpec` describing parameters and docs.
- `env.RegisterFunction(name, ctor)` / `env.RegisterMethod(name, ctor)` — legacy shorthand for simple cases without a rich spec.
- `env.Parse(blobl)` returns an `*Executor` that can be `Execute(...)`d against messages.

From YAML-only contexts, the Redpanda Connect `bloblang` processor allows no direct plugin registration; plugins are added at the binary-build level.

**Migration implication**: a mapping validated by a custom environment may contain function/method names that are *not* in the default environment. A migration tool should:

1. Parse against the default environment first.
2. Treat "unknown function" errors for non-standard identifiers as *plugin names* rather than failures, and emit a migration note rather than erroring out.
3. Preserve unknown identifiers verbatim in the output.

---

## 14. Quirks, Legacy Forms, and Migration Gotchas

This section catalogues behaviours that are accepted by the V1 parser but that a migration tool **must** handle explicitly. Many are flagged in the V1 source with `TODO V5` comments.

1. **Bare identifiers as `this.` paths**. `foo.bar` at the start of an expression is parsed as `this.foo.bar` (`parser/query_function_parser.go:271`). Migration should rewrite to explicit `this.foo.bar`.
2. **Bare paths as assignment targets**. `foo.bar = 1` is parsed as `root.foo.bar = 1` (`parser/mapping_parser.go` target parser). Rewrite to explicit `root.foo.bar = 1`.
3. **Unusual `&&`/`||` precedence**. `a || b && c` parses as `(a || b) && c`. Always preserve original parentheses; when adding parentheses in a rewrite, match V1 semantics.
4. **High-precedence `|`**. `a + b | c` is `a + (b | c)`. Parenthesise on rewrite if unsure.
5. **Integer division produces floats**. `4 / 2` is `2.0`, not `2`. Code relying on integer division must use `.floor()`/`.round()`.
6. **`==` is representation-agnostic for numbers**. `5 == 5.0` is `true`. V2 may differ; check before rewriting comparisons.
7. **Triple-quoted strings are raw**. `\n` inside `"""..."""` is a literal backslash+n. Do not mechanically re-escape.
8. **Object keys must be quoted**. `{a: 1}` is a parse error; only `{"a": 1}` is accepted. Any auto-rewrite emitting objects must quote keys.
9. **Computed keys require parentheses**. `{("k_" + x): v}`, not `{"k_" + x: v}`.
10. **`this[0]` is a parse error** — use `.index(0)` or the `this.0` path form.
11. **`this.-1` is a parse error**. The path segment charset is `[A-Za-z0-9_]`, which excludes `-`. Use `.index(-1)` for last-element access. `this.0`, `this.5`, etc. are fine.
12. **`from "file"` replaces the whole mapping** — treat as a distinct migration target from `import`.
13. **Named-map bodies forbid `meta`**. A migration that tries to promote a bulk mapping into a `map` must split out meta writes.
14. **Variables are cleared at `apply` boundaries.** Don't assume `$x` set before `apply(...)` is visible inside the applied map.
15. **`root` inside a map is not the outer `root`.** It's a fresh value scoped to the map. Inner `root.x = ...` does not write to the outer document — the outer caller writes the map's result.
16. **Bare expression shorthand is single-statement-only**. `this.x.y` alone is `root = this.x.y`; adding any other statement makes it a parse error. Always emit explicit `root = ...` on rewrite.
17. **`nothing()` silently no-ops assignments**. Mappings relying on conditional `nothing()` returns to "skip" an assignment must be preserved; a naive conditional rewrite that always writes `null` changes semantics.
18. **`deleted()` has different meaning at each target level**. Whole-message delete (`root = deleted()`) vs. field removal (`root.x = deleted()`) vs. meta removal (`meta key = deleted()`). Migrations must preserve the target.
19. **`meta` assignment with bare identifier vs. quoted string**: `meta foo = v` and `meta "foo" = v` are equivalent. `meta(expr) = v` with a computed key is also valid.
20. **`@` alone is the whole metadata object**; `@foo` is `meta("foo")`. Don't confuse with `this.@foo` (which isn't valid).
21. **Plugin-registered functions and methods** are invisible without the plugin context. Tools should preserve unknown identifiers rather than reject.
22. **Imports resolve relative to the file**, not the mapping's logical location. When rewriting, rebase paths if the file moves.
23. **Recursive map calls** are allowed up to an environment-dependent depth. Don't flatten recursion during rewrite without checking that depth is bounded.
24. **Short-circuit evaluation of `&&`/`||` IS guaranteed by the implementation** (`query/arithmetic.go:396-442`), even though older docs hedge on this. `this != null && this.foo > 0` is safe regardless of null-safety of path access.
25. **Hex/binary/exponent numeric literals are not supported**. Source like `1e6`, `0x10`, or the short forms `.5` / `5.` is a parse error.
26. **Integer overflow is silent** — `(2^62) * 4` wraps per Go int64 semantics; there is no automatic promotion to float or big-int. A migration tool should flag large-constant arithmetic for review.
27. **Division and modulo by zero raise an error**, not `Inf` or `NaN`. `1 / 0` is `ErrDivideByZero`.
28. **Booleans are not orderable**. `true > false` is a type error — V1 refuses the comparison rather than using Go's `false < true` convention.
29. **Timestamp comparisons work by accident**: timestamps are RFC3339Nano-formatted and compared as strings, which happens to produce the right order for well-formed timestamps in the same timezone. Mixed-timezone or mixed-format timestamps may compare incorrectly.
30. **`==` across types usually returns `false` rather than erroring**. `5 == "5"` is `false`, not a type error (`query/type_helpers.go:839-892`). Migration tools may choose to preserve or normalise these.
31. **`.from(idx)` with negative or out-of-range index is implementation-defined** — depending on the `MsgBatch` implementation, it may panic, return `nil`, or wrap. Treat as suspect on migration.
32. **Count/counter stateful functions** (`count("name")`) persist state between messages. A mapping that uses them behaves differently when run in isolation vs. in a running pipeline. Tooling that evaluates mappings for migration testing must seed this state explicitly.
33. **Bracketed named-capture form** — `foo.(name -> body)` binds `name` but leaves `this` unchanged; `foo.(body)` rebinds `this` to `foo`. The two forms are semantically different even when `body` looks the same.
34. **`.map_each` treats `deleted()` and `nothing()` differently** — `deleted()` drops the element; `nothing()` keeps the original element unchanged. Do not substitute one for the other during rewrite.
35. **Lambda arguments pop the context**. In `items.map_each(x -> body)`, `body` executes with `this` = the **outer** context, not the element. Only the named parameter `x` refers to the element. Contrast `items.map_each(this.value)` (no lambda), where `this` IS the element. Migration tools must never mechanically wrap a non-lambda argument in `x -> ...` — semantics change. See §6.5 for the rule and `query/expression.go:166-175` for the implementation (`NamedContextFunction.Exec` calls `ctx.PopValue`).
36. **`.catch(...)` and `.or(...)` treat sentinels differently**. `.catch(x)` passes `deleted()` / `nothing()` through untouched; `.or(x)` replaces them with the fallback. They cannot be used interchangeably when sentinels are in play. See §9.4 and §12.2.
37. **Constant folding turns runtime errors into compile errors**. When both operands of an arithmetic, comparison, or coalesce operator are literal expressions, V1 evaluates the operation at parse time. Any type mismatch, divide-by-zero, or similar error is raised at parse time as a fatal compile error. `root = 5 / 0`, `root = true + false`, `root = null < 3` all fail at parse. The same expressions with one non-literal operand (e.g. `root = this.x / 0`) fail at runtime as recoverable errors. See §5.3 for the table.
38. **`==` is asymmetric for `bool` vs. `number` operands**. `true == 1` returns `true` (bool-path coerces number to bool via `IGetBool`); `1 == true` returns `false` (number-path cannot coerce bool to number). Never swap comparison operand order during rewrite. See §5.3 note on comparison asymmetry.
39. **`%` truncates float operands to `int64`** silently — `7.5 % 2.5` evaluates as `7 % 2 == 1`, not a type error. If the mapping intends an fmod-like operation, the V1 result will be wrong. See §5.3.
40. **String `.length()` returns byte count, not codepoint count**. `"héllo".length()` is `6` (é is 2 bytes UTF-8), `"🎉".length()` is `4`. Migration tools should flag any test or mapping that assumes codepoint semantics from `.length()` on a string. Array `.length()` and object `.length()` behave as expected (element / key count). For codepoint counts, V1 has no built-in equivalent to V2's string-length semantics — rewrite to `.split("").length()` or a regex-based count if migrated.
41. **Lambda bodies cannot start on a new line**. `items.map_each(x ->` followed by a newline and the body is a parse error — both sides of `->` use `SpacesAndTabs` only. Keep the lambda body on the same line as `->`, or move the whole expression inside `.(...)`/let-binding on one line. See §2.1.
42. **Arithmetic/comparison/logical operators reject a leading newline**. `a\n + b` is a parse error; `a +\n b` is fine. Migration tools pretty-printing long expressions must break *after* the operator, not before.
43. **Method/path dots cannot have whitespace before them**. `a.b` and `a.\n  b` work; `a .b`, `a\n .b`, `a\n.b` all fail. A mapping that pretty-prints method chains across lines must break *after* each `.`. See §2.1.
44. **`if`-without-matching-branch and non-matching `match` produce `nothing`, not `null`**. The resulting assignment is silently skipped (§8.3, §8.4, §9.4). `.catch()` cannot rescue this — there is no error to catch.
45. **`meta(<expr>) = v` is NOT a valid assignment target**, even though `meta("key")` as a read works. Only bare identifier or quoted string is accepted after `meta` on the LHS. See §6.4.
46. **Numeric path segments as write targets create OBJECT keys, not array indices**. `root.items.0 = "x"` produces `{"items": {"0": "x"}}`. No array gap-filling. See §6.3.
47. **`@` and `meta` refer to the SAME underlying map**; there is no copy-on-write separation between input-metadata reads and output-metadata writes. A later `@key` expression sees the most recent `meta key = …` write.
48. **`&&` and `||` coerce numbers to bool** via `IGetBool` — `true && 1` is `true`, `0 || true` is `true`. They are strict about strings/null/arrays/objects (which error). See §5.3.
49. **`.number()` always returns `float64`** — `"42".number()` is `42.0`, and `.sum()` / `.min()` / `.max()` on arrays return `float64` even when inputs are all-int. Integer methods like `.floor()` / `.round()` reduce to int64 when needed. See §3 and §9.0.
50. **V1 `assign` is a deep merge** (overrides per-key), and `merge` *combines* duplicate keys into arrays — this is the opposite of V2's naming. Read the method docs before rewriting either.
51. **`.all()` on an empty array returns `false`** in V1 (not `true` by vacuous truth). `.any()` on an empty array returns `false` as expected.
52. **`.fold()` is NOT curried in V1**. Despite occasional older docs showing `fold(init, tally -> value -> expr)`, the parser rejects that form as a name collision. The actual form is `.fold(init, item -> expr)` where `item` is an **object with `{tally, value}` fields**.
53. **`.reverse()` is string-only in V1 core**. Reversing arrays requires a comparator sort (`.sort(left > right)`) or the extension-only array `.reverse()` from `internal/impl/pure`.
54. **`.round()` in the core env takes no arguments**. `.round(2)` for decimal precision lives in `internal/impl/pure`; without it, use `(x * 100).round() / 100`.
55. **`.index()` silently truncates non-whole float arguments**. `[1,2,3].index(1.7)` behaves like `.index(1)`.
56. **`find()` on arrays returns Go `int`, not `int64`** — an unusual type-width quirk that can cause subtle comparison / arithmetic mismatches. Normalise with `.number()` if needed.
57. **`now()` returns a `string`**, not a `timestamp`. For a typed timestamp use `ts_parse(...)` (extension-only) on the string, or build a `time.Time` via other means.
58. **`range(a, b)` is a compile-time validated builtin**. `range(5, 5)` errors at parse. `range(0, 10, 3)` yields `[0, 3, 6]` (integer truncation of `(stop-start)/step`, not inclusive of the stop bound).
59. **`random_int` validates arguments at compile time** — negative `min`, `min > max`, etc. fail to parse.
60. **`format_json` HTML-escapes by default** (`<` → `<`) and returns the literal string `"null"` when called on an empty array.
61. **`.apply("name")` resolves the map name at runtime**, not compile time. `.apply("missing")` produces a runtime error, not a parse error — tools validating imports against a manifest should know this.
62. **Recursion-limit errors ARE catchable** via `.catch()` — they come through as ordinary runtime errors even though they originate deep in the interpreter.
63. **Error messages carry a prefix describing the failure source**: `"field \`this.x\`: <inner>"`, `"null literal: value is null"`, `"string literal: <inner>"`, `"number literal: attempted to divide by zero"`. Migration tools that substring-match on V1 errors should allow for these prefixes.
64. **The `throw` function's argument is named `why`** (not `msg`). Compile errors about wrong type/arity mention `why`, not `throw`: `"missing parameter: why"`, `"field why: wrong argument type, expected string, got number"`.
65. **Top-level statements in imported files** are silently accepted by the parser but do not establish bindings visible to subsequent `.apply()` calls. The var-scoping isolation makes those statements effectively dead code, and access from the caller raises a runtime `"variable 'x' undefined"` error.
66. **Whole-meta assignment requires an object**. `meta = "str"`, `meta = 5`, `meta = [1,2]` all raise runtime `"setting root meta object requires object value, received: <type>"`. See §6.4.
67. **Path collision on assignment raises a runtime error**. `root.user = "Alice"; root.user.name = "Jane"` fails with `"unable to set target path user.name as the value of user was a non-object type (string)"`. Migration tools should order/restructure assignments to avoid setting a scalar on a path prefix that is later extended.

---

## 15. Grammar Summary (Informal EBNF)

```
Mapping         = { Statement (Newline | EOF) }

Statement       = Assignment
                | LetBinding
                | MapDefinition
                | ImportStatement
                | FromStatement
                | RootLevelIf
                | BareExpression          # only if it's the sole statement

Assignment      = Target "=" Expression
Target          = "root" ("." PathSegment)*
                | "this" ("." PathSegment)*                 # legacy = root…
                | "meta" [ BareKey | QuotedString ]         # whole-meta or single key (no computed keys)
                | BareKey ("." PathSegment)*                # legacy bare path = root.…

LetBinding      = "let" (Ident | QuotedString) "=" Expression

MapDefinition   = "map" Ident "{" { Statement (Newline) } "}"

ImportStatement = "import" StringLiteral
FromStatement   = "from" StringLiteral

RootLevelIf     = "if" Expression "{" { Statement } "}"
                  { "else" "if" Expression "{" { Statement } "}" }
                  [ "else" "{" { Statement } "}" ]

BareExpression  = Expression                                # only as sole statement

Expression      = ArithmeticChain
ArithmeticChain = [ "-" ] Term { BinOp [ "-" ] Term }
BinOp           = "+" | "-" | "*" | "/" | "%"
                | "==" | "!=" | "<" | "<=" | ">" | ">="
                | "&&" | "||" | "|"

Term            = [ "!" ] Unary

Unary           = Primary { Tail }
Primary         = Literal
                | "this" | "root"
                | Ident                   # legacy root-scoped access (= this.Ident)
                | "$" Ident
                | "@" [ Ident | QuotedString ]
                | "(" Expression ")"
                | ArrayLiteral
                | ObjectLiteral
                | FunctionCall
                | IfExpression
                | MatchExpression
                | LambdaExpression

Tail            = "." PathSegment                          # field access
                | "." Ident "(" [ Args ] ")"               # method call
                | "." "(" Expression ")"                   # map expression

PathSegment     = Ident | QuotedString

FunctionCall    = Ident "(" [ Args ] ")"
Args            = Arg { "," Arg }          # all-positional or all-named, not mixed
Arg             = Expression | Ident ":" Expression

IfExpression    = "if" Expression "{" Expression "}"
                  { "else" "if" Expression "{" Expression "}" }
                  [ "else" "{" Expression "}" ]

MatchExpression = "match" [ Expression ] "{" MatchCase { Sep MatchCase } "}"
MatchCase       = ( "_" | Expression ) "=>" Expression
Sep             = "," | Newline

LambdaExpression= (Ident | "_") "->" Expression

ArrayLiteral    = "[" [ Expression { "," Expression } [ "," ] ] "]"
ObjectLiteral   = "{" [ ObjectMember { "," ObjectMember } [ "," ] ] "}"
ObjectMember    = (QuotedString | "(" Expression ")") ":" Expression

Literal         = IntLit | FloatLit | QuotedString | TripleQuotedString
                | "true" | "false" | "null"

IntLit          = Digit+
FloatLit        = Digit+ "." Digit+
QuotedString    = "\"" { EscapedChar | NonQuote } "\""
TripleQuotedString = "\"\"\"" { any } "\"\"\""
Ident           = [A-Za-z0-9_]+           # lenient (path segments, lambda params)
BareKey         = [a-z0-9_]+              # snake_case (function/method names, named args, meta keys)
```

This EBNF is *informal* — the real parser is a hand-written combinator with specific ordering and lookahead choices. For corner cases, consult `internal/bloblang/parser/`.

---

## 16. File Map

| Concern | Source |
|---------|--------|
| Parser entry and dispatch | `internal/bloblang/parser/mapping_parser.go`, `parser/query_parser.go` |
| Expression tails and paths | `parser/query_function_parser.go` |
| Arithmetic, precedence, coalesce | `parser/query_arithmetic_parser.go`, `query/arithmetic.go` |
| If / match / lambda / parens | `parser/query_expression_parser.go`, `parser/root_expression_parser.go` |
| Literals | `parser/query_literal_parser.go`, `parser/combinators.go` |
| Field interpolation dialect | `parser/field_parser.go`, `field/` |
| Assignment semantics | `mapping/assignment.go`, `mapping/statement.go` |
| Built-in functions | `query/functions*.go`, registry in `query/function_set.go` |
| Built-in methods | `query/methods*.go`, registry in `query/method_set.go` |
| Docs metadata for each spec | `query/docs.go`, `query/params.go` |
| Environment / plugin API | `internal/bloblang/environment.go`, `plugins/` |

---

## 17. Known Gaps in This Spec

This document is descriptive, not exhaustive on individual built-ins. In particular:

- **Per-function and per-method semantics** are not enumerated here. Use `query/docs.go` registrations and the online docs as the source of truth.
- **Deprecated builtins** are not individually listed. Enumerate by scanning the registry for `StatusDeprecated`.
- **Plugin-provided builtins** are inherently out of scope for a static document.
- **Implementation-defined behaviour** under extreme inputs (very large numbers, deep recursion, enormous strings) is not specified here; measure against the reference implementation.

When a migration tool encounters a construct not described here, default to: parse with the reference parser, preserve verbatim, and flag for human review.
