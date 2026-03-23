# Bloblang V2 Implementation Plan

Hand-written recursive descent + Pratt parsing implementation in Go.

## Architecture

```
internal/bloblang2/
├── syntax/         — Token types, scanner, AST nodes, parser, name resolution
├── eval/           — Tree-walking interpreter, standard library
└── bloblang2.go    — Public entry point (implements bloblspec.Interpreter)
```

Two packages: `syntax` for "what the code looks like" and `eval` for "what the code does." The scanner is an unexported implementation detail of `syntax`.

## Phase 1: Token Types and Scanner

### Token types (`syntax` package)

Define all token types as an enum:
- Keywords: `input`, `output`, `if`, `else`, `match`, `as`, `map`, `import`, `true`, `false`, `null`, `_`
- Reserved names: `deleted`, `throw`
- Operators: `.`, `?.`, `@`, `::`, `=`, `+`, `-`, `*`, `/`, `%`, `!`, `>`, `>=`, `==`, `!=`, `<`, `<=`, `&&`, `||`, `=>`, `->`
- Delimiters: `(`, `)`, `{`, `}`, `[`, `]`, `?[`, `,`, `:`
- Literals: `INT`, `FLOAT`, `STRING`, `RAW_STRING`
- Special: `VAR` (single token for `$name`), `IDENT`
- Note: No separate `WORD` token. Keywords scan as their specific types (INPUT, MAP, etc.). The parser treats any keyword token as a valid field name after `.` or `?.`.
- Control: `NL` (newline separator), `EOF`

Each token carries: type, literal string, source position (file, line, column).

### Scanner (unexported in `syntax` package)

Lexer with three newline suppression mechanisms:

1. **Paren/bracket nesting**: track `()` and `[]` depth; suppress NL when depth > 0. Braces `{}` do NOT suppress (newlines are significant inside block bodies).

2. **Postfix continuation**: after scanning a NL, peek at next non-whitespace token. If it's `.`, `?.`, `[`, `?[`, or `else`, suppress the NL.

3. **Operator continuation**: after scanning a token that cannot end a complete expression (binary operators, unary operators, `=`, `=>`, `->`, `:`), suppress the following NL.

The scanner handles:
- Integer literals: `[0-9]+`, validated to fit int64 range at scan time (compile error if not)
- Float literals: `[0-9]+.[0-9]+`, requiring digits on both sides
- String literals: `"..."` with escape sequences (`\\`, `\"`, `\n`, `\t`, `\r`, `\uXXXX`, `\u{X...}`)
- Raw strings: `` `...` `` with no escape processing
- Comments: `#` to end of line (stripped by scanner)
- `$identifier` scanned as a single `VAR` token (e.g., `$count` → `VAR("count")`)
- `?[` scanned as a single token (not `?` + `[`)
- `?.` scanned as a single token (not `?` + `.`)

## Phase 2: AST Node Types

### AST nodes (`syntax` package)

All nodes carry source position for error reporting.

**Top-level:**
- `Program` — list of top-level statements, map declarations keyed by namespace
- `MapDecl` — name, params (with defaults, discards), body (expr body)
- `ImportStmt` — path string, namespace alias

**Statements (can assign to output/output@/variables):**
- `Assignment` — target expression (parsed as a path), `=`, value expression
- `IfStmt` — condition, body (statements), else-if chain, else body
- `MatchStmt` — subject expression, optional `as` binding, cases (each with statement body)

