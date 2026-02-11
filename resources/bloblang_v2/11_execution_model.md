# 11. Execution Model

Bloblang V2 uses a **single, immutable execution model** for predictable, order-independent behavior.

## 11.1 Immutable Input, Mutable Output

**Input Document and Metadata**: The `input` context (both document and metadata) is **immutable** throughout execution. It always refers to the original input message:

```bloblang
output.id = input.id
output.invitees = input.invitees.filter(i -> i.mood >= 0.5)
output.rejected = input.invitees.filter(i -> i.mood < 0.5)  # Original still accessible

# Input metadata is also immutable:
output.original_topic = input@.kafka_topic
output@.kafka_topic = "processed-topic"
output.still_original = input@.kafka_topic  # Still returns original value

# Order doesn't matter - input never changes:
output.a = input.items.length()
output.b = input.items.filter(x -> x.active)
output.c = input.items.length()  # Still the same value as output.a
```

**Output Document and Metadata**: The `output` context (both document and metadata) is **built incrementally**. Each assignment adds or modifies fields in the output:

```bloblang
output.user.id = input.id         # Creates output.user.id
output.user.name = input.name     # Adds output.user.name
output.status = "processed"       # Adds output.status
output@.kafka_topic = "processed" # Creates output metadata
```

## 11.2 Assignment Order Independence

Because `input` is immutable, **assignment order doesn't affect correctness**:

```bloblang
# These produce the same result regardless of order:

# Order A:
output.active = input.users.filter(u -> u.active)
output.count = input.users.length()

# Order B (same result):
output.count = input.users.length()
output.active = input.users.filter(u -> u.active)
```

**Benefit**: Mappings are easier to read, refactor, and reason about.

## 11.3 Evaluation Order

Despite order-independence, assignments still execute **sequentially in source order**:

- Statements execute top-to-bottom
- Variables must be declared before use
- Later statements can reference earlier `output` fields

```bloblang
output.price = input.price
output.tax = output.price * 0.1        # Can reference earlier output
output.total = output.price + output.tax
```

## 11.4 Copy-and-Modify Pattern

To copy the input and modify specific fields:

```bloblang
output = input                    # Copy entire input document
output.password = deleted()       # Remove sensitive field
output.processed_at = now()       # Add new field
output.status = "processed"       # Modify field

output@ = input@                  # Copy entire input metadata
output@.kafka_topic = "processed" # Override specific metadata key
```
