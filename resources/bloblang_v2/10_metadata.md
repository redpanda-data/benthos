# 10. Metadata

Messages carry metadata separate from the document payload. Access metadata using `@.` notation after `input` or `output`:

## Reading Input Metadata

Input metadata is **immutable** and accessed via `input@.key`:

```bloblang
output.topic = input@.kafka_topic
output.partition = input@.kafka_partition
output.key = input@.kafka_key

# Null-safe access
output.content_type = input@.content_type.or("application/json")
```

## Writing Output Metadata

Output metadata is **mutable** and assigned via `output@.key`:

```bloblang
output@.kafka_topic = "processed-topic"
output@.kafka_key = input.id
output@.content_type = "application/json"
```

## Deleting Metadata

Remove metadata keys using `deleted()`:

```bloblang
output@.kafka_key = deleted()
```

## Copy and Modify

Copy all input metadata and modify specific keys:

```bloblang
output@ = input@                          # Copy all metadata
output@.kafka_topic = "new-topic"         # Override specific key
output@.processed_at = now()              # Add new key
output@.internal_field = deleted()        # Remove key
```

## Semantics

- **Input metadata** (`input@.key`): Immutable, always refers to original message metadata
- **Output metadata** (`output@.key`): Mutable, built incrementally like the output document
- Metadata is completely separate from document fields (`input.field` vs `input@.key`)
- Reading undefined metadata keys returns `null`
- Use `output@ = input@` to copy all metadata, similar to `output = input` for documents
