// Copyright 2026 Redpanda Data, Inc.

package io_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	scannertestutil "github.com/redpanda-data/benthos/v4/internal/component/scanner/testutil"
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
	scannertestutil.MustRegisterDetailsCaptureScanner("capture_size_test", func(details *service.ScannerSourceDetails) {
		sizeCapture.Lock()
		sizeCapture.sizes[filepath.Base(details.Name())] = details.SizeHint()
		sizeCapture.Unlock()
	})
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
