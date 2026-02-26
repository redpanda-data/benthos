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

output.user = users::extract_user(input.user_data)
output.display_name = users::format_name(input.user)
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

## 6.4 Visibility & File Constraints

**All top-level maps are exported automatically.** Maps are accessible through the namespace.

**Imported files may only contain map declarations and import statements.** Top-level statements (assignments, variable declarations, if/match statements) are a compile-time error in imported files. Since maps are fully isolated and cannot access top-level variables, `input`, or `output` (Section 5.3), there is no useful purpose for top-level statements in library files.

```bloblang
# utils.blobl — valid imported file
import "./helpers.blobl" as helpers         # ✅ Imports allowed
map transform(data) { data.value * 2 }     # ✅ Map declarations allowed

# invalid_utils.blobl — would fail when imported
$internal = 42                              # ❌ Compile error: statement in imported file
output.side_effect = "hello"                # ❌ Compile error: statement in imported file
map transform(data) { data.value * 2 }

# main.blobl
import "./utils.blobl" as utils
output.result = utils::transform(input)     # ✅ Works: maps are exported
```

## 6.5 Error Handling

- **File not found:** Error at import
- **Duplicate namespace:** Error if same name used twice
- **Circular imports:** Detected at compile time and error
- **Statements in imported file:** Compile-time error if an imported file contains top-level statements
- **Map not found:** Error when calling non-existent map

**Circular import detection:** Import cycles are not allowed. If file A imports B (directly or transitively through other files), then B cannot import A.

```bloblang
# a.blobl
import "./b.blobl" as b
map foo(x) { b::bar(x) }

# b.blobl
import "./a.blobl" as a  # ERROR: Circular import (A->B->A)
map bar(x) { a::foo(x) }
```

This restriction prevents mutual recursion across files. Implementations must detect cycles at compile time before execution.

## 6.6 Recursion

Maps can call themselves without namespace prefix:
```bloblang
map walk(node) {
  match node.type() as t {
    t == "object" => node.map_object((key, value) -> walk(value)),
    _ => node,
  }
}
```
