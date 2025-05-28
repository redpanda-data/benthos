// Copyright 2025 Redpanda Data, Inc.

package pure_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/scanner/testutil"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func TestLinesScanner(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  lines:
    custom_delimiter: 'X'
    max_buffer_size: 200
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

func TestLinesScannerSuite(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))

	tests := []struct {
		name      string
		input     string
		want      []string
		omitEmpty bool
	}{
		{
			name:      "parses input",
			input:     "firstXsecondXthird",
			want:      []string{"first", "second", "third"},
			omitEmpty: false,
		},
		{
			name:      "parses input and emits empty line",
			input:     "firstXsecondXXthird",
			want:      []string{"first", "second", "", "third"},
			omitEmpty: false,
		},
		{
			name:      "parses input and omits empty line",
			input:     "firstXsecondXXthird",
			want:      []string{"first", "second", "third"},
			omitEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pConf, err := confSpec.ParseYAML(fmt.Sprintf(`
test:
  lines:
    custom_delimiter: 'X'
    max_buffer_size: 200
    omit_empty: %t
`, tt.omitEmpty), nil)
			require.NoError(t, err)

			rdr, err := pConf.FieldScanner("test")
			require.NoError(t, err)

			testutil.ScannerTestSuite(t, rdr, nil, []byte(tt.input), tt.want...)
		})
	}
}
