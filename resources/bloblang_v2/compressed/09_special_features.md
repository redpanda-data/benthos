# 9. Special Features

## 9.1 Dynamic Field Names

Use string indexing for dynamic field access on objects:

```bloblang
# Dynamic field read
$field_name = "user_id"
output.value = input[$field_name]

# Dynamic field write
$key = "dynamic_field"
output[$key] = "value"

# With literals
output.first = input["user_id"]
output["computed_" + input.type] = input.value

# Null-safe dynamic access
output.value = input?.user?[$field_name]
```

## 9.2 Message Filtering & Deletion

**The `deleted()` function** returns a special deletion marker that instructs assignments to remove the target.

### Deletion Semantics

```bloblang
# Delete output field
output.field = deleted()        # Field marked for deletion

# Delete entire document
output = deleted()              # Document marked for deletion (metadata persists)

# Delete metadata key
output@.key = deleted()         # Specific key removed

# Delete all metadata
output@ = deleted()             # All metadata removed
```

**`output` vs `output@` deletion — different semantics:**

`output` can hold any type (object, array, string, bytes, etc.), so `output = deleted()` removes the document entirely — it is marked for deletion and reads as `null`. Restoring it requires a root reassignment (`output = value`); field assignment through a deleted document is a type error since there is no underlying structure.

`output@` is always an object (a key-value map of metadata). It has no other valid form. `output@ = deleted()` clears all keys from the metadata object but the underlying object remains. Because the object still exists, assigning individual keys after clearing is always valid.

```bloblang
# Document deletion: removes the document, reads as null
output = deleted()
$val = output                   # $val is null
output = "hello"                # OK: root reassignment restores document
output = deleted()
output.field = "value"          # ERROR: cannot assign field on non-object (null)

# Metadata deletion: clears all keys, object still exists
output@ = deleted()             # All metadata keys removed (object remains empty)
output@.key = "value"           # OK: assigning key to the (empty) metadata object
```

**Restoration of deleted output:** Only root reassignment restores a deleted document:
```bloblang
output = deleted()              # Document deleted
output = "hello"                # Document restored with new value

# Same error as assigning through any non-object value:
output = deleted()
output.field = "value"          # ERROR: cannot assign field on non-object (null)
output = "hello"
output.field = "value"          # ERROR: cannot assign field on non-object (string)
```

**Variable deletion:** Assigning `deleted()` to a variable **removes the variable**:
```bloblang
$val = deleted()                # Variable $val is deleted (ceases to exist)
output.field = $val             # ERROR: variable $val does not exist
```

### deleted() as a First-Class Value

**Any expression can yield `deleted()`**, including maps, lambdas, if expressions, and match expressions. When `deleted()` flows through expressions and is assigned or included in a collection, it causes removal:

**In array operations:**
```bloblang
# Array literal - deleted elements omitted
output.items = [1, deleted(), 3]              # Result: [1, 3]

# if-without-else also skips elements (same as deleted)
output.items = [1, if false { 2 }, 3]         # Result: [1, 3]
output.mixed = ["a", if false { "b" } else { deleted() }, "c"]  # Result: ["a", "c"]

# map_array - deleted elements filtered out
output.positive = input.numbers.map_array(x -> if x > 0 { x } else { deleted() })
# Input: [-1, 2, -3, 4] → Output: [2, 4]
```

**In object literals:**
```bloblang
# Using deleted() explicitly
output.user = {
  "id": input.id,
  "email": if input.email_verified { input.email } else { deleted() },
  "phone": input.phone
}
# If email not verified, field "email" is omitted from object

# if-without-else also omits fields (same as deleted)
output.user = {
  "id": input.id,
  "email": if input.email_verified { input.email },  # Omitted if not verified
  "phone": input.phone
}
```

