# 12. Context and Scoping

## 12.1 Input Context

`input` refers to the input document being processed. Available throughout the entire mapping:
```bloblang
output.field = input.field
output.nested = input.user.profile.email
```

## 12.2 Output Context

`output` refers to the output document being constructed. Accessible throughout mapping:
```bloblang
output.field = value
output = input
```

## 12.3 Explicit Context in Lambdas and Match

**Core Principle**: All data contexts must be explicitly named. There is no implicit context variable.

**Lambda Parameters**:
```bloblang
# Filter with explicit parameter
input.items.filter(item -> item.score > 50)

# Map with explicit parameter
input.items.map_each(user -> {
  "id": user.id,
  "name": user.name.uppercase()
})

# Nested lambdas - each must name its parameter
input.users.map_each(user ->
  user.orders.map_each(order -> order.total)
)
```

**Match Expression Binding**:
```bloblang
# Match requires explicit 'as' binding
match input.status as status {
  status == "active" => "online"
  status == "inactive" => "offline"
  _ => "unknown"
}

# Can reference input directly in boolean match
match {
  input.score > 80 => "high"
  input.score > 50 => "medium"
  _ => "low"
}
```

**Parenthesized Context**:
```bloblang
# Explicitly name the context with lambda
input.foo.(x -> x.bar + x.baz)

# Can nest with different names
input.outer.(o -> o.inner.(i -> i.value))
```

## 12.4 Variable Scope

Variables declared with `$` are lexically scoped to the block in which they are declared.

**Top-Level Variables**: Available throughout the entire mapping.
```bloblang
$user_id = input.user.id
$name = input.user.name

output.id = $user_id
output.name = $name
```

**Block-Level Variables**: Available only within the declaring block and nested blocks.
```bloblang
# Conditional scope
output.value = if input.type == "special" {
  $multiplier = 2.5  # Only available in this if block
  input.amount * $multiplier
} else {
  input.amount  # $multiplier not accessible here
}

# Match expression scope
output.result = match input.category as category {
  category == "premium" => {
    $discount = 0.20  # Scoped to this branch
    input.price * (1 - $discount)
  }
  category == "standard" => {
    $discount = 0.10  # Different variable, same name
    input.price * (1 - $discount)
  }
  _ => input.price
}

# Nested scopes
$global = "outer"
output.nested = if input.flag {
  $scoped = "middle"
  if input.nested_flag {
    $inner = "innermost"
    $global + " " + $scoped + " " + $inner  # All three accessible
  } else {
    $global + " " + $scoped  # $inner not accessible here
  }
}
```

**Shadowing**: Inner scopes may declare variables with the same name as outer scopes. The innermost declaration shadows outer ones:
```bloblang
$value = "outer"
output.result = if input.override {
  $value = "inner"  # Shadows outer $value
  $value  # Returns "inner"
} else {
  $value  # Returns "outer"
}
```

**Lifetime**: Variables exist only for the duration of their scope's execution.
