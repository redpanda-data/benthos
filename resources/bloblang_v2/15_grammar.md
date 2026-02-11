# 15. Grammar Summary

```
program         := statement*
statement       := assignment | var_decl | map_decl | import_stmt
assignment      := path '=' expression
var_decl        := '$' identifier '=' expression
map_decl        := 'map' identifier '(' identifier ')' '{' statement* '}'
import_stmt     := 'import' string_literal

expression      := literal | path | function_call | method_chain |
                   if_expr | match_expr | binary_expr | unary_expr |
                   lambda_expr | paren_expr

path            := ('output' | 'input' | var_ref | meta_ref) path_component*
path_component  := '.' field_name | '?.' field_name | '[' expression ']' | '?[' expression ']'
field_name      := identifier | quoted_string
var_ref         := '$' identifier
meta_ref        := '@' identifier

function_call   := identifier '(' [arg_list] ')'
method_chain    := expression ('.' identifier '(' [arg_list] ')')+

if_expr         := 'if' expression '{' (expression | statement*) '}'
                   ('else' 'if' expression '{' (expression | statement*) '}')*
                   ('else' '{' (expression | statement*) '}')?

match_expr      := 'match' expression 'as' identifier '{' match_case (',' match_case)* '}'
                 | 'match' '{' match_case (',' match_case)* '}'
match_case      := (expression | '_') '=>' (expression | '{' statement* '}')

binary_expr     := expression binary_op expression
binary_op       := '+' | '-' | '*' | '/' | '%' |
                   '==' | '!=' | '>' | '>=' | '<' | '<=' | '&&' | '||'
unary_expr      := unary_op expression
unary_op        := '!' | '-'

lambda_expr     := identifier '->' expression

literal         := number | string | boolean | null | array | object
array           := '[' [expression (',' expression)*] ']'
object          := '{' [key_value (',' key_value)*] '}'
key_value       := (identifier | string) ':' expression

arg_list        := positional_args | named_args
positional_args := expression (',' expression)*
named_args      := identifier ':' expression (',' identifier ':' expression)*
```

## Notes

- **Variable declarations** use the `$` prefix: `$variable = expression`
- **Metadata assignments** use the same `assignment` production with `meta_ref` on the left side
- The `@` prefix is used for both reading and writing metadata
- The `$` prefix is used for both declaring and referencing variables
- Both metadata and variables use consistent prefix notation for declaration and reference
- **Path components** can be:
  - Field access: `.identifier` or `."quoted"`
  - Null-safe field access: `?.identifier` or `?."quoted"`
  - Array indexing: `[expression]`
  - Null-safe array indexing: `?[expression]`
- **Array indexing** does not require a preceding dot: `input.foo[0]` not `input.foo.[0]`
- **Negative indices** are supported (Python-style):
  - `path[0]` accesses the first element
  - `path[-1]` accesses the last element
  - The index expression can be any expression evaluating to an integer
  - Out-of-bounds access throws an error (use `.catch()` for safe access)
- **Null-safe operators** short-circuit to `null` if the left operand is `null`:
  - `input.user?.name` returns `null` if `user` is `null`
  - `input.items?[0]` returns `null` if `items` is `null`
  - Null-safe operators only handle `null`, not errors (use `.catch()` for errors)
- **Path components can be mixed**: `input.users?[0]?.orders[-1]?.total` combines all forms
