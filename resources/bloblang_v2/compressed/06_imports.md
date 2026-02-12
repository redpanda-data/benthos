# 6. Imports & Modules

## 6.1 Namespace Imports

```bloblang
import "path" as namespace
```

All maps from the file available via namespace.

## 6.2 Example

```bloblang
# user_transforms.blobl
map extract_user(data) {
  {
    "id": data.user_id,
    "name": data.full_name
  }
}

map format_name(data) {
  data.first_name + " " + data.last_name
}

# main.blobl
import "./user_transforms.blobl" as users

output.user = users.extract_user(input.user_data)
output.display_name = users.format_name(input.user)
```

## 6.3 Path Resolution

**Relative paths:** Relative to importing file's directory
```bloblang
import "./sibling.blobl" as sibling
import "../parent/file.blobl" as parent
```

**Absolute paths:** Used as-is
```bloblang
import "/etc/benthos/common.blobl" as common
```

## 6.4 Visibility

**All top-level maps are exported automatically.**

For private helpers, use variables (not maps):
```bloblang
$private_helper = data -> data.value * 2  # Not exported

map public_api(data) {                     # Exported
  output = $private_helper(data)
}
```

## 6.5 Error Handling

- **File not found:** Error at import
- **Duplicate namespace:** Error if same name used twice
- **Circular imports:** Detected and error
- **Map not found:** Error when calling non-existent map

## 6.6 Recursion

Maps can call themselves without namespace prefix:
```bloblang
map walk(node) {
  match node.type() as t {
    t == "object" => node.map_each(item -> walk(item.value))
    _ => node
  }
}
```
