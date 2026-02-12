# 7. Execution Model

## 7.1 Immutable Input, Mutable Output

**Input (document + metadata) is always immutable:**
```bloblang
output.invitees = input.invitees.filter(i -> i.mood >= 0.5)
output.rejected = input.invitees.filter(i -> i.mood < 0.5)
# input.invitees unchanged - both see original

# Input metadata also immutable
output.original_topic = input@.kafka_topic
output@.kafka_topic = "processed"
output.still_original = input@.kafka_topic  # Still original value
```

**Output (document + metadata) built incrementally:**
```bloblang
output.user.id = input.id         # Creates output.user.id
output.user.name = input.name     # Adds output.user.name
output@.kafka_topic = "processed" # Adds output metadata
```

**Initial state:** `output` starts as empty object `{}`, so `output.field` returns `null` before assignment.

**Scalar output:** `output` can be assigned any type (string, number, etc.):
```bloblang
output = "foo"         # Replace object with string
output.field           # ERROR: cannot access field of string
```

## 7.2 Order Independence

Assignment order doesn't affect correctness (input never changes):
```bloblang
# These produce same result regardless of order
output.count = input.items.length()
output.active = input.items.filter(i -> i.active)
output.count2 = input.items.length()  # Same as count
```

**Benefit:** Easier to refactor, statements can be reordered.

## 7.3 Copy-and-Modify Pattern

```bloblang
# Copy document
output = input
output.password = deleted()
output.processed_at = now()

# Copy metadata
output@ = input@
output@.kafka_topic = "new-topic"
```

## 7.4 Contexts

**Input Context:**
- `input.field` - Document field (immutable)
- `input@.key` - Metadata key (immutable)
- Always refers to original input message

**Output Context:**
- `output.field` - Document field (mutable)
- `output@.key` - Metadata key (mutable)
- Built incrementally during execution

**Variables:**
- `$variable` - Block-scoped, immutable
- Can shadow variables from outer scopes

## 7.5 Metadata

Messages have metadata separate from document payload.

**Access:**
```bloblang
# Read input metadata (immutable)
output.topic = input@.kafka_topic
output.partition = input@.kafka_partition

# Write output metadata (mutable)
output@.kafka_topic = "processed-topic"
output@.kafka_key = input.id
output@.content_type = "application/json"

# Delete metadata
output@.kafka_key = deleted()
```

**Types:**
Metadata values can be any type (string, number, bool, null, bytes, array, object):
```bloblang
output@.retry_count = 5
output@.tags = ["urgent", "customer-service"]
output@.routing = {"region": "us-west", "priority": 10}
```

**Copy all metadata:**
```bloblang
output@ = input@                    # Copy all
output@.kafka_topic = "new-topic"   # Override specific
```

Undefined metadata keys return `null`.

## 7.6 Scoping Rules

**Top-level scope:**
- Variables accessible throughout mapping
- Maps accessible globally (or via namespace if imported)

**Block scope:**
- Variables in `if`, `match`, lambda bodies
- Only accessible within declaring block and nested blocks
- Can shadow outer variables

```bloblang
$global = 10

output.result = if input.flag {
  $local = 20        # Only in this block
  $global + $local   # Can access both
}

# $local not accessible here
output.final = $global  # Still 10
```

**Shadowing:**
```bloblang
$value = 10

output.inner = if input.flag {
  $value = 20        # NEW variable, shadows outer
  $value             # Returns 20
}

output.outer = $value  # Still 10
```

## 7.7 Variable Immutability

Variables cannot be reassigned in same scope:
```bloblang
$value = 10
$value = 20      # ERROR: cannot reassign
```

Shadowing in inner scope is allowed (creates new variable).

## 7.8 Evaluation Order

Statements execute sequentially, top-to-bottom.
Variables must be declared before use.
Later statements can reference earlier `output` fields:

```bloblang
output.price = input.price
output.tax = output.price * 0.1          # Uses earlier output
output.total = output.price + output.tax
```
