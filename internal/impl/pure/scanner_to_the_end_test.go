// Copyright 2026 Redpanda Data, Inc.

package pure_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/scanner/testutil"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func TestToTheEndScanner(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  to_the_end: {}
`, nil)
	require.NoError(t, err)

	rdr, err := pConf.FieldScanner("test")
	require.NoError(t, err)

	buf := bytes.NewReader([]byte(`firstXsecondXthird`))
	var acked bool
	strm, err := rdr.Create(io.NopCloser(buf), func(ctx context.Context, err error) error {
		acked = true
		return nil
	}, service.NewScannerSourceDetails())
	require.NoError(t, err)

	for _, s := range []string{
		"firstXsecondXthird",
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

// TestToTheEndScannerSizeHints asserts that the content returned is unchanged
// regardless of the size hint supplied, including hints that are absent, wrong
// in either direction, or negative. The hint is an optimisation only.
func TestToTheEndScannerSizeHints(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  to_the_end: {}
`, nil)
	require.NoError(t, err)

	rdr, err := pConf.FieldScanner("test")
	require.NoError(t, err)

	const content = `firstXsecondXthird`

	for _, test := range []struct {
		name    string
		details func() *service.ScannerSourceDetails
	}{
		{
			name:    "no details at all",
			details: func() *service.ScannerSourceDetails { return nil },
		},
		{
			name:    "details without a size",
			details: func() *service.ScannerSourceDetails { return service.NewScannerSourceDetails() },
		},
		{
			name: "exact size",
			details: func() *service.ScannerSourceDetails {
				d := service.NewScannerSourceDetails()
				d.SetSizeHint(int64(len(content)))
				return d
			},
		},
		{
			name: "size too small",
			details: func() *service.ScannerSourceDetails {
				d := service.NewScannerSourceDetails()
				d.SetSizeHint(3)
				return d
			},
		},
		{
			name: "size too large",
			details: func() *service.ScannerSourceDetails {
				d := service.NewScannerSourceDetails()
				d.SetSizeHint(int64(len(content)) * 100)
				return d
			},
		},
		{
			name: "negative size",
			details: func() *service.ScannerSourceDetails {
				d := service.NewScannerSourceDetails()
				d.SetSizeHint(-5)
				return d
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			buf := bytes.NewReader([]byte(content))
			strm, err := rdr.Create(io.NopCloser(buf), func(ctx context.Context, err error) error {
				return nil
			}, test.details())
			require.NoError(t, err)

			m, _, err := strm.NextBatch(t.Context())
			require.NoError(t, err)
			require.Len(t, m, 1)

			mBytes, err := m[0].AsBytes()
			require.NoError(t, err)
			assert.Equal(t, content, string(mBytes))

			_, _, err = strm.NextBatch(t.Context())
			require.Equal(t, io.EOF, err)
			require.NoError(t, strm.Close(t.Context()))
		})
	}
}

func TestToTheEndScannerSuite(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  to_the_end: {}
`, nil)
	require.NoError(t, err)

	rdr, err := pConf.FieldScanner("test")
	require.NoError(t, err)

	testutil.ScannerTestSuite(t, rdr, nil, []byte(`firstXsecondXthird`), "firstXsecondXthird")
}
