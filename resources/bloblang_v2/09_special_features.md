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
output.field = deleted()        # Field removed from output

# Drop entire message (immediately exits the mapping)
output = deleted()              # Message dropped, no further statements execute

# Delete metadata key
output@.key = deleted()         # Specific key removed

# Clear all metadata (replace with empty object)
output@ = {}                    # All metadata keys removed

# Replace all metadata
output@ = {"key": "value"}     # Replaces all metadata with this object

# Cannot delete metadata — it is always an object
output@ = deleted()             # ERROR: cannot delete metadata object
```

**`output = deleted()` — immediate message drop:**

`output = deleted()` drops the entire message (document + metadata) from the stream and **immediately exits the mapping**. No subsequent statements execute. This is a terminal operation — there is no way to "restore" a deleted output. It is specifically the *assignment* of `deleted()` to `output` (the root document) that triggers the exit — merely evaluating `deleted()` in an expression does not exit the mapping.

```bloblang
output = deleted()
output.field = "value"          # Never executes — mapping already exited
output@.kafka_topic = "topic"   # Never executes — mapping already exited
```

To conditionally drop messages, use an if expression or match:
```bloblang
# Conditional drop — if spam, message is dropped and mapping exits
output = if input.spam { deleted() } else { input }

# Using match
output = match input.type {
  "spam" => deleted(),
  _ => input,
}
```

**`output@` cannot be deleted:**

`output@` is always an object (a key-value map of metadata). It cannot be deleted — `output@ = deleted()` is an error. To clear all metadata keys, assign an empty object: `output@ = {}`. You can also replace all metadata with an object literal or copy from input: `output@ = input@`.

```bloblang
# Metadata: cannot be deleted, only cleared or replaced
output@ = deleted()             # ERROR: cannot delete metadata object
output@ = {}                    # OK: clears all metadata keys
output@.key = "value"           # OK: assigning key to the metadata object
```

**Variable assignment:** Assigning `deleted()` to a variable is a runtime error — variables cannot be deleted.
```bloblang
$val = deleted()                # ERROR: cannot assign deleted() to a variable
```

### deleted() in Expressions

**Any expression can yield `deleted()`**, including maps, lambdas, if expressions, and match expressions. When `deleted()` flows through expressions and is assigned to a field or included in a collection, it causes removal. When it flows to a root output assignment (`output = deleted()`), it drops the message and exits the mapping.

**In array operations:**
```bloblang
# Array literal - deleted elements removed
output.items = [1, deleted(), 3]              # Result: [1, 3]

# map - deleted elements filtered out
output.positive = input.numbers.map(x -> if x > 0 { x } else { deleted() })
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
# If email not verified, field "email" is removed from object
```

**`deleted()` vs void:** These are different concepts. `deleted()` is an active deletion marker that removes elements and fields from collections. Void (from an if-without-else when false, or a match-without-`_` when no case matches) means "no value was produced" and is an **error** in collection literals — use `deleted()`, an `else` branch, or a `_` case instead. Void is only meaningful in assignments, where it causes the assignment to be skipped. See Section 4.1 for full void semantics.
```bloblang
output.items = [1, deleted(), 3]          # [1, 3] — deleted: element actively removed
output.items = [1, if false { 2 }, 3]     # ERROR: void in collection literal
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
  {"name": "Alice", "email": if true { "a@example.com" } else { deleted() }},
  {"name": "Bob", "email": if false { "b@example.com" } else { deleted() }}
]
# Result: [{"name": "Alice", "email": "a@example.com"}, {"name": "Bob"}]
```

`deleted()` works recursively at all nesting levels - elements are omitted wherever they appear in the structure.

**In conditional expressions and match arms:** Match arms are transparent — `deleted()` produced by a case arm flows out of the match expression and behaves exactly as it would from any other expression.
```bloblang
# Field actively removed when expression yields deleted()
output.category = if input.spam { deleted() } else { input.category }
# If spam, output.category field doesn't exist (not even with null value)

# Message drop — deleted() flows to root assignment, exits mapping
output = if input.spam { deleted() } else { input }
# If spam, message is dropped and mapping exits immediately

