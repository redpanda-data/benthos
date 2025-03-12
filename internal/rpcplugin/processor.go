// Copyright 2025 Redpanda Data, Inc.

package rpcplugin

import (
	"context"

	"github.com/redpanda-data/benthos/v4/public/service"
)

type processorPluginClient struct {
	path string
}

func newProcessorPluginClient(path string) (*processorPluginClient, error) {
	// TODO
	return nil, nil
}

func (p *processorPluginClient) ProcessBatch(ctx context.Context, batch service.MessageBatch) ([]service.MessageBatch, error) {
	return nil, nil // TODO
}

func (p *processorPluginClient) Close(ctx context.Context) error {
	return nil // TODO
}
