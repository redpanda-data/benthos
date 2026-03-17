# 10. Grammar Reference

**Statement separation:** Statements are separated by newlines. Multiple statements on a single line are not allowed — each statement must begin on its own line. Newlines inside parentheses `()` and brackets `[]` are treated as whitespace and do not produce separator tokens, allowing argument lists, array literals, and grouped expressions to span multiple lines freely. Newlines inside braces `{}` are still significant — they separate statements within block bodies (if/match/map/lambda). In object literals (also delimited by `{}`), entries are comma-separated, so newlines between entries are ignored.

**Postfix continuation:** Newlines are also suppressed when the next line begins with a postfix operator token — `.`, `?.`, `[`, or `?[`. This allows method chains and indexing to span multiple lines without parentheses:
```bloblang
output.result = input.text
  .trim()
  .lowercase()
  .replace_all(" ", "-")
```
This is safe because none of these tokens can begin a valid statement (`statement := assignment | if_stmt | match_stmt`), so there is no ambiguity between continuation and a new statement.

```
# --- Lexical: newline handling ---
# NL represents one or more newline characters that act as statement separators.
# Inside parentheses () and brackets [] — newlines are treated as whitespace
# and do not produce NL tokens. The lexer tracks () and [] nesting depth; NL
# tokens are suppressed when inside these delimiters.
# Inside braces {} — newlines still produce NL tokens (needed to separate
# statements in block bodies). Object literals use comma separation, so NL
# tokens between entries are simply consumed as optional whitespace by the
# comma-separated list productions.
# Postfix continuation — NL tokens are suppressed when the next non-whitespace
# token is a postfix operator: '.', '?.', '[', or '?['. This enables multi-line
# method chains and indexing without requiring parentheses.
# Blank lines (consecutive newlines) and trailing newlines are allowed and
# collapsed — NL means "one or more newline boundaries."
# A program may optionally begin or end with NL (leading/trailing blank lines).

program         := NL? (top_level_statement (NL top_level_statement)*)? NL?
top_level_statement := statement | map_decl | import_stmt
statement       := assignment | if_stmt | match_stmt

# Statement contexts (top-level, if/match statement bodies): can assign to output, metadata, or variables
assignment      := assign_target '=' expression
assign_target   := 'output' metadata_accessor? path_component* | var_ref path_component*

# Expression contexts (map bodies, lambda blocks, if/match expressions): can only assign to variables
var_assignment  := var_ref path_component* '=' expression

map_decl        := 'map' identifier '(' [param_list] ')' '{' NL? (var_assignment NL)* expression NL? '}'
param_list      := param (',' param)*
param           := identifier | identifier '=' literal | '_'
import_stmt     := 'import' string_literal 'as' identifier

expression      := postfix_expr | binary_expr | unary_expr | lambda_expr
control_expr    := if_expr | match_expr

# Postfix expressions: a primary followed by zero or more field access, indexing, or method calls.
# This unified production allows chaining any postfix operation on any expression result:
#   input.items.filter(x -> x > 0)[0]       — index a method result
#   extract_user(input.data).name            — field access on a call result
#   "a,b,c".split(",")[0]                    — index a method on a literal
postfix_expr    := primary_expr postfix_op*
primary_expr    := literal | context_root | call_expr | control_expr | paren_expr
context_root    := ('output' | 'input') metadata_accessor? | var_ref | qualified_name | identifier

postfix_op      := path_component | method_call
path_component  := '.' field_name | '?.' field_name | '[' expression ']' | '?[' expression ']'
# Note: '.' field_name and '.' word '(' ... ')' both start with '.'. Disambiguation:
# after '.' word, if '(' follows it is a method call; otherwise it is field access.
method_call     := '.' word '(' [arg_list] ')' | '?.' word '(' [arg_list] ')'

metadata_accessor := '@'
field_name      := word | string_literal
var_ref         := '$' identifier

call_expr       := (identifier | var_ref | qualified_name | reserved_name) '(' [arg_list] ')'
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

# Note: The grammar uses the same case production for all three match forms
# (equality, boolean with 'as', boolean without expression). The distinction
# between these forms is semantic, not syntactic. A semantic pass must enforce:
#   - With 'as': case expressions must evaluate to boolean (Section 4.2)
#   - Without 'as' (equality form): case expressions that evaluate to boolean
#     are a runtime error (Section 4.2)
#   - Without expression: case expressions must evaluate to boolean (Section 4.2)

# Note: this is a simplified flat production. Operator precedence, associativity,
# and non-associativity rules are defined in Section 3.2 and must be applied by
# the parser. In particular, chaining non-associative operators (e.g., a < b < c)
# is a parse error.
binary_expr     := expression binary_op expression
binary_op       := '+' | '-' | '*' | '/' | '%' |
                   '==' | '!=' | '>' | '>=' | '<' | '<=' | '&&' | '||'
unary_expr      := unary_op expression
unary_op        := '!' | '-'

lambda_expr     := lambda_params '->' (expression | lambda_block)
lambda_params   := identifier | '_' | '(' param (',' param)* ')'
lambda_block    := '{' NL? (var_assignment NL)* expression NL? '}'
paren_expr      := '(' expression ')'

literal         := float_literal | int_literal | string_literal | boolean | null | array | object
int_literal     := [0-9]+                          # Must fit int64; overflow is a compile-time error
float_literal   := [0-9]+ '.' [0-9]+
boolean         := 'true' | 'false'
null            := 'null'
string_literal  := '"' string_char* '"' | '`' raw_char* '`'
string_char     := [^"\\\n] | escape_seq
escape_seq      := '\\' ( '"' | '\\' | 'n' | 't' | 'r' | 'u' hex hex hex hex | 'u{' hex+ '}' )
                   # \uXXXX: exactly 4 hex digits (BMP only, U+0000–U+FFFF)
                   # \u{X...}: 1–6 hex digits (any valid Unicode codepoint, U+0000–U+10FFFF)
                   # Surrogate codepoints (U+D800–U+DFFF) are invalid in both forms
hex             := [0-9a-fA-F]
raw_char        := [^`]
array           := '[' [expression (',' expression)* ','?] ']'
object          := '{' NL? [key_value (',' NL? key_value)* ','?] NL? '}'
key_value       := expression ':' expression