**Expressions (return values, no output side effects):**
- `IfExpr` — condition, body (expr body), else-if chain, else body
- `MatchExpr` — subject, optional `as` binding, cases (each with expression or expr body)
- `BinaryExpr` — left, operator, right
- `UnaryExpr` — operator, operand
- `CallExpr` — name (identifier or qualified `namespace::name`), args (positional or named)
- `MethodCallExpr` — receiver, method name, args, null-safe flag
- `FieldAccessExpr` — receiver, field name (string), null-safe flag
- `IndexExpr` — receiver, index expression, null-safe flag
- `LambdaExpr` — params (with defaults, discards), body (expression or block)
- `LiteralExpr` — int, float, string, bool, null
- `ArrayLiteral` — elements
- `ObjectLiteral` — key-value pairs (keys are expressions)
- `InputExpr` — atom for `input` keyword
- `InputMetaExpr` — atom for `input@`
- `OutputExpr` — atom for `output` keyword (read in expression context)
- `OutputMetaExpr` — atom for `output@` (read in expression context)
- `VarExpr` — atom for `$name` variable reference
- `IdentExpr` — bare identifier (resolved by name resolution to parameter, map, or stdlib function)

**Expression body** (`ExprBody`): list of variable assignments + final expression.

**Path expressions:** `InputExpr`, `OutputExpr`, `VarExpr` are pure atoms in the Pratt parser. All path operations (`.field`, `[index]`, `.method()`) are uniform postfix nodes wrapping any receiver expression. After parsing, a post-parse pass collapses postfix chains rooted at `InputExpr`, `OutputExpr`, `VarExpr`, `InputMetaExpr`, or `OutputMetaExpr` into flat `PathExpr` nodes containing a root and a list of path segments (field name, index expression, or method call). This gives the interpreter efficient single-node path traversal instead of recursive chain unwinding.

**`PathExpr`** — root (input/output/var/input@/output@), segments `[]PathSegment` where each segment is one of:
- `FieldAccess` — name string, null-safe flag
- `Index` — expression, null-safe flag
- `MethodCall` — name string, args, null-safe flag

## Phase 3: Parser

### Parser (`syntax` package)

**Error recovery:** On parse error, skip tokens until the next NL (statement separator) and resume parsing top-level statements. Collect all errors with source positions. `CompileError` holds a list of individual positioned errors.

**Top-level parsing:**
- `parseProgram()` — loop: `parseTopLevelStatement()` separated by NL
- `parseTopLevelStatement()` — dispatch on token: `map` → map decl, `import` → import, `if` → if stmt, `match` → match stmt, else → assignment

**Import resolution:** Parser resolves imports inline by recursing into itself. On `import "file.blobl" as ns`, the parser looks up the filename in the `files` map, creates a new scanner, parses it (only map declarations and imports allowed), and attaches the maps under the namespace. Circular imports detected by tracking the "currently parsing" file set. Errors (file not found, circular, statements in imported file) caught at parse time.

**Statement parsing (context: statement):**
- `parseAssignment()` — parse target (`output`/`output@`/`$var` + path), `=`, expression
- `parseIfStmt()` — condition, `{`, statement body, `}`, optional else chain
- `parseMatchStmt()` — subject, optional `as`, `{`, cases with statement bodies, `}`
- `parseStmtBody()` — zero or more statements (no trailing expression)

**Expression parsing (context: expression):**
- `parseExpr()` — entry point, delegates to Pratt parser
- `parseIfExpr()` — condition, `{`, expr body, `}`, optional else chain
- `parseMatchExpr()` — subject, optional `as`, `{`, cases with expressions, `}`
- `parseExprBody()` — zero or more var assignments + final expression
- `parseLambda()` — params, `->`, body (expression or `{` expr body `}`)

**Pratt parser:**

Binding powers (higher = tighter):

| Level | Operators | BP | Associativity |
|-------|-----------|-----|---------------|
| 7 | Postfix (`.`, `?.`, `[`, `?[`, `.method()`) | 140 | left |
| 6 | Unary (`!`, `-`) | 120 | prefix |
| 5 | Multiplicative (`*`, `/`, `%`) | 100 | left |
| 4 | Additive (`+`, `-`) | 80 | left |
| 3 | Comparison (`>`, `>=`, `<`, `<=`) | 60 | non-assoc |
| 2 | Equality (`==`, `!=`) | 40 | non-assoc |
| 1 | Logical AND (`&&`) | 20 | left |
| 0 | Logical OR (`\|\|`) | 10 | left |

