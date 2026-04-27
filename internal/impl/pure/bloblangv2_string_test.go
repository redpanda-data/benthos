// Copyright 2026 Redpanda Data, Inc.

package pure_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func runBloblangV2(t *testing.T, mapping string, input any) any {
	t.Helper()
	exec, err := bloblangv2.GlobalEnvironment().Parse(mapping)
	require.NoError(t, err)
	out, err := exec.Query(input)
	require.NoError(t, err)
	return out
}

func TestBloblangV2StringPlugins(t *testing.T) {
	cases := []struct {
		name    string
		mapping string
		input   any
		want    any
	}{
		{
			name:    "capitalize",
			mapping: `output = input.capitalize()`,
			input:   "the foo bar",
			want:    "The Foo Bar",
		},
		{
			name:    "escape_html",
			mapping: `output = input.escape_html()`,
			input:   "foo & bar",
			want:    "foo &amp; bar",
		},
		{
			name:    "unescape_html",
			mapping: `output = input.unescape_html()`,
			input:   "foo &amp; bar",
			want:    "foo & bar",
		},
		{
			name:    "escape_url_query",
			mapping: `output = input.escape_url_query()`,
			input:   "foo & bar",
			want:    "foo+%26+bar",
		},
		{
			name:    "unescape_url_query",
			mapping: `output = input.unescape_url_query()`,
			input:   "foo+%26+bar",
			want:    "foo & bar",
		},
		{
			name:    "quote",
			mapping: `output = input.quote()`,
			input:   "foo\nbar",
			want:    `"foo\nbar"`,
		},
		{
			name:    "unquote",
			mapping: `output = input.unquote()`,
			input:   `"foo\nbar"`,
			want:    "foo\nbar",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runBloblangV2(t, tc.mapping, tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBloblangV2StringPluginsRejectNonString(t *testing.T) {
	// V2 typed wrappers are strict — non-string receivers should error
	// rather than be silently coerced. Mirrors the public/bloblangv2
	// StringMethod contract documented on its godoc.
	_, err := bloblangv2.GlobalEnvironment().Parse(`output = input.capitalize()`)
	require.NoError(t, err)
	exec, _ := bloblangv2.GlobalEnvironment().Parse(`output = input.capitalize()`)
	_, err = exec.Query(int64(42))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "string")
}
