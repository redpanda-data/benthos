// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"context"

	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/component/interop"
	"github.com/redpanda-data/benthos/v4/internal/component/output"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func init() {
	service.MustRegisterBatchOutput(
		"drop", service.NewConfigSpec().
			Stable().
			Categories("Utility").
			Summary(`Drops all messages.`).
			Description(`The `+"`drop`"+` output acknowledges each message as soon as it receives it and then discards it, without errors or side effects. Because it adds almost no overhead, it's useful for measuring input and processing throughput without an output bottleneck, for temporarily disabling output while you develop a pipeline, for discarding unwanted messages in a xref:components:outputs/switch.adoc[`+"`switch`"+`] output, and as the final fallback in a xref:components:outputs/fallback.adoc[`+"`fallback`"+`] output.`).
			Field(service.NewObjectField("").Default(map[string]any{})),
		func(conf *service.ParsedConfig, res *service.Resources) (out service.BatchOutput, batchPolicy service.BatchPolicy, maxInFlight int, err error) {
			nm := interop.UnwrapManagement(res)
			var o output.Streamed
			if o, err = output.NewAsyncWriter("drop", 1, false, newDropWriter(nm), nm); err != nil {
				return
			}
			out = interop.NewUnwrapInternalOutput(o)
			return
		})
}

type dropWriter struct {
	mgr component.Observability
}

func newDropWriter(mgr component.Observability) *dropWriter {
	return &dropWriter{mgr: mgr}
}

func (d *dropWriter) ConnectionTest(ctx context.Context) component.ConnectionTestResults {
	return component.ConnectionTestSucceeded(d.mgr).AsList()
}

func (d *dropWriter) Connect(ctx context.Context) error {
	return nil
}

func (d *dropWriter) WriteBatch(ctx context.Context, msg message.Batch) error {
	return nil
}

func (d *dropWriter) Close(context.Context) error {
	return nil
}
