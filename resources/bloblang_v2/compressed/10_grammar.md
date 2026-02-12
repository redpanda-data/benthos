# 10. Grammar Reference

```
program         := statement*
statement       := assignment | var_decl | map_decl | import_stmt
assignment      := path '=' expression
var_decl        := '$' identifier '=' expression
map_decl        := 'map' identifier '(' identifier ')' '{' statement* '}'
import_stmt     := 'import' string_literal 'as' identifier

expression      := literal | path | function_call | method_chain |
                   if_expr | match_expr | binary_expr | unary_expr |
                   lambda_expr | paren_expr

path            := context_root path_component*
context_root    := ('output' | 'input') metadata_accessor? | var_ref
metadata_accessor := '@.'
path_component  := '.' field_name | '?.' field_name | '[' expression ']' | '?[' expression ']'
field_name      := identifier | quoted_string
var_ref         := '$' identifier

function_call   := (identifier | var_ref | qualified_name) '(' [arg_list] ')'
qualified_name  := identifier '.' identifier
method_chain    := expression ('.' identifier '(' [arg_list] ')')+

if_expr         := 'if' expression '{' (expression | statement*) '}'
                   ('else' 'if' expression '{' (expression | statement*) '}')*
                   ('else' '{' (expression | statement*) '}')?

match_expr      := 'match' expression ('as' identifier)? '{' match_case (',' match_case)* '}'
                 | 'match' '{' match_case (',' match_case)* '}'
match_case      := (expression | '_') '=>' (expression | '{' statement* '}')

binary_expr     := expression binary_op expression
binary_op       := '+' | '-' | '*' | '/' | '%' |
                   '==' | '!=' | '>' | '>=' | '<' | '<=' | '&&' | '||'
unary_expr      := unary_op expression
unary_op        := '!' | '-'

lambda_expr     := lambda_params '->' (expression | lambda_block)
lambda_params   := identifier | '(' identifier (',' identifier)* ')'
lambda_block    := '{' statement* expression '}'

literal         := number | string | boolean | null | array | object
array           := '[' [expression (',' expression)*] ']'
object          := '{' [key_value (',' key_value)*] '}'
key_value       := (identifier | string) ':' expression

arg_list        := positional_args | named_args
positional_args := expression (',' expression)*
named_args      := identifier ':' expression (',' identifier ':' expression)*
```

## Key Points

- **Variables:** `$var` for declaration and reference
- **Metadata:** `input@.key` (read), `output@.key` (write)
- **Indexing:** `[expr]` on arrays, strings, bytes. Negative indices supported.
- **Null-safe:** `?.` and `?[` short-circuit to `null`
- **Map calls:** `name(arg)` or `namespace.name(arg)`
- **Lambdas:** Single param `x -> expr`, multi-param `(a, b) -> expr`, block `x -> { ... }`
- **Purity:** Expressions cannot assign to `output` or `output@`
- **Type coercion:** `+` requires same types (no implicit conversion)
- **Operator precedence:** Field access > unary > multiplicative > additive > comparison > equality > logical AND > logical OR
