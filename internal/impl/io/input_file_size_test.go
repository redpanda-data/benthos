// Copyright 2025 Redpanda Data, Inc.

package io_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/public/service"
)

// sizeCapture records the source details observed by a scanner, so that a test
// can assert on what the file input actually propagated.
var sizeCapture = struct {
	sync.Mutex
	sizes map[string]int64
}{sizes: map[string]int64{}}

func init() {
	service.MustRegisterBatchScannerCreator("capture_size_test",
		service.NewConfigSpec().Field(service.NewObjectField("").Default(map[string]any{})),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchScannerCreator, error) {
			return &captureSizeScannerCreator{}, nil
		})
}

type captureSizeScannerCreator struct{}

func (c *captureSizeScannerCreator) Create(rdr io.ReadCloser, aFn service.AckFunc, details *service.ScannerSourceDetails) (service.BatchScanner, error) {
	if details != nil {
		sizeCapture.Lock()
		sizeCapture.sizes[filepath.Base(details.Name())] = details.SizeHint()
		sizeCapture.Unlock()
	}
	return service.AutoAggregateBatchScannerAcks(&captureSizeScanner{r: rdr}, aFn), nil
}

func (c *captureSizeScannerCreator) Close(context.Context) error { return nil }

type captureSizeScanner struct {
	r io.ReadCloser
}

func (c *captureSizeScanner) NextBatch(ctx context.Context) (service.MessageBatch, error) {
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

func (c *captureSizeScanner) Close(ctx context.Context) error {
	if c.r == nil {
		return nil
	}
	return c.r.Close()
}

// TestFileInputPropagatesSize asserts that the file input reads Size() from the
// Stat it already performs and passes it to the scanner, for regular files.
func TestFileInputPropagatesSize(t *testing.T) {
	tmpDir := t.TempDir()

	const content = "hello world, this content has a known length"
	path := filepath.Join(tmpDir, "sized.txt")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	conf, err := testutil.InputFromYAML(fmt.Sprintf(`
file:
  paths: [ "%v/*.txt" ]
  scanner:
    capture_size_test: {}
`, tmpDir))
	require.NoError(t, err)

	i, err := mock.NewManager().NewInput(conf)
	require.NoError(t, err)

	i.TriggerStartConsuming()

	var tran message.Transaction
	select {
	case tran = <-i.TransactionChan():
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	assert.Equal(t, content, string(tran.Payload.Get(0).AsBytes()))
	require.NoError(t, tran.Ack(t.Context(), nil))

	sizeCapture.Lock()
	got := sizeCapture.sizes["sized.txt"]
	sizeCapture.Unlock()

	assert.Equal(t, int64(len(content)), got,
		"file input should propagate the size it already obtained from Stat")
}
