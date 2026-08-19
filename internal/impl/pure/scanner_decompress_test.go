// Copyright 2026 Redpanda Data, Inc.

package pure_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/scanner/testutil"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func TestDecompressScannerSuite(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  decompress:
    algorithm: gzip
    into:
      lines:
        custom_delimiter: X
`, nil)
	require.NoError(t, err)

	rdr, err := pConf.FieldScanner("test")
	require.NoError(t, err)

	inputBytes, err := hex.DecodeString("1f8b080000096e8800ff001e00e1ff68656c6c6f58776f726c64587468697358697358636f6d7072657373656403009104d92d1e000000")
	require.NoError(t, err)

	testutil.ScannerTestSuite(t, rdr, nil, inputBytes, "hello", "world", "this", "is", "compressed")
}

// hintCapture records the source details observed by the capture_hint_test
// scanner, so a test can assert on what a wrapping scanner propagated.
var hintCapture = struct {
	sync.Mutex
	name string
	size int64
}{}

func init() {
	testutil.MustRegisterDetailsCaptureScanner("capture_hint_test", func(details *service.ScannerSourceDetails) {
		hintCapture.Lock()
		hintCapture.name = details.Name()
		hintCapture.size = details.SizeHint()
		hintCapture.Unlock()
	})
}

// TestDecompressScannerStripsSizeHint asserts that a size hint, which
// describes the compressed stream, is not forwarded to the child scanner
// reading the decompressed one, while the rest of the details survive.
func TestDecompressScannerStripsSizeHint(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  decompress:
    algorithm: gzip
    into:
      capture_hint_test: {}
`, nil)
	require.NoError(t, err)

	rdr, err := pConf.FieldScanner("test")
	require.NoError(t, err)

	inputBytes, err := hex.DecodeString("1f8b080000096e8800ff001e00e1ff68656c6c6f58776f726c64587468697358697358636f6d7072657373656403009104d92d1e000000")
	require.NoError(t, err)

	details := service.NewScannerSourceDetails()
	details.SetName("compressed.gz")
	details.SetSizeHint(int64(len(inputBytes)))

	strm, err := rdr.Create(io.NopCloser(bytes.NewReader(inputBytes)), func(ctx context.Context, err error) error {
		return nil
	}, details)
	require.NoError(t, err)

	m, _, err := strm.NextBatch(t.Context())
	require.NoError(t, err)
	require.Len(t, m, 1)

	mBytes, err := m[0].AsBytes()
	require.NoError(t, err)
	assert.Equal(t, "helloXworldXthisXisXcompressed", string(mBytes))

	hintCapture.Lock()
	capturedName, capturedSize := hintCapture.name, hintCapture.size
	hintCapture.Unlock()
	assert.Equal(t, "compressed.gz", capturedName, "name should survive the decompress layer")
	assert.Zero(t, capturedSize, "compressed size hint should not reach the child scanner")

	require.NoError(t, strm.Close(t.Context()))
}
