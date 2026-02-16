# 10. Grammar Reference

```
program         := top_level_statement*
top_level_statement := statement | map_decl | import_stmt
statement       := assignment | var_decl | if_stmt | match_stmt
assignment      := top_level_path '=' expression
var_decl        := '$' identifier '=' expression
map_decl        := 'map' identifier '(' param_list ')' '{' var_decl* expression '}'
param_list      := identifier (',' identifier)*
import_stmt     := 'import' string_literal 'as' identifier

expression      := literal | expr_path | function_call | method_chain |
                   control_expr | binary_expr | unary_expr |
                   lambda_expr | paren_expr
control_expr    := if_expr | match_expr

# Top-level paths (in assignments): no bare identifiers
top_level_path  := top_level_context_root path_component*
top_level_context_root := ('output' | 'input') metadata_accessor? | var_ref

# Expression paths (in maps/lambdas/match-as): allow bare identifiers
expr_path       := expr_context_root path_component*
expr_context_root := ('output' | 'input') metadata_accessor? | var_ref | identifier

metadata_accessor := '@'
path_component  := '.' field_name | '?.' field_name | '[' expression ']' | '?[' expression ']'
field_name      := identifier | string_literal
var_ref         := '$' identifier

function_call   := (identifier | var_ref | qualified_name) '(' [arg_list] ')'
qualified_name  := identifier '.' identifier
method_chain    := expression ('.' identifier '(' [arg_list] ')')+

if_expr         := 'if' expression '{' expr_body '}'
                   ('else' 'if' expression '{' expr_body '}')*
                   ('else' '{' expr_body '}')?
if_stmt         := 'if' expression '{' stmt_body '}'
                   ('else' 'if' expression '{' stmt_body '}')*
                   ('else' '{' stmt_body '}')?
expr_body       := var_decl* expression
stmt_body       := statement+

match_expr      := 'match' expression ('as' identifier)? '{' (expr_match_case ',')+ '}'
                 | 'match' '{' (expr_match_case ',')+ '}'
match_stmt      := 'match' expression ('as' identifier)? '{' (stmt_match_case ',')+ '}'
                 | 'match' '{' (stmt_match_case ',')+ '}'
expr_match_case := (expression | '_') '=>' (expression | '{' expr_body '}')
stmt_match_case := (expression | '_') '=>' '{' stmt_body '}'

binary_expr     := expression binary_op expression
binary_op       := '+' | '-' | '*' | '/' | '%' |
                   '==' | '!=' | '>' | '>=' | '<' | '<=' | '&&' | '||'
unary_expr      := unary_op expression
unary_op        := '!' | '-'

lambda_expr     := lambda_params '->' (expression | lambda_block)
lambda_params   := identifier | '(' identifier (',' identifier)* ')'
lambda_block    := '{' var_decl* expression '}'

literal         := float_literal | int_literal | string | boolean | null | array | object
int_literal     := '-'? [0-9]+
float_literal   := '-'? [0-9]+ '.' [0-9]+
array           := '[' [expression (',' expression)* ','?] ']'
object          := '{' [key_value (',' key_value)* ','?] '}'
key_value       := expression ':' expression

arg_list        := positional_args | named_args
positional_args := expression (',' expression)*
named_args      := identifier ':' expression (',' identifier ':' expression)*
```

## Key Points

- **Top-level only:** Map declarations (`map_decl`) and imports (`import_stmt`) can only appear at the top level of a program, not inside statement bodies. Control flow statements (`if_stmt`, `match_stmt`) can be nested
- **Variables:** `$var` for declaration and reference
- **Metadata:** `input@.key` (read), `output@.key` (write)
- **Context-dependent paths:**
  - **Top-level assignments:** Must use `output`, `input`, or `$variable` (no bare identifiers)
  - **Map/lambda bodies:** Can use bare identifiers for parameters **in expressions only** (e.g., `data.field` where `data` is a parameter). Parameters are read-only and cannot be assigned to.
  - **Match with `as`:** Creates a read-only binding in expressions (e.g., `match input.x as val { val.field ... }`)
- **Quoted fields:** Use `."string"` for field names (dot required before quote): `input."field name"`
- **Object literals:** Keys are expressions that **must** evaluate to strings at runtime (error if not): `{"key": value}` or `{$var: value}`. Use `.string()` for explicit type conversion.
- **Indexing:** `[expr]` on objects (string index), arrays (numeric index), strings (codepoint position, returns int32), bytes (byte position, returns int32). Negative indices supported for arrays.
- **Null-safe:** `?.` and `?[` short-circuit to `null`
- **Map calls:** `name(arg)` or `namespace.name(arg)` (positional or named arguments)
- **Named arguments:** `func(a: 1, b: 2)` - cannot mix with positional arguments
- **Lambdas:** Single param `x -> expr`, multi-param `(a, b) -> expr`, block `x -> { ... }`. Lambda parameters are available as bare identifiers within the lambda body
- **Purity:**
  - Expressions cannot assign to `output` or `output@`
  - Lambda blocks: Variable declarations + final expression (pure, no side effects)
  - Map bodies: Same as lambda blocks - pure functions that return values
  - Maps cannot reference `input` or `output` (only their parameter)
- **Control flow forms:**
  - `if_expr` / `match_expr`: Used in assignments, contain `expr_body` (no `output` assignments)
  - `if_stmt` / `match_stmt`: Standalone statements, contain `stmt_body` (may assign to `output`)
  - `expr_body`: Variable declarations + final expression (must be pure)
  - `stmt_body`: One or more statements (no trailing expression)
- **Type coercion:** `+` requires same types (no implicit conversion)
- **Operator precedence:** Field access > unary > multiplicative > additive > comparison > equality > logical AND > logical OR

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
