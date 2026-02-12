# 5. Maps (User-Defined Functions)

Reusable transformations called as functions.

## 5.1 Syntax

```bloblang
map name(parameter) {
  # statements
  output.field = parameter.value
}

# Invocation
output.result = name(input.data)
```

## 5.2 Examples

**Basic:**
```bloblang
map extract_user(data) {
  output.id = data.user_id
  output.name = data.full_name
  output.email = data.email
}

output.customer = extract_user(input.customer_data)
```

**Multiple values via object:**
```bloblang
map format_output(data) {
  output.value = data.value
  output.pattern = data.pattern
}

output.result = format_output({
  "value": input.a,
  "pattern": "[%v]"
})
```

**Recursion:**
```bloblang
map walk_tree(node) {
  output = match node.type() as t {
    t == "object" => node.map_each(item -> walk_tree(item.value))
    t == "array" => node.map_each(elem -> walk_tree(elem))
    _ => node
  }
}

output = walk_tree(input)
```

## 5.3 Parameter Semantics

The parameter name replaces `input` within the map body. The parameter is immutable.
