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

path            := ('output' | 'input' | var_ref | meta_ref) ('.' field_access)*
field_access    := identifier | quoted_string | '[' expression ']'
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
