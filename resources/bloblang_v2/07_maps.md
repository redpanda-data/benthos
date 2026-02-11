# 7. Maps (Named Mappings)

Reusable transformation definitions that receive an explicit parameter:
```bloblang
map extract_user(data) {
  output.id = data.user_id
  output.name = data.full_name
  output.email = data.contact.email
}

output.customer = extract_user(input.customer_data)
```

**Syntax**: `map name(parameter) { statements }`

**Parameter**: Maps declare an explicit parameter name that replaces `input` within the map body.

**Invocation**: Function call syntax: `map_name(argument)`

**Multiple Parameters**: Pass multiple values via object literal:
```bloblang
map format_output(data) {
  output.value = data.value
  output.pattern = data.pattern
}

output.result = format_output({"value": input.a, "pattern": "[%v]"})
```

**Recursion**: Maps may recursively invoke themselves:
```bloblang
map walk_tree(node) {
  output = match node.type() as type {
    type == "object" => node.map_each(item -> walk_tree(item.value))
    type == "array" => node.map_each(elem -> walk_tree(elem))
    _ => node
  }
}

output = walk_tree(input)
```
