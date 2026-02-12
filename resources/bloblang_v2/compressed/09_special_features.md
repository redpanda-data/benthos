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

**Type operations:** You cannot call methods on `deleted()`:
```bloblang
deleted().type()                # ERROR: cannot call methods on deleted value
```

**Practical use - message filtering:**
```bloblang
# Conditional filtering
output = if input.spam {
  deleted()
} else {
  input
}
```

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
