// Copyright 2025 Redpanda Data, Inc.

package policy

import "github.com/redpanda-data/benthos/v4/internal/docs"

// FieldSpec returns a spec for a common batching field.
func FieldSpec() docs.FieldSpec {
	return docs.FieldSpec{
		Name:        "batching",
		Type:        docs.FieldTypeObject,
		Description: `Configure a xref:configuration:batching.adoc[batching policy].`,
		Examples: []any{
			map[string]any{
				"count":     0,
				"byte_size": 5000,
				"period":    "1s",
			},
			map[string]any{
				"count":  10,
				"period": "1s",
			},
			map[string]any{
				"count":  0,
				"period": "1m",
				"check":  `this.contains("END BATCH")`,
			},
		},
		Children: docs.FieldSpecs{
			docs.FieldInt(
				"count",
				"The number of messages at which the batch is flushed. Set to `0` to disable count-based batching.",
			).HasDefault(0),
			docs.FieldInt(
				"byte_size",
				`The maximum total size (in bytes) that a batch can reach before it is flushed. When the combined size of all messages in the batch reaches or exceeds this limit, the batch is immediately sent to the next stage (such as a processor or output).

Set to `+"`"+`0`+"`"+` to disable size-based batching. When disabled, messages are flushed based on other conditions (such as `+"`"+`count`+"`"+` or `+"`"+`period`+"`"+`).`,
			).HasDefault(0),
			docs.FieldString(
				"period",
				"The length of time after which an incomplete batch is flushed regardless of its size. This field accepts Go duration format strings such as `100ms`, `1s`, or `5s`. Supported time units are `ns`, `us`, `ms`, `s`, `m`, and `h`.",
				"1s", "1m", "500ms",
			).HasDefault(""),
			docs.FieldBloblang(
				"check",
				"A xref:guides:bloblang/about.adoc[Bloblang query] that returns a boolean value indicating whether a message should end a batch.",
				`this.type == "end_of_transaction"`,
			).HasDefault(""),
			docs.FieldProcessor(
				"processors",
				"A list of xref:components:processors/about.adoc[processors] to apply to a batch as it is flushed. This allows you to aggregate and archive the batch however you see fit. All resulting messages are flushed as a single batch, so splitting the batch into smaller batches with these processors has no effect.",
				[]map[string]any{
					{
						"archive": map[string]any{
							"format": "concatenate",
						},
					},
				},
				[]map[string]any{
					{
						"archive": map[string]any{
							"format": "lines",
						},
					},
				},
				[]map[string]any{
					{
						"archive": map[string]any{
							"format": "json_array",
						},
					},
				},
			).Array().Advanced().Optional(),
		},
	}
}
