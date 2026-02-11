# 8. Imports

Import mappings from external files:
```bloblang
import "./transformations.blobl"
```

**Syntax**: `import "path"`

**Path Resolution**: Absolute paths or relative to execution directory.

**Semantics**: Imports make all maps defined in the target file available in the current mapping. Maps are merged into the global namespace.