Non-associative operators: left BP = right BP. After parsing a binary expression with a non-associative operator, check if the next token is another non-associative operator at the same level — if so, emit a parse error.

**Null-denotation (prefix/atom) handlers:**
- Literals (int, float, string, bool, null)
- `(` → grouped expression or multi-param lambda
- `[` → array literal
- `{` → object literal
- `!` → unary not
- `-` → unary minus
- `if` → if expression
- `match` → match expression
- `input` → InputExpr atom (with `@` check for InputMetaExpr)
- `output` → OutputExpr atom (with `@` check for OutputMetaExpr)
- `VAR` → VarExpr atom
- `IDENT` → bare identifier (call, lambda, or name reference)

**Left-denotation (infix/postfix) handlers:**
- Binary operators: `+`, `-`, `*`, `/`, `%`, `==`, `!=`, `>`, `>=`, `<`, `<=`, `&&`, `||`
- `.` → peek ahead: if WORD + `(`, method call; else field access
- `?.` → same but null-safe
- `[` → index access
- `?[` → null-safe index access

**Context passing:** The parser tracks whether it's in a statement context or expression context. This determines:
- Whether `if`/`match` bodies use `parseStmtBody()` or `parseExprBody()`
- Whether assignments to `output`/`output@` are allowed

### Name resolution (semantic pass in `syntax` package)

After parsing, a separate pass resolves bare identifiers to their bindings (parameters, variables, map names, stdlib functions). The resolver receives an injected set of known method and function names from the caller (the `eval` package provides the default stdlib set). This keeps `syntax` decoupled from `eval` and supports extensibility.

The resolver checks:
- Unresolved identifiers → compile error
- Map isolation (no `input`/`output` references inside map bodies)
- Lambda purity (no `output`/`output@` assignments)
- Parameter read-only enforcement
- Match form validation (boolean cases in equality match, non-boolean in as/boolean match)
- Boolean literal cases in equality match → compile error
- Unknown method/function names → compile error (checked against the injected set)

## Phase 4: Interpreter

### Interpreter (`eval` package)

Tree-walking interpreter that evaluates the AST.

**Value representation:** Use native Go types per the bloblspec contract:
- string, int32, int64, uint32, uint64, float32, float64, bool, nil, []byte, time.Time, []any, map[string]any

**Special values** (unexported struct types behind `any`):
- `voidVal struct{}` — singleton, triggers assignment skip or error depending on context
- `deletedVal struct{}` — singleton, triggers field removal, element omission, or message drop
- `errorVal struct{ message string }` — carries error message, propagates through postfix chains until `.catch()`

