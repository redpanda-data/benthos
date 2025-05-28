// Copyright 2025 Redpanda Data, Inc.

package codec_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"
	"github.com/redpanda-data/benthos/v4/public/service/codec"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func TestInteropCodecOldStyle(t *testing.T) {
	confSpec := service.NewConfigSpec().Fields(codec.DeprecatedCodecFields("lines")...)
	pConf, err := confSpec.ParseYAML(`
codec: lines
max_buffer: 1000000
`, nil)
	require.NoError(t, err)

	rdr, err := codec.DeprecatedCodecFromParsed(pConf)
	require.NoError(t, err)

	buf := bytes.NewReader([]byte(`first
second
third`))
	var acked bool
	strm, err := rdr.Create(io.NopCloser(buf), func(ctx context.Context, err error) error {
		acked = true
		return nil
	}, service.NewScannerSourceDetails())
	require.NoError(t, err)

	for _, s := range []string{
		"first", "second", "third",
	} {
		m, aFn, err := strm.NextBatch(t.Context())
		require.NoError(t, err)
		require.Len(t, m, 1)
		mBytes, err := m[0].AsBytes()
		require.NoError(t, err)
		assert.Equal(t, s, string(mBytes))
		require.NoError(t, aFn(t.Context(), nil))
		assert.False(t, acked)
	}

	_, _, err = strm.NextBatch(t.Context())
	require.Equal(t, io.EOF, err)

	require.NoError(t, strm.Close(t.Context()))
	assert.True(t, acked)
}

func TestInteropCodecNewStyle(t *testing.T) {
	confSpec := service.NewConfigSpec().Fields(codec.DeprecatedCodecFields("lines")...)
	pConf, err := confSpec.ParseYAML(`
scanner:
  lines:
    custom_delimiter: 'X'
    max_buffer_size: 200
`, nil)
	require.NoError(t, err)

	rdr, err := codec.DeprecatedCodecFromParsed(pConf)
	require.NoError(t, err)

	buf := bytes.NewReader([]byte(`firstXsecondXthird`))
	var acked bool
	strm, err := rdr.Create(io.NopCloser(buf), func(ctx context.Context, err error) error {
		acked = true
		return nil
	}, service.NewScannerSourceDetails())
	require.NoError(t, err)

	for _, s := range []string{
		"first", "second", "third",
	} {
		m, aFn, err := strm.NextBatch(t.Context())
		require.NoError(t, err)
		require.Len(t, m, 1)
		mBytes, err := m[0].AsBytes()
		require.NoError(t, err)
		assert.Equal(t, s, string(mBytes))
		require.NoError(t, aFn(t.Context(), nil))
		assert.False(t, acked)
	}

	_, _, err = strm.NextBatch(t.Context())
	require.Equal(t, io.EOF, err)

	require.NoError(t, strm.Close(t.Context()))
	assert.True(t, acked)
}

func TestInteropCodecDefault(t *testing.T) {
	confSpec := service.NewConfigSpec().Fields(codec.DeprecatedCodecFields("lines")...)
	pConf, err := confSpec.ParseYAML(`{}`, nil)
	require.NoError(t, err)

	rdr, err := codec.DeprecatedCodecFromParsed(pConf)
	require.NoError(t, err)

	buf := bytes.NewReader([]byte("first\nsecond\nthird"))
	var acked bool
	strm, err := rdr.Create(io.NopCloser(buf), func(ctx context.Context, err error) error {
		acked = true
		return nil
	}, service.NewScannerSourceDetails())
	require.NoError(t, err)

	for _, s := range []string{
		"first", "second", "third",
	} {
		m, aFn, err := strm.NextBatch(t.Context())
		require.NoError(t, err)
		require.Len(t, m, 1)
		mBytes, err := m[0].AsBytes()
		require.NoError(t, err)
		assert.Equal(t, s, string(mBytes))
		require.NoError(t, aFn(t.Context(), nil))
		assert.False(t, acked)
	}

	_, _, err = strm.NextBatch(t.Context())
	require.Equal(t, io.EOF, err)

	require.NoError(t, strm.Close(t.Context()))
	assert.True(t, acked)
}

func TestInteropCodecError(t *testing.T) {
	// This test asserts that `DeprecatedFallbackCodec.Create` returns a nil `DeprecatedFallbackStream` if the scanner
	// `Create` method returns an error.

	confSpec := service.NewConfigSpec().Fields(codec.DeprecatedCodecFields("lines")...)
	pConf, err := confSpec.ParseYAML(`
scanner:
  decompress:
    algorithm: gzip
    into:
      lines: {}
`, nil)
	require.NoError(t, err)

	rdr, err := codec.DeprecatedCodecFromParsed(pConf)
	require.NoError(t, err)

	// The `decompress` scanner will error out when trying to parse the `gzip` header from the nil buffer.
	buf := bytes.NewReader(nil)
	strm, err := rdr.Create(io.NopCloser(buf), func(ctx context.Context, err error) error {
		return nil
	}, service.NewScannerSourceDetails())
	require.ErrorIs(t, err, io.EOF)
	if strm != nil {
		assert.Fail(t, "expected nil stream")
	}
}
