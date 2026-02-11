# 8. Imports

Import mappings from external files using namespace syntax:

```bloblang
import "path" as namespace_name
```

## 8.1 Basic Import Syntax

**Syntax**: `import "path" as identifier`

All maps in the imported file become available under the specified namespace:

```bloblang
# user_transforms.blobl
map extract_user(data) {
  output.id = data.user_id
  output.name = data.full_name
  output.email = data.email
}

map format_name(data) {
  output = data.first_name + " " + data.last_name
}

# main.blobl
import "./user_transforms.blobl" as users

output.user = users.extract_user(input.user_data)
output.display_name = users.format_name(input.user)
```

## 8.2 Path Resolution

**Relative Paths**: Resolved relative to the importing file's directory
```bloblang
import "./sibling.blobl" as sibling          # Same directory
import "./subdir/file.blobl" as sub          # Subdirectory
import "../parent/file.blobl" as parent      # Parent directory
```

**Absolute Paths**: Used as-is
```bloblang
import "/etc/benthos/mappings/common.blobl" as common
```

## 8.3 Namespace Usage

Maps are invoked using qualified names:
```bloblang
import "./users.blobl" as users
import "./orders.blobl" as orders

# Both define a 'transform' map - no collision
output.user = users.transform(input.user_data)
output.order = orders.transform(input.order_data)
```

## 8.4 Exported Maps

**All top-level maps are automatically exported**. No export keyword needed.

To keep helpers private, use local variables instead of maps:
```bloblang
# Private helper (not visible to importers)
$validate_email = email -> email.contains("@")

# Public map (visible to importers)
map process_user(data) {
  output.email = if $validate_email(data.email) {
    data.email
  } else {
    null
  }
}
```

## 8.5 Error Handling

**File Not Found**:
```bloblang
import "./nonexistent.blobl" as utils
# Error: Import failed: file not found: ./nonexistent.blobl
```

**Duplicate Namespace**:
```bloblang
import "./utils1.blobl" as utils
import "./utils2.blobl" as utils
# Error: Namespace 'utils' already defined
```

**Circular Imports**:
```bloblang
# a.blobl imports b.blobl
# b.blobl imports a.blobl
# Error: Circular import detected: a.blobl -> b.blobl -> a.blobl
```

**Map Not Found**:
```bloblang
import "./utils.blobl" as utils
output.result = utils.nonexistent(input.data)
# Error: Map 'nonexistent' not found in namespace 'utils'
```

## 8.6 Recursion Within Maps

Maps can call themselves recursively without namespace prefix:
```bloblang
# tree_walker.blobl
map walk_tree(node) {
  output = match node.type() as t {
    t == "object" => node.map_each(item -> walk_tree(item.value))
    t == "array" => node.map_each(elem -> walk_tree(elem))
    t == "string" => node.uppercase()
    _ => node
  }
}

# main.blobl
import "./tree_walker.blobl" as tree
output.transformed = tree.walk_tree(input.data)
```

## 8.7 Benefits

**Clear Provenance**: Always obvious where a map comes from
```bloblang
output.user = users.extract_user(input.data)  # From users namespace
```

**No Name Collisions**: Namespaces prevent conflicts
```bloblang
users.transform(...)    # Different from
orders.transform(...)   # this transform
```

**Better Organization**: Group related maps by file and namespace
```bloblang
import "./validators.blobl" as validators
import "./formatters.blobl" as formatters
import "./enrichers.blobl" as enrichers
```
