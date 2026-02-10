# 7. Maps (Named Mappings)

Reusable transformation definitions that receive an explicit parameter:
```bloblang
map extract_user(data) {
  output.id = data.user_id
  output.name = data.full_name
  output.email = data.contact.email
}

output.customer = input.customer_data.apply("extract_user")
```

**Syntax**: `map name(parameter) { statements }`

**Parameter**: Maps declare an explicit parameter name that replaces `input` within the map body.

**Invocation**: `.apply("map_name")` method

**Multiple Parameters**: Pass multiple values via object literal:
```bloblang
map format_output(data) {
  output.value = data.value
  output.pattern = data.pattern
}

{"value": input.a, "pattern": "[%v]"}.apply("format_output")
```

**Recursion**: Maps may recursively invoke themselves:
```bloblang
map walk_tree(node) {
  output = match node.type() as type {
    type == "object" => node.map_each(item -> item.value.apply("walk_tree"))
    type == "array" => node.map_each(elem -> elem.apply("walk_tree"))
    _ => node
  }
}

output = input.apply("walk_tree")
```
