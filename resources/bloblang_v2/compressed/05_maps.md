# 5. Maps (User-Defined Functions)

Pure, reusable transformations called as functions.

## 5.1 Syntax

```bloblang
map name(parameter) {
  # optional variable declarations
  # final expression (return value)
  expression
}

# Invocation
output.result = name(input.data)
```

Maps are **pure functions**: they take a parameter, optionally declare variables, and return a value. They cannot reference `input` or `output`.

## 5.2 Examples

**Basic:**
```bloblang
map extract_user(data) {
  {
    "id": data.user_id,
    "name": data.full_name,
    "email": data.email
  }
}

output.customer = extract_user(input.customer_data)
```

**With variables:**
```bloblang
map calculate_total(order) {
  $subtotal = order.items.reduce((acc, item) -> acc + item.price, 0)
  $tax = $subtotal * 0.1
  $subtotal + $tax
}

output.total = calculate_total(input.order)
```

**String transformation:**
```bloblang
map format_name(user) {
  user.first_name + " " + user.last_name
}

output.display_name = format_name(input.user)
```

**Recursion:**
```bloblang
map walk_tree(node) {
  match node.type() as t {
    t == "object" => node.map_each(item -> walk_tree(item.value))
    t == "array" => node.map_each(elem -> walk_tree(elem))
    t == "string" => node.uppercase()
    _ => node
  }
}

output = walk_tree(input)
```

## 5.3 Parameter Semantics

- The parameter is the **only** input to the map (no access to `input` or `output`)
- The parameter is immutable
- Maps are pure: same input always produces same output

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
