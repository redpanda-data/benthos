# 5. Maps (User-Defined Functions)

Isolated, reusable transformations called as functions.

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

# Default parameters (must come after required parameters)
map format_price(amount, currency = "USD", decimals = 2) {
  currency + " " + amount.round(decimals).string()
}

# Discard parameters (match a call signature, ignore unused args)
map handle_event(_, _, payload) {
  payload.uppercase()
}

# Invocation
output.headers = default_headers()
output.result = name(input.data)
output.calc = calculate(1, 2, 3)

# Invocation - named arguments (for maps with parameters)
output.result = name(parameter: input.data)
output.calc = calculate(x: 1, y: 2, z: 3)

# Using defaults — positional (trailing optional args omitted)
output.price = format_price(99.99)                # "USD 99.99"
output.price = format_price(99.99, "EUR")         # "EUR 99.99"
output.price = format_price(99.99, "EUR", 0)      # "EUR 100"

# Using defaults — named (missing optional args use defaults)
output.price = format_price(amount: 99.99)                          # "USD 99.99"
output.price = format_price(amount: 99.99, decimals: 0)             # "USD 100"
output.price = format_price(amount: 99.99, currency: "EUR")         # "EUR 99.99"
```

Maps are **isolated functions**: they take zero or more parameters, optionally declare variables, and return a value. They cannot reference `input` or `output`.

**Argument styles:** Functions can be called with positional or named arguments, but not both in the same call.

**Default parameters:** Parameters may have default values (`param = literal`). Parameters with defaults must come after all required parameters. Default values must be literals (`42`, `"hello"`, `true`, `false`, `null`) — expressions, function calls, and references to other parameters are not allowed in defaults. **Note:** Since there is no timestamp literal syntax, timestamp defaults are not possible. Use `null` with `.or()` as a workaround: `map query(start = null) { $s = start.or(now()); ... }`.

**Dynamic defaults pattern:** When a parameter's default depends on other parameters or on computed values, use `null` as a sentinel default and compute the actual value in the map body. This is the standard pattern for truly optional parameters in user-defined maps:

```bloblang
map connect(host, port = null) {
  $p = port.or(if host.has_prefix("https") { 443 } else { 80 })
  host + ":" + $p.string()
}

connect("https://example.com")       # "https://example.com:443"
connect("http://example.com")        # "http://example.com:80"
connect("http://example.com", 8080)  # "http://example.com:8080"
```

This pattern cannot distinguish "caller passed null explicitly" from "caller omitted the argument." If null is a meaningful value for a parameter, use a different sentinel or restructure the map.

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
  match node.type() {
    "object" => node.map_values(v -> walk_tree(v)),
    "array" => node.map(elem -> walk_tree(elem)),
    "string" => node.uppercase(),
    _ => node,
  }
}

output = walk_tree(input)
```

**Recursion limits:** Maximum recursion depth is implementation-defined. Implementations **must** support at least 1000 levels of recursion depth to ensure basic portability. Exceeding the recursion limit throws a runtime error that stops execution immediately and **cannot be caught** with `.catch()`. Mutual recursion (map A calls map B which calls map A) is valid — maps are hoisted (Section 7.7) — and shares the same depth limit.

## 5.3 Parameter Semantics

- **Maps are isolated** — they can only access their parameters and variables declared within the map body. They cannot access `input`, `output`, or top-level `$variables`. The result is determined entirely by the parameter values.
- Parameters are **read-only** — they cannot be reassigned or used as assignment targets
- Parameters are available as bare identifiers within the map body (e.g., `data.field`)
- Variables declared within maps (using `$`) can be reassigned
- **Discard parameters (`_`):** `_` can be used as a parameter name to accept and ignore an argument. It is not bound — referencing `_` in the body is a compile error. Multiple `_` parameters are allowed in the same parameter list. Discard parameters cannot have defaults.
- Call with positional arguments (match order) or named arguments (match names)
- **Cannot mix** positional and named arguments in the same call
- **`_` restricts to positional calls:** Maps with any `_` parameters can only be called positionally. Named calls to such maps are a compile error since `_` has no name to target.
- **Arity:** Positional calls must provide at least the required parameter count and at most the total parameter count. Named calls must provide all required parameters; missing parameters with defaults use their defaults. Extra or unknown arguments are errors. Arity mismatches are compile-time errors when detectable, runtime errors otherwise.
- **Parameter shadowing:** Parameter names shadow any map names with the same name within the map body. The parameter always wins. Imported namespaces are not affected since they use `::` syntax — `namespace::func()` is always unambiguous regardless of parameter names.

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

# Parameter and namespace coexist — :: makes namespace calls unambiguous
import "./math.blobl" as math
map transform(math) {
  math::add(math, 2)     # math:: is the namespace call, math is the parameter
}
```

## 5.4 Scope Restrictions

To use external context inside a map, pass it as a parameter:

```bloblang
# ❌ Cannot access input inside a map
map invalid(items) {
  items.map(x -> x * input.multiplier)   # ERROR: cannot access input
}

# ✅ Pass external context as a parameter instead
map scale(items, multiplier) {
  items.map(x -> x * multiplier)
}
output.result = scale(input.items, input.multiplier)
```

## 5.5 Maps as Method Arguments

Map names, namespace-qualified references, and standard library function names can be passed directly to higher-order methods like `.map()`, `.filter()`, and `.sort_by()`. The compiler resolves the name to the definition at compile time — this is syntactic sugar for an inline lambda that calls the map/function, not a runtime value.

```bloblang
map double(x) { x * 2 }

# Pass map directly to higher-order methods
output.doubled = input.items.map(double)          # Same as: map(x -> double(x))

# Namespace-qualified references also work
import "./math.blobl" as math
output.results = input.items.map(math::double)    # Same as: map(x -> math::double(x))
```

These names are **compile-time references**, not runtime values. They cannot be stored in variables or used as general-purpose expressions:
```bloblang
$fn = double              # ERROR: cannot store a map reference in a variable
$fn = math::double        # ERROR: cannot store a namespace reference in a variable
output.x = double         # ERROR: bare map name is not a valid expression here
```

**Void from map bodies:** If a map body's final expression is an if-without-else or match-without-`_`, the map can produce void when the condition is false or no case matches. Void from a map call follows the same propagation rules as void from any other expression (Section 4.1) — it will be a runtime error in most calling contexts (variable declarations, collection literals, function arguments, etc.). To avoid this, always include an `else` branch or `_` case in a map body's final expression.
