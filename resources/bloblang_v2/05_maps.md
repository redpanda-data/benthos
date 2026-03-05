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

**Default parameters:** Parameters may have default values (`param = literal`). Parameters with defaults must come after all required parameters. Default values must be literals (`42`, `"hello"`, `true`, `false`, `null`) — expressions, function calls, and references to other parameters are not allowed in defaults.

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
    t == "object" => node.iter_kv().map(e -> {"k": e.k, "v": walk_tree(e.v)}).collect_kv(),
    t == "array" => node.map(elem -> walk_tree(elem)),
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
- Maps are isolated: the map body has no access to external state (`input`, `output`, top-level variables). The result is determined by the parameter values, but note that closures passed as arguments may carry captured mutable state (Section 3.4), so the same lambda value can produce different results across calls
- Call with positional arguments (match order) or named arguments (match names)
- **Cannot mix** positional and named arguments in the same call
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

## 5.4 Isolation Constraints

Maps are isolated functions — they cannot access `input`, `output`, or top-level variables:

```bloblang
map transform(data) {
  data.value * 2           # ✅ Valid: isolated transformation
}

map invalid(data) {
  output.x = data.value    # ❌ Invalid: cannot reference output
  data.value               # ❌ Invalid: cannot assign to output
}
```

**Why isolated?**
- Predictable: No hidden dependencies on `input`, `output`, or global state
- Composable: Can be used anywhere, including in lambdas
- No hidden dependencies: Cannot read or modify global state

**Closure caveat:** If a closure (lambda that captures variables by reference) is passed as an argument, the closure carries external mutable state. The map itself remains isolated — it has no direct access to the captured variables — but the same lambda variable can produce different results if its captured state changes between calls:
```bloblang
$multiplier = 2
$fn = x -> x * $multiplier

map apply(data, callback) { callback(data) }

output.a = apply(5, $fn)  # 10
$multiplier = 3
output.b = apply(5, $fn)  # 15 — $fn's captured $multiplier changed
```
- Testable: Easy to test in isolation

## 5.5 Maps as Values

Maps are **first-class values** — a map name used without parentheses evaluates to a lambda value. This allows maps to be passed as arguments, stored in variables, and used anywhere a lambda is expected.

```bloblang
map double(x) { x * 2 }

# Pass map directly to higher-order methods
output.doubled = input.items.map(double)          # Same as: map(x -> double(x))

# Store map in a variable
$fn = double
output.result = $fn(21)                                 # 42

# Namespace-qualified references also work as values
import "./math.blobl" as math
output.results = input.items.map(math::double)    # Same as: map(x -> math::double(x))
$fn = math::double
output.result = $fn(21)                                 # 42
```

**Type:** Map references evaluate to `lambda`. Their `.type()` returns `"lambda"`.

**Note:** Map name references are resolved at compile time and produce lambda values directly — they are not "returned from" a call. This does not violate the lambda return restriction (Section 2.1), which applies to the *result of calling* a map, function, or lambda at runtime.

**Cannot return lambdas:** Maps (and lambdas) cannot return lambda values — if a map body produces a lambda as its result, this is a runtime error. Lambdas are for parameterizing operations, not for building higher-order call chains. See Section 2.1 for full lambda restrictions.

**Parameter info preserved:** Named arguments, defaults, and arity are preserved when a map is used as a value:
```bloblang
map greet(name, greeting = "Hello") { greeting + ", " + name }

$fn = greet
output.a = $fn("Alice")                   # "Hello, Alice"
output.b = $fn(name: "Alice")             # "Hello, Alice"
output.c = $fn("Alice", "Hi")             # "Hi, Alice"
```

**Shadowing:** Parameter names still shadow map names within map and lambda bodies (Section 5.3). A bare identifier always resolves to the innermost binding — parameter first, then map name:
```bloblang
map double(x) { x * 2 }
input.items.map(double -> double + 1)   # 'double' is the parameter, not the map
```
