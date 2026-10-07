// Copyright 2026 Redpanda Data, Inc.

package service

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestURLListField(t *testing.T) {
	mustURL := func(s string) *url.URL {
		t.Helper()
		u, err := url.Parse(s)
		require.NoError(t, err)
		return u
	}

	tests := []struct {
		name        string
		input       string
		output      []*url.URL
		errContains string
	}{
		{
			name:   "no urls",
			input:  `[]`,
			output: []*url.URL{},
		},
		{
			name:  "one valid url",
			input: `- https://example.com`,
			output: []*url.URL{
				mustURL("https://example.com"),
			},
		},
		{
			name: "multiple urls",
			input: `
- https://example.com/foo
- https://example.com/bar
`,
			output: []*url.URL{
				mustURL("https://example.com/foo"),
				mustURL("https://example.com/bar"),
			},
		},
		{
			name: "multiple urls some csvs",
			input: `
- https://example.com/foo,https://example.com/bar
- https://example.com/baz
- ",https://example.com/buz,"
`,
			output: []*url.URL{
				mustURL("https://example.com/foo"),
				mustURL("https://example.com/bar"),
				mustURL("https://example.com/baz"),
				mustURL("https://example.com/buz"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := NewConfigSpec().Field(NewStringListField(""))

			parsed, err := spec.ParseYAML(test.input, nil)
			require.NoError(t, err)

			us, err := parsed.FieldURLList()
			if test.errContains == "" {
				require.NoError(t, err)
				assert.Equal(t, test.output, us)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.errContains)
			}
		})
	}
}

func TestURLFieldsRedactCredentials(t *testing.T) {
	const pass = "S3cretPass"

	spec := NewConfigSpec().
		Field(NewURLField("a")).
		Field(NewURLListField("b"))

	conf, err := spec.ParseYAML(`
a: 'http://u:`+pass+`@bad host:8080'
b: [ 'http://u:`+pass+`1@good:8080,http://u:`+pass+`2@bad host:8080' ]
`, nil)
	require.NoError(t, err)

	_, err = conf.FieldURL("a")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), pass)
	assert.Contains(t, err.Error(), "failed to parse url field 'a'")

	_, err = conf.FieldURLList("b")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), pass)
}
