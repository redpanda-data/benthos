package service

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterpolatedURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expr     string
		msg      *Message
		expected *url.URL
	}{
		{
			name:     "content interpolation",
			expr:     `http://foo.com/${! content() }/bar`,
			msg:      NewMessage([]byte("hello world")),
			expected: mustParseURL(`http://foo.com/hello world/bar`),
		},
		{
			name:     "no interpolation",
			expr:     `https://foo.bar`,
			msg:      NewMessage([]byte("hello world")),
			expected: mustParseURL(`https://foo.bar`),
		},
		{
			name: "metadata interpolation",
			expr: `http://foo.com/${! meta("var1") }/bar`,
			msg: func() *Message {
				m := NewMessage([]byte("hello world"))
				m.MetaSet("var1", "value1")
				return m
			}(),
			expected: mustParseURL("http://foo.com/value1/bar"),
		},
	}

	for _, test := range tests {
		test := test

		t.Run("api/"+test.name, func(t *testing.T) {
			t.Parallel()

			i, err := NewInterpolatedURL(test.expr)
			require.NoError(t, err)

			{
				got, err := i.TryURL(test.msg)
				require.NoError(t, err)

				assert.Equal(t, test.expected, got)
			}
		})
	}
}

func TestInterpolatedURLCtor(t *testing.T) {
	t.Parallel()

	i, err := NewInterpolatedURL(`http://foo.com/${! meta("var1")  bar`)

	assert.EqualError(t, err, "required: expected end of expression, got: bar")
	assert.Nil(t, i)
}

func TestInterpolatedURLMethods(t *testing.T) {
	t.Parallel()

	i, err := NewInterpolatedURL(`http://foo.com/${! meta("var1") + 1 }/bar`)
	require.NoError(t, err)

	m := NewMessage([]byte("hello world"))
	m.MetaSet("var1", "value1")

	{
		got, err := i.TryURL(m)
		require.EqualError(t, err, "cannot add types string (from meta field var1) and number (from number literal)")
		require.Empty(t, got)
	}
}

func mustParseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
