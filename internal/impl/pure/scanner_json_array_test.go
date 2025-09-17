// Copyright 2025 Redpanda Data, Inc.

package pure_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/scanner/testutil"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func TestJSONArrayScannerDefault(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  json_array: {}
`, nil)
	require.NoError(t, err)

	rdr, err := pConf.FieldScanner("test")
	require.NoError(t, err)

	testutil.ScannerTestSuite(t, rdr, nil, []byte(`[
{"a":"a0"},
{"a":"a1"},
{"a":"a2"},
{"a":"a3"},
{"a":"a4"}
]`),
		`{"a":"a0"}`,
		`{"a":"a1"}`,
		`{"a":"a2"}`,
		`{"a":"a3"}`,
		`{"a":"a4"}`,
	)
}

func TestJSONArrayScannerBadData(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  json_array: {}
`, nil)
	require.NoError(t, err)

	rdr, err := pConf.FieldScanner("test")
	require.NoError(t, err)

	var ack error

	scanner, err := rdr.Create(io.NopCloser(strings.NewReader(`[
{"a":"a0"},
nope !@ not good json
{"a":"a1"}
]`)), func(ctx context.Context, err error) error {
		ack = err
		return nil
	}, &service.ScannerSourceDetails{})
	require.NoError(t, err)

	resBatch, aFn, err := scanner.NextBatch(t.Context())
	require.NoError(t, err)
	require.NoError(t, aFn(t.Context(), nil))
	require.Len(t, resBatch, 1)
	mBytes, err := resBatch[0].AsBytes()
	require.NoError(t, err)
	assert.Equal(t, `{"a":"a0"}`, string(mBytes))

	_, _, err = scanner.NextBatch(t.Context())
	assert.Error(t, err)

	_, _, err = scanner.NextBatch(t.Context())
	assert.ErrorIs(t, err, io.EOF)

	assert.ErrorContains(t, ack, "invalid character")
}

func TestJSONArrayScannerFormatted(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  json_array: {}
`, nil)
	require.NoError(t, err)

	rdr, err := pConf.FieldScanner("test")
	require.NoError(t, err)

	testutil.ScannerTestSuite(t, rdr, nil, []byte(`[
{
	"a":"a0"
},
{
	"a":"a1"
},
{
	"a":"a2"
},
{
	"a":"a3"
},
{
	"a":"a4"
}
]
`),
		`{"a":"a0"}`,
		`{"a":"a1"}`,
		`{"a":"a2"}`,
		`{"a":"a3"}`,
		`{"a":"a4"}`,
	)
}

func TestJSONArrayScannerMultipleArrays(t *testing.T) {
	confSpec := service.NewConfigSpec().Field(service.NewScannerField("test"))
	pConf, err := confSpec.ParseYAML(`
test:
  json_array: {}
`, nil)
	require.NoError(t, err)

	rdr, err := pConf.FieldScanner("test")
	require.NoError(t, err)

	testutil.ScannerTestSuite(t, rdr, nil, []byte(`[
{"a":"a0"},
{"a":"a1"}
]
[
{"a":"a2"},
{"a":"a3"},
{"a":"a4"}
]`),
		`{"a":"a0"}`,
		`{"a":"a1"}`,
		`{"a":"a2"}`,
		`{"a":"a3"}`,
		`{"a":"a4"}`,
	)
}
