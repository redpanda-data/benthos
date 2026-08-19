// Copyright 2026 Redpanda Data, Inc.

package testutil

import (
	"context"
	"io"

	"github.com/redpanda-data/benthos/v4/public/service"
)

// MustRegisterDetailsCaptureScanner registers a scanner plugin under name that
// invokes fn with the source details each stream is created with, then reads
// the stream to the end and delivers it as a single message. It is intended
// for tests asserting on the details an input or wrapping scanner propagates.
func MustRegisterDetailsCaptureScanner(name string, fn func(*service.ScannerSourceDetails)) {
	service.MustRegisterBatchScannerCreator(name,
		service.NewConfigSpec().Field(service.NewObjectField("").Default(map[string]any{})),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchScannerCreator, error) {
			return &captureScannerCreator{fn: fn}, nil
		})
}

type captureScannerCreator struct {
	fn func(*service.ScannerSourceDetails)
}

func (c *captureScannerCreator) Create(rdr io.ReadCloser, aFn service.AckFunc, details *service.ScannerSourceDetails) (service.BatchScanner, error) {
	c.fn(details)
	return service.AutoAggregateBatchScannerAcks(&captureScanner{r: rdr}, aFn), nil
}

func (c *captureScannerCreator) Close(context.Context) error { return nil }

type captureScanner struct {
	r io.ReadCloser
}

func (c *captureScanner) NextBatch(ctx context.Context) (service.MessageBatch, error) {
	if c.r == nil {
		return nil, io.EOF
	}
	b, err := io.ReadAll(c.r)
	if err != nil {
		return nil, err
	}
	_ = c.r.Close()
	c.r = nil
	return service.MessageBatch{service.NewMessage(b)}, nil
}

func (c *captureScanner) Close(ctx context.Context) error {
	if c.r == nil {
		return nil
	}
	return c.r.Close()
}
