# 16. Type Coercion Rules

Bloblang V2 emphasizes **explicit type conversion** - type mismatches result in errors rather than silent coercion.

## Addition Operator (`+`)

The `+` operator behavior depends strictly on operand types:

**String Concatenation**:
- If **both operands are strings**, concatenates them
- Examples:
  ```bloblang
  output.result = "Hello " + "World"   # "Hello World"
  output.result = "Count: " + "5"      # "Count: 5"
  ```

**Numeric Addition**:
- If **both operands are numbers**, performs numeric addition
- Examples:
  ```bloblang
  output.result = 5 + 3                # 8
  output.result = 10.5 + 2.3           # 12.8
  ```

**Type Errors** (Explicit Conversion Required):
- Mixing strings and numbers requires **explicit conversion** using `.string()` method
- Examples:
  ```bloblang
  # ❌ ERROR: Type mismatch
  output.result = 5 + "3"              # Error: cannot add number and string
  output.result = "Count: " + 5        # Error: cannot add string and number

  # ✅ CORRECT: Explicit conversion
  output.result = 5.string() + "3"     # "53" (explicit string conversion)
  output.result = "Count: " + 5.string() # "Count: 5"
  output.result = "Total: " + input.amount.string()
  ```

- Other type mismatches also result in errors:
  ```bloblang
  output.result = [1, 2] + 3           # Error: cannot add array and number
  output.result = {"a": 1} + 5         # Error: cannot add object and number
  output.result = true + 10            # Error: cannot add boolean and number
  ```

## Other Arithmetic Operators

- **Numeric operations** (`-`, `*`, `/`, `%`): Require numeric operands or result in mapping error
  ```bloblang
  output.result = 10 - 3               # 7
  output.result = "10" - 3             # Error: subtraction requires numbers
  ```

## Boolean Operators

- **Boolean operations** (`&&`, `||`): Require boolean operands or result in mapping error
  ```bloblang
  output.result = true && false        # false
  output.result = "yes" && true        # Error: requires boolean operands
  ```

## Comparison Operators

- **Equality** (`==`, `!=`): Perform type-sensitive equality (no coercion)
  ```bloblang
  output.result = 5 == "5"             # false (different types)
  output.result = 5 == 5               # true
  ```

- **Ordering** (`>`, `>=`, `<`, `<=`): Require comparable types (both numbers or both strings)
  ```bloblang
  output.result = 10 > 5               # true
  output.result = "abc" < "xyz"        # true (lexicographic)
  output.result = 10 > "5"             # Error: cannot compare number to string
  ```