**Nested structures (recursive deletion):**
```bloblang
# Nested arrays
output.matrix = [[1, deleted(), 3], [4, 5]]
# Result: [[1, 3], [4, 5]]

# Deeply nested - deletion propagates through all levels
output.nested = [deleted(), [deleted(), 3]]
# Result: [[3]] (first element deleted, then inner first element deleted)

# Nested objects
output.user = {
  "name": "Alice",
  "contact": {
    "email": if input.verified { input.email } else { deleted() },
    "phone": input.phone
  }
}
# If not verified: {"name": "Alice", "contact": {"phone": "..."}}

# Arrays of objects
output.users = [
  {"name": "Alice", "email": if true { "a@example.com" }},
  {"name": "Bob", "email": if false { "b@example.com" }}  # email omitted
]
# Result: [{"name": "Alice", "email": "a@example.com"}, {"name": "Bob"}]
```

`deleted()` works recursively at all nesting levels - elements are omitted wherever they appear in the structure.

**In conditional expressions:**
```bloblang
# Field assignment skipped when expression yields deleted()
output.category = if input.spam { deleted() } else { input.category }
# If spam, output.category field doesn't exist (not even with null value)

# Message filtering
output = if input.spam { deleted() } else { input }
# If spam, entire document deleted
```

**In maps and lambdas:**
```bloblang
map filter_negative(val) {
  if val < 0 { deleted() } else { val }
}
output.result = filter_negative(input.value)  # Field deleted if value < 0
```

**Operations on `deleted()` are errors:**
```bloblang
deleted() + 5                   # ERROR: cannot perform arithmetic on deleted
deleted() == deleted()          # ERROR: cannot compare deleted values
deleted().type()                # ERROR: cannot call methods on deleted value
```

These operations result in **runtime errors** (or compile-time errors if detectable by implementation).

**When deleted() Causes Errors vs Deletion:**

`deleted()` behaves differently depending on context:

**Triggers deletion (no error):**
- Assignment targets: `output.field = deleted()`, `output = deleted()`, `output@.key = deleted()`
- Collection literals: `[1, deleted(), 3]` → `[1, 3]`, `{"a": deleted()}` → `{}`
- Return values from expressions used in assignments: `output.x = if spam { deleted() } else { value }`
- `map_array` lambda return value: element is filtered out
- `map_object` lambda return value: key-value pair is removed from result

**Causes runtime error:**
- Binary operators: `deleted() + 5`, `deleted() == deleted()`, `deleted() && true`
- Method calls: `deleted().type()`, `deleted().uppercase()`
- Used as function arguments (except assignment): `some_function(deleted())`
- Lambda return values in methods other than `map_array`/`map_object` (e.g., `filter`, `sort`)

The distinction: `deleted()` is a special marker that triggers deletion when flowing into an assignment or collection, but cannot be used as a normal value in computations.

**Metadata persistence during execution:**
```bloblang
# Metadata persists even if output is temporarily deleted
output = deleted()
output@.kafka_topic = "processed"   # Metadata set
output = "hello"                    # Output restored
# At end: both output and metadata exist
```

**Important:** If output is deleted **at the end of execution**, the entire message (document + metadata) is removed from the stream. Metadata assignments are meaningless for deleted messages:

```bloblang
# INCORRECT: These metadata assignments serve no purpose
output = deleted()
output@.reason = "spam_detected"    # Pointless - message will be removed
output@.kafka_topic = "dead_letter" # Pointless - message will be removed
# Result: Entire message deleted, metadata ignored
```

To route failed/spam messages, the output document must exist:
```bloblang
# CORRECT: Route spam to dead letter with document
output = input                       # Keep document (or create error document)
output@.reason = "spam_detected"     # Metadata for routing
output@.kafka_topic = "dead_letter"  # Route to dead letter topic
```

## 9.3 Non-Structured Data

Handle raw strings/bytes:
```bloblang
# If input is raw string
output.parsed = input.parse_json()

# If input is raw bytes
output.decoded = input.string()
```

## 9.4 Conditional Literals

Build dynamic structures:
```bloblang
output.user = {
  "id": input.id,
  "name": input.name,
  "email": if input.email_verified {
    input.email
  } else {
    null
  }
}

# Conditional array elements - if without else skips the element
output.items = [
  input.a,
  if input.b != null { input.b },  # Skipped if b is null
  input.c
]
# If b is null: [input.a, input.c]

# Equivalent using deleted()
output.items = [
  input.a,
  if input.b != null { input.b } else { deleted() },
  input.c
]
```
