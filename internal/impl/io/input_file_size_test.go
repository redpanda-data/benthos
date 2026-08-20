// Copyright 2026 Redpanda Data, Inc.

package io_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	scannertestutil "github.com/redpanda-data/benthos/v4/internal/component/scanner/testutil"
	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

// capturedSizes reports the source details observed by the capture_size_test
// scanner, so that a test can assert on what the file input propagated.
var capturedSizes = scannertestutil.MustRegisterDetailsCaptureScanner("capture_size_test")

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

	var got int64
	for _, c := range capturedSizes() {
		if filepath.Base(c.Name) == "sized.txt" {
			got = c.SizeHint
		}
	}
	assert.Equal(t, int64(len(content)), got,
		"file input should propagate the size it already obtained from Stat")
}
