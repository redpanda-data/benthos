// Copyright 2025 Redpanda Data, Inc.

package pure_test

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/scanner/testutil"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func TestCSVScannerDefault(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  csv: {}
`, nil)
	require.NoError(t, err)

	rdr, err := pConf.FieldScanner("test")
	require.NoError(t, err)

	testutil.ScannerTestSuite(t, rdr, nil, []byte(`a,b,c
a1,b1,c1
a2,b2,c2
a3,b3,c3
a4,b4,c4
`),
		`{"a":"a1","b":"b1","c":"c1"}`,
		`{"a":"a2","b":"b2","c":"c2"}`,
		`{"a":"a3","b":"b3","c":"c3"}`,
		`{"a":"a4","b":"b4","c":"c4"}`,
	)
}

func TestCSVScannerCustomDelim(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  csv:
    custom_delimiter: '|'
`, nil)
	require.NoError(t, err)

	rdr, err := pConf.FieldScanner("test")
	require.NoError(t, err)

	testutil.ScannerTestSuite(t, rdr, nil, []byte(`a|b|c
a1|b1|c1
a2|b2|c2
a3|b3|c3
a4|b4|c4
`),
		`{"a":"a1","b":"b1","c":"c1"}`,
		`{"a":"a2","b":"b2","c":"c2"}`,
		`{"a":"a3","b":"b3","c":"c3"}`,
		`{"a":"a4","b":"b4","c":"c4"}`,
	)
}

func TestCSVScannerNoHeaderRow(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  csv:
    parse_header_row: false
`, nil)
	require.NoError(t, err)

	rdr, err := pConf.FieldScanner("test")
	require.NoError(t, err)

	testutil.ScannerTestSuite(t, rdr, nil, []byte(`a1,b1,c1
a2,b2,c2
a3,b3,c3
a4,b4,c4
`),
		`["a1","b1","c1"]`,
		`["a2","b2","c2"]`,
		`["a3","b3","c3"]`,
		`["a4","b4","c4"]`,
	)
}

func TestCSVScannerContinueOnError(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  csv:
    continue_on_error: true
`, nil)
	require.NoError(t, err)

	creator, err := pConf.FieldScanner("test")
	require.NoError(t, err)

	readErr := errors.New("connection reset by peer")
	rdr := io.MultiReader(
		strings.NewReader("a,b\na1,b1\na2\na3,b3\n"),
		iotest.ErrReader(readErr),
	)

	s, err := creator.Create(io.NopCloser(rdr), func(context.Context, error) error { return nil }, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close(t.Context()) })

	next := func() (service.MessageBatch, error) {
		b, _, err := s.NextBatch(t.Context())
		return b, err
	}

	b, err := next()
	require.NoError(t, err)
	require.Len(t, b, 1)
	require.NoError(t, b[0].GetError())

	// A parse error (wrong field count) is skipped as an errored message.
	b, err = next()
	require.NoError(t, err)
	require.Len(t, b, 1)
	require.ErrorIs(t, b[0].GetError(), csv.ErrFieldCount)

	b, err = next()
	require.NoError(t, err)
	require.Len(t, b, 1)
	require.NoError(t, b[0].GetError())

	// A read error from the source is returned instead of looping forever.
	_, err = next()
	require.ErrorIs(t, err, readErr)
}
