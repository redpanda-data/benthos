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

**Restoration:** Reassigning to a deleted target restores it:
```bloblang
output = deleted()              # Document deleted
output = "hello"                # Document restored with new value

output@ = deleted()             # All metadata deleted
output@.key = "value"           # Metadata restored with one key
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
output.mixed = ["a", if false { "b" } else { deleted() }, "c"]  # Result: ["a", "c"]

# map_each - deleted elements filtered out
output.positive = input.numbers.map_each(x -> if x > 0 { x } else { deleted() })
# Input: [-1, 2, -3, 4] → Output: [2, 4]
```

**In object literals:**
```bloblang
output.user = {
  "id": input.id,
  "email": if input.email_verified { input.email } else { deleted() },
  "phone": input.phone
}
# If email not verified, field "email" is omitted from object
```

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

# Conditional array elements (arrays can't have deleted)
$items = [input.a]
$items = if input.b != null { $items.append(input.b) } else { $items }
```
