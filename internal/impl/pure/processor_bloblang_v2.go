// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"context"
	"errors"

	tracing "github.com/redpanda-data/benthos/v4/internal/tracing/v2"
	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func init() {
	service.MustRegisterBatchProcessor("bloblang_v2", bloblangV2ProcConfig(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchProcessor, error) {
			return newBloblangV2FromParsed(conf, mgr)
		})
}

func bloblangV2ProcConfig() *service.ConfigSpec {
	return service.NewConfigSpec().
		Experimental().
		Categories("Mapping", "Parsing").
		Field(service.NewBloblangV2Field("")).
		Summary("Executes a Bloblang V2 mapping on messages, producing a new document that replaces (or filters) the original message.").
		Description(`
Bloblang V2 is a redesigned mapping language with explicit input/output
contexts and deterministic evaluation. See the V2 specification in
`+"`internal/bloblang2/spec`"+` for the full language reference.

== Experimental feature

Bloblang V2 and this processor are experimental, made available for early
adopters. We are actively looking for feedback on the language and its
tooling, and details of both may change in response outside of major version
releases. The V1 `+"`mapping`"+`, `+"`mutation`"+` and `+"`bloblang`"+`
processors remain the stable choice for production pipelines.

== Input and output semantics

Each message is evaluated with `+"`input`"+` bound to the incoming document
and `+"`output`"+` starting as an empty object. The mapping is expected to
build up `+"`output`"+` (or assign it directly), for example:

`+"```"+`
output.id = input.id
output.fans = input.fans.filter(f -> f.obsession > 0.5)
`+"```"+`

If the mapping assigns `+"`output = deleted()`"+` the message is filtered out
of the batch. If the mapping fails the original message continues down the
pipeline but is marked with the error via standard processor error handling.

== Metadata

Metadata follows V2 semantics: `+"`input@`"+` exposes the incoming metadata
(immutable) and `+"`output@`"+` starts as an empty object on every
invocation. Whatever the mapping writes to `+"`output@`"+` becomes the
metadata of the produced message — metadata is not preserved implicitly.
To copy the incoming metadata through, write:

`+"```"+`
output@ = input@
`+"```"+`

This differs from the V1 `+"`mapping`"+` processor, which preserves metadata
by default.

== Migrating from Bloblang V1

A migrator tool converts existing configs to Bloblang V2 automatically. It
parses each V1 mapping (`+"`bloblang`"+`, `+"`mapping`"+` and
`+"`mutation`"+` processors, plus in-line Bloblang plugin fields) and
translates it construct-by-construct into an equivalent V2 mapping,
rewriting the config to use this processor (or `+"`bloblang_v2_file`"+`
for mappings loaded with `+"`from \"path\"`"+`, following and translating
any V1 import files along the way).

Where V1 and V2 semantics genuinely differ the migrator does not guess
silently: each such rewrite is reported with a severity and an explanation
of the behavioural difference, constructs with no V2 equivalent are marked
unsupported and left for manual attention, and a per-file coverage ratio
summarises how much translated cleanly.

A short guide:

1. Preview what would change without writing anything:
`+"`rpk connect migrate v5 --check ./config.yaml`"+`.
2. Run the migration for real: `+"`rpk connect migrate v5 ./config.yaml`"+`
writes a sibling `+"`config.v5.yaml`"+` (use `+"`--in-place`"+` to
overwrite, which keeps a `+"`.bak`"+` backup).
3. Review every reported warning — each one names a V1/V2 semantic
divergence in your mapping — and address anything marked unsupported.
4. Validate the migrated config against representative data before swapping
it into production.

For programmatic migration the same machinery is available as Go APIs:
`+"`public/service/migrator`"+` operates on whole configs and
`+"`public/bloblangv2/migrator`"+` on individual mappings.
`).
		Example("Mapping with metadata preserved", `
Given JSON documents containing an array of fans, reduce them to just the ID
and the fans with an obsession score above 0.5, while keeping the original
metadata on the resulting message:`,
			`
pipeline:
  processors:
    - bloblang_v2: |
        output@ = input@
        output.id = input.id
        output.fans = input.fans.filter(fan -> fan.obsession > 0.5)
`)
}

