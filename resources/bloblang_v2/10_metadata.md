# 10. Metadata

Access and modify message metadata using `@` prefix notation:

## Reading Metadata

```bloblang
output.topic = @kafka_topic
output.partition = @kafka_partition
output.key = @kafka_key
```

## Writing Metadata

```bloblang
@output_topic = "processed"
@kafka_key = input.id
@content_type = "application/json"
```

## Deleting Metadata

```bloblang
@kafka_key = deleted()
```

## Semantics

- Metadata keys are accessed and assigned using the `@` prefix
- Metadata is separate from the document payload
- Metadata assignments do not affect the output document
- Use `deleted()` to remove metadata keys
