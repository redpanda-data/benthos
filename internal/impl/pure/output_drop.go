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
			Field(service.NewObjectField("").Default(map[string]any{})),
		func(conf *service.ParsedConfig, res *service.Resources) (out service.BatchOutput, batchPolicy service.BatchPolicy, maxInFlight int, err error) {
			nm := interop.UnwrapManagement(res)
			var o output.Streamed
			if o, err = output.NewAsyncWriter("drop", 1, newDropWriter(nm), nm); err != nil {
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