# deleted() from a match arm flows out identically
output.result = match input.x {
  "b" => deleted(),    # Field actively removed if x == "b"
  _ => input.x,
}

# Message drop via match
output = match input.type {
  "spam" => deleted(),   # Message dropped, mapping exits
  _ => input,
}
```

**In maps and lambdas:**
```bloblang
map filter_negative(val) {
  if val < 0 { deleted() } else { val }
}
output.result = filter_negative(input.value)  # Field deleted if value < 0
```

**Operations on `deleted()` are errors (except `.or()`):**
```bloblang
deleted() + 5                   # ERROR: cannot perform arithmetic on deleted
deleted() == deleted()          # ERROR: cannot compare deleted values
deleted().type()                # ERROR: cannot call methods on deleted value
deleted()?.field                # ERROR: ?. only short-circuits on null, not deleted
deleted().or("fallback")        # OK: returns "fallback" (.or() rescues deleted)
```

These operations result in **runtime errors** (or compile-time errors if detectable by implementation). The sole exception is `.or()`, which rescues `deleted()` the same way it rescues null and void (Section 8.3). **Method chain propagation:** When `deleted()` hits an unsupported method, the method produces an error. That error then propagates through subsequent methods (skipping them) until caught by `.catch()`, following normal error propagation rules (Section 8.2). For example, `deleted().uppercase().catch(err -> "recovered")` errors at `.uppercase()`, then `.catch()` catches the error and returns `"recovered"`.

**When deleted() Causes Errors vs Deletion:**

`deleted()` behaves differently depending on context:

**Triggers deletion (no error):**
- Field assignment: `output.field = deleted()` — removes the field
- Root output assignment: `output = deleted()` — drops the message and exits the mapping
- Metadata key assignment: `output@.key = deleted()` — removes the key
- Variable field assignment: `$var.field = deleted()` — removes the field from the variable's value
- Collection literals: `[1, deleted(), 3]` → `[1, 3]`, `{"a": deleted()}` → `{}`
- Return values from expressions used in assignments: `output.x = if spam { deleted() } else { value }`
- `map` lambda return value: element is filtered out

**Causes runtime error:**
- Variable assignment: `$var = deleted()` (cannot assign deleted to a variable)
- Array index assignment: `$arr[0] = deleted()`, `output.items[0] = deleted()` (cannot delete array elements by index — use `.filter()` to remove elements)
- Metadata root assignment: `output@ = deleted()` (cannot delete metadata object)
- Binary operators: `deleted() + 5`, `deleted() == deleted()`, `deleted() && true`
- Method calls (except `.or()`): `deleted().type()`, `deleted().uppercase()`
- Used as function arguments: `some_function(deleted())`
- Lambda return values in methods that do not support deletion (e.g., `filter`, `sort`). The standard library method that supports `deleted()` as a lambda return is `map`; extension methods may also support it.

The distinction: `deleted()` is a special marker that triggers deletion when flowing into a field/metadata assignment or collection, but cannot be used as a normal value in computations. Assigning `deleted()` to a variable (`$var = deleted()`) is an error; however, assigning `deleted()` to a field *within* a variable (`$var.field = deleted()`) removes that field from the variable's value. Assigning `deleted()` to an array index (`$arr[0] = deleted()`, `output.items[0] = deleted()`) is an error — use `.filter()` to remove array elements. The sole exception to method restrictions is `.or()`, which rescues `deleted()` and returns its argument (Section 8.3). When `deleted()` flows to the root output assignment, it drops the entire message and immediately exits the mapping.

**Routing messages instead of dropping them:**

To route failed/spam messages to a dead letter topic (rather than dropping them), the output document must exist:
```bloblang
# Route spam to dead letter with document intact
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

# Conditional array elements - use deleted() to omit elements
output.items = [
  input.a,
  if input.b != null { input.b } else { deleted() },  # Omitted if b is null
  input.c
]
# If b is null: [input.a, input.c]

# Void is an error in collection literals — always use deleted() or an else branch
output.items = [
  input.a,
  if input.b != null { input.b },  # ERROR: void in array literal when b is null
  input.c
]
```
