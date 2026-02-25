package pure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

func TestTemplate(t *testing.T) {
	conf, err := testutil.ProcessorFromYAML(`
template:
  code: "{{ .name }}"
`)
	require.NoError(t, err)

	tmpl, err := mock.NewManager().NewProcessor(conf)
	require.NoError(t, err)

	msgIn := message.QuickBatch([][]byte{[]byte(`{"name": "John Doe"}`)})
	msgsOut, err := tmpl.ProcessBatch(t.Context(), msgIn)
	require.NoError(t, err)
	require.Len(t, msgsOut, 1)
	require.Len(t, msgsOut[0], 1)
	assert.Equal(t, "John Doe", string(msgsOut[0][0].AsBytes()))

	type testCase struct {
		name     string
		input    []string
		expected []string
	}

	tests := []testCase{
		{
			name:     "template test 1",
			input:    []string{`{"name": "John Doe"}`},
			expected: []string{`John Doe`},
		},
		{
			name:     "template test 2",
			input:    []string{`{"wrong": "John Doe"}`},
			expected: []string{`<no value>`},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			msg := message.QuickBatch(nil)
			for _, s := range test.input {
				msg = append(msg, message.NewPart([]byte(s)))
			}
			msgs, res := tmpl.ProcessBatch(t.Context(), msg)
			require.NoError(t, res)

			resStrs := []string{}
			for _, b := range message.GetAllBytes(msgs[0]) {
				resStrs = append(resStrs, string(b))
			}
			assert.Equal(t, test.expected, resStrs)
		})
	}
}

func TestTemplateError(t *testing.T) {
	conf, err := testutil.ProcessorFromYAML(`
template:
  missing_key: 'error'
  code: '{{ .name }}'
`)
	require.NoError(t, err)

	tmpl, err := mock.NewManager().NewProcessor(conf)
	require.NoError(t, err)

	msgIn := message.QuickBatch([][]byte{[]byte(`{"wrong": "John Doe"}`)})
	msgsOut, err := tmpl.ProcessBatch(t.Context(), msgIn)
	require.NoError(t, err)
	require.Len(t, msgsOut, 1)
	require.Len(t, msgsOut[0], 1)
	assert.Equal(t, string(msgIn[0].AsBytes()), string(msgsOut[0][0].AsBytes()))
}
