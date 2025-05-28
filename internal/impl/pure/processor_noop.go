// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"context"

	"github.com/redpanda-data/benthos/v4/internal/component/interop"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func init() {
	service.MustRegisterBatchProcessor("noop", service.NewConfigSpec().
		Stable().
		Summary("Noop is a processor that does nothing, the message passes through unchanged. Why? Sometimes doing nothing is the braver option.").
		Field(service.NewObjectField("").Default(map[string]any{})),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchProcessor, error) {
			p := &noopProcessor{}
			return interop.NewUnwrapInternalBatchProcessor(p), nil
		})
}

type noopProcessor struct{}

func (c *noopProcessor) ProcessBatch(ctx context.Context, msg message.Batch) ([]message.Batch, error) {
	msgs := [1]message.Batch{msg}
	return msgs[:], nil
}

func (c *noopProcessor) Close(context.Context) error {
	return nil
}
