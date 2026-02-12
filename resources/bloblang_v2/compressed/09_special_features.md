# 9. Special Features

## 9.1 Dynamic Field Names

```bloblang
# Dynamic field access
$field_name = "user_id"
output.value = input.get($field_name)

# Dynamic field creation
$key = "dynamic_field"
output.set($key, "value")
```

## 9.2 Message Filtering

**Delete document:**
```bloblang
output = if input.spam {
  deleted()
} else {
  input
}
```

**Behavior:**
- `output = deleted()` marks document as deleted (metadata persists)
- `output@ = deleted()` explicitly deletes all metadata
- Document and metadata are independent during execution
- Reassigning output unmarks deletion: `output = "hello"` restores document with metadata intact
- If mapping ends with output deleted, system removes entire message (document + metadata)

**Examples:**
```bloblang
# Delete everything
output = deleted()
output@ = deleted()

# Delete document, keep metadata for routing
output = deleted()
output@.reason = "spam_detected"
output@.kafka_topic = "dead_letter"

# Conditional restore
output = deleted()
output = if input.override { input } else { deleted() }
```

Downstream processors remove deleted messages from stream.

## 9.3 Message Expansion

Return array to expand into multiple messages:
```bloblang
$doc_root = input.without("items")
output = input.items.map_each(item -> $doc_root.merge(item))
```

Downstream processors split array into separate messages.

## 9.4 Non-Structured Data

Handle raw strings/bytes:
```bloblang
# If input is raw string
output.parsed = input.parse_json()

# If input is raw bytes
output.decoded = input.string()
```

## 9.5 Conditional Literals

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