**Scope chain:** Linked scopes with mode flag:
- Each scope has: `parent *scope`, `mode` (expression/statement), `vars map[string]any`
- Expression mode: assignment always writes locally (shadow)
- Statement mode: if variable exists in ancestor, update ancestor; otherwise create locally
- Map bodies: `parent: nil` (fully isolated — no access to input/output/outer variables)
- Lambda bodies inside maps: also isolated (inherit map's nil parent)
- Lambda bodies at top level: can read input/output (inherit top-level scope as parent)

**Copy-on-write:** Deep-clone maps, slices, and `[]byte` on variable assignment. Simple values (numbers, strings, bools, `time.Time`) are naturally copied by Go value semantics. Deferred COW wrappers are a potential follow-up optimization.

**Execution model:**
- Output starts as `map[string]any{}`
- Output metadata starts as `map[string]any{}`
- Input and input metadata are immutable (COW on variable assignment)
- Statements execute top-to-bottom
- `output = deleted()` sets a flag and returns immediately
- Map declarations are hoisted (collected before execution)

**Numeric promotion:** Implement the spec's promotion rules:
- Same type → no promotion
- Same signedness, different width → wider type
- Signed + unsigned → int64 (error if uint64 > 2^63-1)
- Any integer + any float → float64 (error if integer > 2^53)
- Division always → float (f32/f32 → f32, all else → f64)
- Integer overflow → runtime error (checked arithmetic)

**Error handling:**
- Errors propagate through expressions (skip postfix operations)
- `.catch()` intercepts errors, passes void/deleted through
- `.or()` intercepts null/void/deleted, short-circuits argument evaluation
- Recursion depth tracked, exceeding limit is uncatchable
- `throw()` produces a catchable error

## Phase 5: Standard Library

### Standard library (`eval` package)

Implement all functions and methods from Section 13 of the spec. Register them in function/method tables that are injected into the parser (for compile-time name validation) and used by the interpreter (for runtime dispatch).

**Functions:** uuid_v4, now, random_int, range, timestamp, second/minute/hour/day, throw, deleted

**Methods by category:**
- Type conversion: .string(), .int32(), .int64(), .uint32(), .uint64(), .float32(), .float64(), .bool(), .char(), .bytes()
- Type introspection: .type()
- Sequence: .length(), .contains(), .index_of(), .slice(), .reverse()
- String: .uppercase(), .lowercase(), .trim(), .trim_prefix(), .trim_suffix(), .has_prefix(), .has_suffix(), .split(), .replace_all(), .repeat(), .re_match(), .re_find_all(), .re_replace_all()
- Array: .filter(), .map(), .sort(), .sort_by(), .append(), .concat(), .flatten(), .unique(), .without_index(), .enumerate(), .any(), .all(), .find(), .join(), .sum(), .min(), .max(), .fold(), .collect()
- Object: .iter(), .keys(), .values(), .has_key(), .merge(), .without(), .map_values(), .map_keys(), .map_entries(), .filter_entries()
- Numeric: .abs(), .floor(), .ceil(), .round()
- Timestamp: .ts_parse(), .ts_format(), .ts_unix(), .ts_unix_milli(), .ts_unix_micro(), .ts_unix_nano(), .ts_from_unix(), .ts_from_unix_milli(), .ts_from_unix_micro(), .ts_from_unix_nano(), .ts_add()
- Error handling: .catch(), .or(), .not_null()
- Encoding: .parse_json(), .format_json(), .encode(), .decode()

**Intrinsic methods** (require special handling, not regular dispatch):
- `.catch()` — activates on error, passes through on success/void/deleted
- `.or()` — short-circuit evaluation, rescues null/void/deleted
- `throw()` — produces error
- `deleted()` — produces deletion marker

## Phase 6: Integration

### `bloblang2.go`

Implement `bloblspec.Interpreter` interface:
- `Compile(mapping, files)` → scan, parse, resolve names (with stdlib method/function sets), return a `Mapping`
- `Mapping.Exec(input, metadata)` → execute the AST, return output/metadata/deleted/error

### Test harness

```go
func TestBloblangV2Spec(t *testing.T) {
    bloblspec.RunT(t, "../../resources/bloblang_v2/tests", &bloblang2.Interpreter{})
}
```

## Implementation Order

1. **Token types** — zero dependencies, defines the vocabulary
2. **Scanner** — depends on token types, testable in isolation
3. **AST nodes** — zero dependencies, defines the tree structure
4. **Parser** — depends on scanner + AST, testable against expected AST shapes
5. **Name resolution** — depends on AST, validates scope/isolation/method names
6. **Interpreter core** — depends on AST, testable with hand-built AST nodes
7. **Standard library** — depends on interpreter value types, each method testable in isolation
8. **Integration** — wire up bloblspec.Interpreter, run the full test suite

Each phase should be independently testable. The spec test suite (bloblspec) provides the end-to-end validation at phase 8, but each phase should also have unit tests for its internal logic.
