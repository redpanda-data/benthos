# 16. Type Coercion Rules

- String concatenation: `+` operator converts operands to strings when either operand is string
- Numeric operations: `+`, `-`, `*`, `/`, `%` require numeric operands or result in mapping error
- Boolean operations: `&&`, `||` require boolean operands or result in mapping error
- Comparisons: `==`, `!=` perform type-sensitive equality; `>`, `>=`, `<`, `<=` require comparable types
