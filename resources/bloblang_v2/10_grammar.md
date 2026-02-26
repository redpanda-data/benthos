# 10. Grammar Reference

```
program         := top_level_statement*
top_level_statement := statement | map_decl | import_stmt
statement       := assignment | var_decl | if_stmt | match_stmt
assignment      := top_level_path '=' expression
var_decl        := '$' identifier '=' expression
map_decl        := 'map' identifier '(' [param_list] ')' '{' var_decl* expression '}'
param_list      := identifier (',' identifier)*
import_stmt     := 'import' string_literal 'as' identifier

expression      := literal | expr_path | function_call | method_chain |
                   control_expr | binary_expr | unary_expr |
                   lambda_expr | paren_expr
control_expr    := if_expr | match_expr

# Top-level paths (in assignments): only output or $variables (input is read-only)
top_level_path  := top_level_context_root path_component*
top_level_context_root := 'output' metadata_accessor? | var_ref

# Expression paths: allow bare identifiers (resolved by semantic analysis, see below)
expr_path       := expr_context_root path_component*
expr_context_root := ('output' | 'input') metadata_accessor? | var_ref | identifier

metadata_accessor := '@'
path_component  := '.' field_name | '?.' field_name | '[' expression ']' | '?[' expression ']'
field_name      := identifier | string_literal
var_ref         := '$' identifier

function_call   := (identifier | var_ref | qualified_name) '(' [arg_list] ')'
qualified_name  := identifier '::' identifier
method_chain    := expression ('.' identifier '(' [arg_list] ')')+

if_expr         := 'if' expression '{' expr_body '}'
                   ('else' 'if' expression '{' expr_body '}')*
                   ('else' '{' expr_body '}')?
if_stmt         := 'if' expression '{' stmt_body '}'
                   ('else' 'if' expression '{' stmt_body '}')*
                   ('else' '{' stmt_body '}')?
expr_body       := var_decl* expression
stmt_body       := statement*

match_expr      := 'match' expression ('as' identifier)? '{' expr_match_case (',' expr_match_case)* ','? '}'
                 | 'match' '{' expr_match_case (',' expr_match_case)* ','? '}'
match_stmt      := 'match' expression ('as' identifier)? '{' stmt_match_case (',' stmt_match_case)* ','? '}'
                 | 'match' '{' stmt_match_case (',' stmt_match_case)* ','? '}'
expr_match_case := (expression | '_') '=>' (expression | '{' expr_body '}')
stmt_match_case := (expression | '_') '=>' '{' stmt_body '}'

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
lambda_params   := identifier | '(' identifier (',' identifier)* ')'
lambda_block    := '{' var_decl* expression '}'
paren_expr      := '(' expression ')'

literal         := float_literal | int_literal | string_literal | boolean | null | array | object
int_literal     := [0-9]+
float_literal   := [0-9]+ '.' [0-9]+
string_literal  := '"' string_char* '"' | '`' raw_char* '`'
string_char     := [^"\\\n] | escape_seq
escape_seq      := '\\' ( '"' | '\\' | 'n' | 't' | 'r' | 'u' hex hex hex hex )
raw_char        := [^`]
array           := '[' [expression (',' expression)* ','?] ']'
object          := '{' [key_value (',' key_value)* ','?] '}'
key_value       := expression ':' expression

arg_list        := positional_args | named_args
positional_args := expression (',' expression)*
named_args      := identifier ':' expression (',' identifier ':' expression)*
```

## Key Points

- **Top-level only:** Map declarations (`map_decl`) and imports (`import_stmt`) can only appear at the top level of a program, not inside statement bodies. Control flow statements (`if_stmt`, `match_stmt`) can be nested
- **Variables:** `$var` for declaration and reference. Variable path assignment (`$var.field = expr`, `$var[0] = expr`) goes through the `assignment` production (not `var_decl`) and supports the same field access, indexing, and auto-creation semantics as `output`. Since it is an `assignment`, it is only available in statement contexts.
- **Metadata:** `input@.key` (read), `output@.key` (write)
- **Context-dependent paths:**
  - **Top-level assignments:** Must use `output`, `input`, or `$variable` (no bare identifiers)
  - **Map/lambda bodies:** Can use bare identifiers for parameters **in expressions only** (e.g., `data.field` where `data` is a parameter). Parameters are read-only and cannot be assigned to.
  - **Match with `as`:** Creates a read-only binding in expressions (e.g., `match input.x as val { val.field ... }`)
  - **Name resolution:** The grammar's `expr_context_root` accepts `identifier` in all expression contexts for simplicity. A separate semantic pass must verify that every bare identifier resolves to a bound name (map parameter, lambda parameter, or match `as` binding). Unresolved bare identifiers are a compile-time error (Section 3.1).
- **Quoted fields:** Use `."string"` for field names (dot required before quote): `input."field name"`
- **Object literals:** Keys are expressions that **must** evaluate to strings at runtime (error if not): `{"key": value}` or `{$var: value}`. Use `.string()` for explicit type conversion.
- **Indexing:** `[expr]` on objects (string index), arrays (numeric index), strings (codepoint position, returns int32), bytes (byte position, returns int32). Negative indices supported for arrays, strings, and bytes.
- **Null-safe:** `?.` and `?[` short-circuit to `null`
- **Map calls:** `name(arg)` or `namespace::name(arg)` (positional or named arguments)
- **Named arguments:** `func(a: 1, b: 2)` - cannot mix with positional arguments
- **Lambdas:** Single param `x -> expr`, multi-param `(a, b) -> expr`, block `x -> { ... }`. Lambda parameters are available as bare identifiers within the lambda body
- **Purity:**
  - Expressions cannot assign to `output` or `output@`
  - Lambda blocks: Variable declarations + final expression (pure, no side effects)
  - Map bodies: Same as lambda blocks - isolated functions that return values
  - Maps cannot reference `input`, `output`, or top-level `$variables` (only their parameters)
- **Control flow forms:**
  - `if_expr` / `match_expr`: Used in assignments, contain `expr_body` (no `output` assignments)
  - `if_stmt` / `match_stmt`: Standalone statements, contain `stmt_body` (may assign to `output`)
  - `expr_body`: Variable declarations + final expression (must be pure)
  - `stmt_body`: Zero or more statements (no trailing expression). Empty bodies are valid (no-op).
- **Void:** Not represented in the grammar — void is a semantic concept (absence of a value), not a syntactic form. It arises from if-expressions without `else` when the condition is false. The grammar cannot distinguish expressions that may produce void from those that always produce values; this is a runtime semantic. See Section 4.1 for full void behavior.
- **Type coercion:** `+` requires same type family (no cross-family implicit conversion). Numeric types are promoted using promotion rules; non-numeric types require exact type match.
- **Operator precedence and associativity:** The `binary_expr` production is a simplified flat rule. Implementations must apply the precedence, associativity, and non-associativity rules from Section 3.2. Precedence (high to low): field access > unary > multiplicative > additive > comparison > equality > logical AND > logical OR. Arithmetic and logical operators are left-associative; comparison and equality operators are non-associative (chaining is a parse error).

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
  data.field * 2                 # ✅ Valid: 'data' in expression
}

map invalid(data) {
  data = input.x                 # ❌ Invalid: cannot assign to parameter
  data.field = 10                # ❌ Invalid: cannot assign through parameter
}
```

**Lambda body (bare identifiers only in expressions):**
```bloblang
input.items.map_array(item -> item.value * 2)  # ✅ Valid: 'item' in expression
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