func newBloblangV2FromParsed(conf *service.ParsedConfig, mgr *service.Resources) (*bloblangV2Proc, error) {
	exec, err := conf.FieldBloblangV2()
	if err != nil {
		return nil, err
	}
	return &bloblangV2Proc{exec: exec, log: mgr.Logger()}, nil
}

type bloblangV2Proc struct {
	exec *bloblangv2.Executor
	log  *service.Logger
}

func (p *bloblangV2Proc) ProcessBatch(_ context.Context, batch service.MessageBatch) ([]service.MessageBatch, error) {
	newBatch := make(service.MessageBatch, 0, len(batch))
	batchSize := len(batch)
	for i, msg := range batch {
		input, err := messageInputValue(msg)
		if err != nil {
			newMsg := msg.Copy()
			newMsg.SetError(err)
			p.log.Errorf("%v", err)
			newBatch = append(newBatch, newMsg)
			continue
		}

		inputMeta := map[string]any{}
		_ = msg.MetaWalkMut(func(k string, v any) error {
			inputMeta[k] = v
			return nil
		})

		ctx := &messageContext{
			msg:        msg,
			input:      input,
			meta:       inputMeta,
			batchIndex: i,
			batchSize:  batchSize,
		}
		output, outputMeta, err := p.exec.QueryMessage(ctx)
		if err != nil {
			if errors.Is(err, bloblangv2.ErrRootDeleted) {
				continue
			}
			newMsg := msg.Copy()
			newMsg.SetError(err)
			p.log.Errorf("%v", err)
			newBatch = append(newBatch, newMsg)
			continue
		}

		newMsg := msg.Copy()
		switch v := output.(type) {
		case []byte:
			newMsg.SetBytes(v)
		case string:
			newMsg.SetBytes([]byte(v))
		default:
			newMsg.SetStructured(output)
		}

		// V2 metadata semantics: output@ starts empty, so the produced
		// metadata fully replaces the message metadata on each invocation.
		_ = newMsg.MetaWalkMut(func(k string, _ any) error {
			newMsg.MetaDelete(k)
			return nil
		})
		for k, v := range outputMeta {
			newMsg.MetaSetMut(k, v)
		}

		newBatch = append(newBatch, newMsg)
	}
	if len(newBatch) == 0 {
		return nil, nil
	}
	return []service.MessageBatch{newBatch}, nil
}

func (p *bloblangV2Proc) Close(context.Context) error {
	return nil
}

// messageInputValue returns the value to bind to `input` in the mapping.
// Structured messages parse to their JSON-equivalent Go value; raw messages
// fall back to their byte contents. The V2 interpreter normalises json.Number
// values internally, so callers do not need to pre-process them.
func messageInputValue(msg *service.Message) (any, error) {
	if v, err := msg.AsStructured(); err == nil {
		return v, nil
	}
	b, err := msg.AsBytes()
	if err != nil {
		return nil, err
	}
	return b, nil
}

// messageContext is the adapter that exposes a service.Message + its
// position within a batch through the bloblangv2.MessageContext
// surface. The bundled batch-3 stdlib (batch_index, content, error,
// tracing_id, ...) reads from this adapter.
type messageContext struct {
	msg        *service.Message
	input      any
	meta       map[string]any
	batchIndex int
	batchSize  int
}

func (c *messageContext) Input() any               { return c.input }
func (c *messageContext) Metadata() map[string]any { return c.meta }
func (c *messageContext) BatchIndex() int          { return c.batchIndex }
func (c *messageContext) BatchSize() int           { return c.batchSize }
func (c *messageContext) Error() error             { return c.msg.GetError() }
func (c *messageContext) TraceID() string          { return tracing.GetTraceID(c.msg) }
func (c *messageContext) Span() any {
	if s := tracing.GetSpan(c.msg); s != nil {
		return s
	}
	return nil
}

func (c *messageContext) Bytes() []byte {
	b, err := c.msg.AsBytes()
	if err != nil {
		return nil
	}
	return b
}
