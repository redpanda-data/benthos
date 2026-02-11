# 15. Grammar Summary

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

match_expr      := 'match' expression 'as' identifier '{' match_case (',' match_case)* '}'
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

## Notes

- **Variable declarations** use the `$` prefix: `$variable = expression`
- **Metadata access** uses `@.` accessor after `input` or `output`: `input@.key` or `output@.key`
- Input metadata (`input@.key`) is immutable; output metadata (`output@.key`) is mutable
- The `$` prefix is used for both declaring and referencing variables
- **Map invocation** uses function call syntax: `map_name(argument)` or `namespace.map_name(argument)` for imported maps
- **Path components** can be:
  - Field access: `.identifier` or `."quoted"`
  - Null-safe field access: `?.identifier` or `?."quoted"`
  - Array indexing: `[expression]`
  - Null-safe array indexing: `?[expression]`
- **Indexing** does not require a preceding dot: `input.foo[0]` not `input.foo.[0]`
- **Indexing works on arrays, strings, and bytes**:
  - Arrays: Returns element at position (any type)
  - Strings: Returns single-character string at byte position
  - Bytes: Returns byte value as number (0-255)
- **Negative indices** are supported (Python-style):
  - `path[0]` accesses the first element/character/byte
  - `path[-1]` accesses the last element/character/byte
  - The index expression can be any expression evaluating to an integer
  - Out-of-bounds access throws an error (use `.catch()` for safe access)
- **Null-safe operators** short-circuit to `null` if the left operand is `null`:
  - `input.user?.name` returns `null` if `user` is `null`
  - `input.items?[0]` returns `null` if `items` is `null`
  - Null-safe operators only handle `null`, not errors (use `.catch()` for errors)
- **Path components can be mixed**: `input.users?[0]?.orders[-1]?.total` combines all forms
- **Lambda expressions** are first-class values that support:
  - Single parameter (parentheses optional): `x -> x * 2` or `(x) -> x * 2`
  - Multiple parameters (parentheses required): `(a, b) -> a + b`
  - Single-expression body: `x -> x * 2`
  - Multi-statement block body: `x -> { $temp = x * 2; $temp + 1 }`
  - The last expression in a block is the return value
  - Can be stored in variables and invoked: `$fn = (a, b) -> a + b; $fn(1, 2)`
- **Purity constraints**: Lambda expressions, if expressions, and match expressions cannot contain:
  - Assignments to `output` (e.g., `output.field = value` or `output@.key = value`)
  - These constructs are pure expressions that return values, not statements with side effects
- **Variable immutability**: Variables cannot be reassigned in the same scope, only shadowed in inner scopes
- **Operator precedence**: See Section 4.2 for complete precedence table (field access > unary > multiplicative > additive > comparison > equality > logical AND > logical OR)
- **Type coercion**: The `+` operator requires both operands to be the same type (both strings or both numbers). Mixed types require explicit conversion via `.string()` method (see Section 16)
- **Null-safe operators** work with method calls: `input.text?.uppercase()` returns null if text is null
