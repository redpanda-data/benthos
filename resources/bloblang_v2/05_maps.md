# 5. Maps (User-Defined Functions)

Pure, reusable transformations called as functions.

## 5.1 Syntax

```bloblang
# Zero parameters (useful for common structures/macros)
map default_headers() {
  {"content_type": "application/json", "version": "2.0"}
}

# Single parameter
map name(parameter) {
  # optional variable declarations
  # final expression (return value)
  expression
}

# Multiple parameters
map calculate(x, y, z) {
  x + y * z
}

# Invocation
output.headers = default_headers()
output.result = name(input.data)
output.calc = calculate(1, 2, 3)

# Invocation - named arguments (for maps with parameters)
output.result = name(parameter: input.data)
output.calc = calculate(x: 1, y: 2, z: 3)
```

Maps are **pure functions**: they take zero or more parameters, optionally declare variables, and return a value. They cannot reference `input` or `output`.

**Argument styles:** Functions can be called with positional or named arguments, but not both in the same call.

## 5.2 Examples

**Single parameter:**
```bloblang
map extract_user(data) {
  {
    "id": data.user_id,
    "name": data.full_name,
    "email": data.email
  }
}

output.customer = extract_user(input.customer_data)
output.customer = extract_user(data: input.customer_data)  # Named
```

**Multiple parameters:**
```bloblang
map format_price(amount, currency, decimals) {
  currency + " " + amount.round(decimals).string()
}

# Positional
output.price = format_price(99.99, "USD", 2)

# Named
output.price = format_price(amount: 99.99, currency: "USD", decimals: 2)
```

**With variables:**
```bloblang
map calculate_total(subtotal, tax_rate) {
  $tax = subtotal * tax_rate
  subtotal + $tax
}

output.total = calculate_total(100, 0.1)
output.total = calculate_total(subtotal: 100, tax_rate: 0.1)
```

**Recursion:**
```bloblang
map walk_tree(node) {
  match node.type() as t {
    t == "object" => node.map_object((key, value) -> walk_tree(value)),
    t == "array" => node.map_array(elem -> walk_tree(elem)),
    t == "string" => node.uppercase(),
    _ => node,
  }
}

output = walk_tree(input)
```

**Recursion limits:** Maximum recursion depth is implementation-defined. Implementations **must** support at least 1000 recursive calls to ensure basic portability. Exceeding the recursion limit throws a runtime error that stops execution immediately and **cannot be caught** with `.catch()`.

## 5.3 Parameter Semantics

- **Maps are fully isolated** — they can only access their parameters and variables declared within the map body. They cannot access `input`, `output`, or top-level `$variables`.
- Parameters are **read-only** - they cannot be reassigned or used as assignment targets
- Parameters are available as bare identifiers within the map body (e.g., `data.field`)
- Variables declared within maps (using `$`) can be reassigned
- Maps are pure: same inputs always produce same output
- Call with positional arguments (match order) or named arguments (match names)
- **Cannot mix** positional and named arguments in the same call
- **Parameter names must not conflict with imported namespaces** — if a parameter has the same name as an imported namespace, it is a compile-time error. Rename the parameter or the namespace alias to resolve the conflict.

```bloblang
map example(data) {
  $copy = data            # ✅ Valid: variable declaration
  data.field              # ✅ Valid: read from parameter (final expression)
}

map invalid(data) {
  data = input.x          # ❌ Invalid: cannot assign to parameter
}

map also_invalid(data) {
  $val = $top_level_var   # ❌ Invalid: cannot access top-level variables
  data.field
}

# Assuming: import "./math.blobl" as math
map also_invalid(math) {  # ❌ Compile error: parameter 'math' conflicts with namespace 'math'
  math.add(1, 2)
}
```

## 5.4 Purity Constraints

Maps are pure functions with no side effects:

```bloblang
map transform(data) {
  data.value * 2           # ✅ Valid: pure transformation
}

map invalid(data) {
  output.x = data.value    # ❌ Invalid: cannot reference output
  data.value               # ❌ Invalid: cannot assign to output
}
```

**Why pure?**
- Predictable: Same input always gives same result
- Composable: Can be used anywhere, including in lambdas
- Testable: Easy to test in isolation
