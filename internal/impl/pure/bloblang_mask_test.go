package pure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/value"
)

func TestMask(t *testing.T) {
	testCases := []struct {
		name   string
		method string
		target any
		args   []any
		exp    any
		err    string
	}{
		{
			name:   "default fixed string",
			method: "mask",
			target: "this is a test",
			args:   []any{},
			exp:    "**************",
			err:    "",
		},
		{
			name:   "default fixed string length 5",
			method: "mask",
			target: "this is a test",
			args:   []any{int64(5)},
			exp:    "*****",
			err:    "",
		},
		{
			name:   "Mask left leave left hand four chars unmasked",
			method: "mask",
			target: "this is a test",
			args:   []any{int64(4), "left"},
			exp:    "this**********",
			err:    "",
		},
		{
			name:   "Mask left leave right hand four chars unmasked",
			method: "mask",
			target: "this is a test",
			args:   []any{int64(6), "right"},
			exp:    "********a test",
			err:    "",
		},
		{
			name:   "Mask left leave right hand four chars unmasked, mask with '%' char",
			method: "mask",
			target: "this is a test",
			args:   []any{int64(6), "right", "%"},
			exp:    "%%%%%%%%a test",
			err:    "",
		},
		{
			name:   "invalid direction",
			method: "mask",
			target: "this is a test",
			args:   []any{int64(5), "Fred", "*"},
			exp:    nil,
			err:    "direction must be one of left, right or all",
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			targetClone := value.IClone(test.target)
			argsClone := value.IClone(test.args).([]any)

			fn, err := query.InitMethodHelper(test.method, query.NewLiteralFunction("", targetClone), argsClone...)

			if test.err != "" {
				require.Error(t, err)
				assert.EqualError(t, err, test.err)
				return
			}

			require.NoError(t, err)

			res, err := fn.Exec(query.FunctionContext{
				Maps:     map[string]query.Function{},
				Index:    0,
				MsgBatch: nil,
			})
			require.NoError(t, err)

			assert.Equal(t, test.exp, res)
			assert.Equal(t, test.target, targetClone)
			assert.Equal(t, test.args, argsClone)
		})
	}
}