arg_list        := positional_args | named_args
positional_args := expression (',' expression)* ','?
named_args      := identifier ':' expression (',' identifier ':' expression)* ','?
word            := [a-zA-Z_][a-zA-Z0-9_]*          # Raw lexical pattern (includes keywords and reserved names)
identifier      := word - keyword - reserved_name    # Excludes keywords and reserved names; used for variable/map/param names
keyword         := 'input' | 'output' | 'if' | 'else' | 'match' | 'as' | 'map' | 'import' | 'true' | 'false' | 'null' | '_'
reserved_name   := 'deleted' | 'throw'              # Reserved function names (Section 1.3); cannot be used as identifiers
```

## Key Points

- **Disambiguation of `match` with `{`:** After `match`, if the next token is `{`, it is always the match body (boolean match without expression), never an object literal as the matched expression. This eliminates the ambiguity between `match { cases... }` and `match <object_literal> { cases... }`. In practice, matching on a literal is dead code — the matched expression should always be dynamic. If parenthesization is ever needed for clarity, `match (expr) { ... }` works for any expression.
- **Disambiguation of `call_expr` vs `context_root`:** Both productions can start with `identifier` (or `qualified_name`). The parser must use one token of lookahead after the identifier (or after the second identifier in `qualified_name`) to check for `(`: if present, it is a `call_expr`; otherwise, it is a `context_root`. Reserved names (`deleted`, `throw`) always require `(` — they appear in `call_expr` but not `context_root`, so they can only be called, not used as bare values. This is standard LL(1) lookahead — the grammar is unambiguous but implementers should be aware of the need for it.
- **Unified postfix chains:** The `postfix_expr` production unifies field access, indexing, and method calls into a single chain. Any expression result can be followed by any combination of `.field`, `[index]`, and `.method()` operations. This means `func().field`, `expr.method()[0]`, and `literal["key"]` are all valid.
- **Top-level only:** Map declarations (`map_decl`) and imports (`import_stmt`) can only appear at the top level of a program, not inside statement bodies. Control flow statements (`if_stmt`, `match_stmt`) can be nested.
- **Variables:** `$var` for declaration and reference. The grammar has two assignment productions that reflect context restrictions: `assignment` (used in statement contexts: top-level, if/match statement bodies) can target `output`, `output@`, or `$variables`; `var_assignment` (used in expression contexts: map bodies, lambda blocks, if/match expressions) can only target `$variables`. Both support path assignment (`$var.field = expr`, `$var[0] = expr`) with the same field access, indexing, and auto-creation semantics as `output`.
- **Metadata:** `input@.key` (read), `output@.key` (write). Root metadata assignment (`output@ = expr`) requires the value to be an object at runtime (error otherwise); `output@ = deleted()` is also an error since metadata cannot be deleted (Section 9.2).
- **Context-dependent paths:**
  - **Top-level assignments:** Targets must be `output` (with optional `@` for metadata) or `$variable`. `input` is read-only and cannot be assigned to.
  - **Map/lambda bodies:** Can use bare identifiers for parameters **in expressions only** (e.g., `data.field` where `data` is a parameter). Parameters are read-only and cannot be assigned to.
  - **Match with `as`:** Creates a read-only binding in expressions (e.g., `match input.x as val { val.field ... }`)
  - **Name resolution:** The grammar's `context_root` accepts `identifier` and `qualified_name` in all expression contexts. A separate semantic pass must verify that every bare identifier resolves to a bound name — a map parameter, lambda parameter, match `as` binding, map name, or standard library function name (Section 5.5). Namespace-qualified references (`namespace::name`) resolve to map values from imported modules. Unresolved identifiers are a compile-time error (Section 3.1). Resolution priority (innermost wins): parameters > maps > standard library functions. User-defined maps shadow standard library functions of the same name.
- **Field and method names:** Field names after `.` and `?.` use `word` (any `[a-zA-Z_][a-zA-Z0-9_]*` token, including keywords). Keywords are valid as field names without quoting: `input.map`, `output.if`. Use `."string"` for names with special characters or spaces: `input."field name"`. Method names in `method_call` also use `word`, so standard library methods like `.map()` work despite `map` being a keyword. Declarations (variables, maps, parameters) use `identifier`, which excludes keywords.
- **Object literals:** Keys are expressions that **must** evaluate to strings at runtime (error if not): `{"key": value}` or `{$var: value}`. Use `.string()` for explicit type conversion.
- **Indexing:** `[expr]` on objects (string index), arrays (numeric index), strings (codepoint position, returns int64), bytes (byte position, returns int64). Negative indices supported for arrays, strings, and bytes. Indexing is a `postfix_op` and can follow any expression — including function calls, method chains, and literals.
- **Null-safe:** `?.` and `?[` short-circuit to `null` for field access and indexing; `?.method()` short-circuits to `null` for method calls.
- **Map calls:** `name(arg)` or `namespace::name(arg)` (positional or named arguments). Maps and standard library functions are first-class: `name` or `namespace::name` without parentheses evaluates to a lambda value (Section 5.5).
- **Named arguments:** `func(a: 1, b: 2)` - cannot mix with positional arguments. Duplicate named arguments are a compile-time error.
- **Default parameters:** `map foo(x, y = 10) { ... }` or `(x, y = 10) -> expr`. Parameters with defaults must come after required parameters. Default values must be literals (`42`, `"hello"`, `true`, `false`, `null`). Discard parameters (`_`) cannot have defaults.
- **Discard parameters:** `_` is allowed as a parameter in maps and lambdas. It accepts an argument but does not bind it — `_` cannot be referenced in the body. Multiple `_` parameters are allowed. Maps or lambdas with `_` parameters can only be called positionally (named calls are a compile error).
- **Arity:** Positional calls must provide at least the required parameter count and at most the total count. Named calls must provide all required parameters; missing parameters with defaults use their defaults. Extra or unknown arguments are errors. Arity mismatches are compile-time errors when detectable, runtime errors otherwise.
- **Lambdas:** Single param `x -> expr`, multi-param `(a, b) -> expr`, with defaults `(a, b = 0) -> expr`, discard `_ -> expr` or `(_, b) -> expr`, block `x -> { ... }`. Lambda parameters are available as bare identifiers within the lambda body. `_` parameters are not bound and cannot be referenced.
- **Side effects:**
  - Expressions cannot assign to `output` or `output@`
  - Lambda blocks: Variable assignments + final expression (no `output` side effects)
  - Map bodies: Same as lambda blocks — isolated functions that return values
  - Maps cannot reference `input`, `output`, or top-level `$variables` (only their parameters)
- **Control flow forms:**
  - `if_expr` / `match_expr`: Used in assignments, contain `expr_body` (no `output` assignments)
  - `if_stmt` / `match_stmt`: Standalone statements, contain `stmt_body` (may assign to `output`)
  - `expr_body`: Variable assignments + final expression (no `output` side effects)
  - `stmt_body`: Zero or more statements (no trailing expression). Empty bodies are valid (no-op).
- **Void:** Not represented in the grammar — void is a semantic concept (absence of a value), not a syntactic form. It arises from if-expressions without `else` when the condition is false, or from match expressions without `_` when no case matches. Void is a purely runtime semantic: if void reaches a variable declaration (the first assignment to a name in a scope), it is a **runtime error** (Section 4.1). This ensures every declared variable always has a value.
- **Type coercion:** `+` requires same type family (no cross-family implicit conversion). Numeric types are promoted using promotion rules; non-numeric types require exact type match.
- **Operator precedence and associativity:** The `binary_expr` production is a simplified flat rule. Implementations must apply the precedence, associativity, and non-associativity rules from Section 3.2. Precedence (high to low): postfix operations (field access, indexing, method calls) > unary > multiplicative > additive > comparison > equality > logical AND > logical OR. Arithmetic and logical operators are left-associative; comparison and equality operators are non-associative (chaining is a parse error).
- **`{}` disambiguation:** In contexts where `{}` could be either an empty object literal or an empty block (e.g., inside a map body or lambda block), it is parsed as an empty object literal. Blocks (`expr_body`, `lambda_block`) require at least one expression, so an empty `{}` cannot be a valid block.

## Context Examples

**Top-level assignments:**
```bloblang
output.result = input.value      # ✅ Valid: explicit context roots
$var = input.x                   # ✅ Valid: variable declaration
output.y = $var                  # ✅ Valid: variable in expression

result = input.value             # ❌ Invalid: bare identifier not allowed at top-level
```

**Map body (bare identifiers only in expressions, never assignments):**
```bloblang
map transform(data) {
  $temp = data.field             # ✅ Valid: 'data' in expression (RHS)
  $temp.extra = "added"          # ✅ Valid: variable path assignment
  data.field * 2                 # ✅ Valid: 'data' in expression
}

map invalid(data) {
  data = input.x                 # ❌ Invalid: cannot assign to parameter
  data.field = 10                # ❌ Invalid: cannot assign through parameter
  output.x = data.field          # ❌ Invalid: cannot assign to output in map body
}
```

**Lambda body (bare identifiers only in expressions):**
```bloblang
input.items.map(item -> item.value * 2)  # ✅ Valid: 'item' in expression
input.items.filter(x -> x.active)              # ✅ Valid: 'x' in expression
```

**Match with `as` (creates read-only binding):**
```bloblang
output.result = match input.user as u {
  u.type == "admin" => u.name,   # ✅ Valid: 'u' in expression
  _ => "guest",
}
```

**Key rule:** Bare identifiers (parameters and match `as` bindings) are **read-only** and can only appear in expressions, never as assignment targets.
