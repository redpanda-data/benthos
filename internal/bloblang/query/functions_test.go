// Copyright 2025 Redpanda Data, Inc.

package query

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/message"
)

func TestFunctions(t *testing.T) {
	type easyMsg struct {
		content string
		meta    map[string]any
	}

	mustFunc := func(name string, args ...any) Function {
		t.Helper()
		fn, err := InitFunctionHelper(name, args...)
		require.NoError(t, err)
		return fn
	}

	mustMethod := func(fn Function, name string, args ...any) Function {
		t.Helper()
		fn, err := InitMethodHelper(name, fn, args...)
		require.NoError(t, err)
		return fn
	}

	tests := map[string]struct {
		input    Function
		output   any
		err      string
		messages []easyMsg
		vars     map[string]any
		index    int
	}{
		"check throw function 1": {
			input: mustFunc("throw", "foo"),
			err:   "foo",
		},
		"check throw function 2": {
			input: mustMethod(
				mustFunc("throw", "foo"),
				"catch", "bar",
			),
			output: "bar",
		},
		"check var function": {
			input: mustMethod(
				mustFunc("var", "foo"),
				"uppercase",
			),
			output: "FOOBAR",
			vars: map[string]any{
				"foo": "foobar",
			},
		},
		"check var function object": {
			input: mustMethod(
				mustMethod(
					mustFunc("var", "foo"),
					"get", "bar",
				),
				"uppercase",
			),
			output: "FOOBAR",
			vars: map[string]any{
				"foo": map[string]any{
					"bar": "foobar",
				},
			},
		},
		"check var function error": {
			input: mustFunc("var", "foo"),
			vars:  map[string]any{},
			err:   `variable 'foo' undefined`,
		},
		"check meta function object": {
			input:  mustFunc("meta", "foo"),
			output: "foobar",
			messages: []easyMsg{
				{content: "", meta: map[string]any{
					"foo": "foobar",
				}},
			},
		},
		"check meta function error": {
			input:  mustFunc("meta", "foo"),
			vars:   map[string]any{},
			output: nil,
		},
		"check metadata function object": {
			input:  mustFunc("meta", "foo"),
			output: "foobar",
			messages: []easyMsg{
				{content: "", meta: map[string]any{
					"foo": "foobar",
				}},
			},
		},
		"check source_metadata function object": {
			input:  mustFunc("meta", "foo"),
			output: "foobar",
			messages: []easyMsg{
				{content: "", meta: map[string]any{
					"foo": "foobar",
				}},
			},
		},
		"check range start > end": {
			input: mustFunc("range", mustFunc("var", "start"), 0, 1),
			vars: map[string]any{
				"start": 10,
			},
			err: `with positive step arg start (10) must be < stop (0)`,
		},
		"check range start >= end": {
			input: mustFunc("range", mustFunc("var", "start"), 10, 1),
			vars: map[string]any{
				"start": 10,
			},
			err: `with positive step arg start (10) must be < stop (10)`,
		},
		"check range zero step": {
			input: mustFunc("range", mustFunc("var", "start"), 100, 0),
			vars: map[string]any{
				"start": 10,
			},
			err: `step must be greater than or less than 0`,
		},
		"check range start < end neg step": {
			input: mustFunc("range", mustFunc("var", "start"), 100, -1),
			vars: map[string]any{
				"start": 10,
			},
			err: `with negative step arg stop (100) must be <= start (10)`,
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			msg := message.QuickBatch(nil)
			for _, m := range test.messages {
				part := message.NewPart([]byte(m.content))
				if m.meta != nil {
					for k, v := range m.meta {
						part.MetaSetMut(k, v)
					}
				}
				msg = append(msg, part)
			}

			for i := 0; i < 10; i++ {
				res, err := test.input.Exec(FunctionContext{
					Vars:     test.vars,
					Maps:     map[string]Function{},
					Index:    test.index,
					MsgBatch: msg,
					NewMeta:  msg.Get(test.index),
				})
				if test.err != "" {
					require.EqualError(t, err, test.err)
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, test.output, res)
			}

			// Ensure nothing changed
			for i, m := range test.messages {
				doc, err := msg.Get(i).AsStructuredMut()
				if err == nil {
					msg.Get(i).SetStructured(doc)
				}
				assert.Equal(t, m.content, string(msg.Get(i).AsBytes()))
			}
		})
	}
}

func TestFunctionTargets(t *testing.T) {
	function := func(name string, args ...any) Function {
		t.Helper()
		fn, err := InitFunctionHelper(name, args...)
		require.NoError(t, err)
		return fn
	}

	tests := []struct {
		input  Function
		output []TargetPath
	}{
		{
			input: function("throw", "foo"),
		},
		{
			input: function("json", "foo.bar.baz"),
			output: []TargetPath{
				NewTargetPath(TargetValue, "foo", "bar", "baz"),
			},
		},
		{
			input: NewFieldFunction("foo.bar.baz"),
			output: []TargetPath{
				NewTargetPath(TargetValue, "foo", "bar", "baz"),
			},
		},
		{
			input: function("meta", "foo"),
			output: []TargetPath{
				NewTargetPath(TargetMetadata, "foo"),
			},
		},
		{
			input: function("var", "foo"),
			output: []TargetPath{
				NewTargetPath(TargetVariable, "foo"),
			},
		},
	}

	for i, test := range tests {
		test := test
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			t.Parallel()

			_, res := test.input.QueryTargets(TargetsContext{
				Maps: map[string]Function{},
			})
			assert.Equal(t, test.output, res)
		})
	}
}

func TestUUIDV7Function(t *testing.T) {
	e, err := InitFunctionHelper("uuid_v7")
	require.NoError(t, err)

	res, err := e.Exec(FunctionContext{})
	require.NoError(t, err)
	require.IsType(t, "", res)
	u7 := uuid.FromStringOrNil(res.(string))
	assert.Equal(t, u7.Version(), byte(7))
}

func TestUUIDV7FunctionAtTime(t *testing.T) {
	ts := time.Now().Add(-5 * time.Hour)
	e, err := InitFunctionHelper("uuid_v7", ts)
	require.NoError(t, err)

	res, err := e.Exec(FunctionContext{})
	require.NoError(t, err)
	require.IsType(t, "", res)
	u7 := uuid.FromStringOrNil(res.(string))
	u7ts, err := uuid.TimestampFromV7(u7)
	require.NoError(t, err)
	actual, err := u7ts.Time()
	require.NoError(t, err)
	assert.Equal(t, ts.Truncate(time.Millisecond), actual, "expected: %s, got: %s", ts, actual)
}

func TestNanoidFunction(t *testing.T) {
	e, err := InitFunctionHelper("nanoid")
	require.NoError(t, err)

	res, err := e.Exec(FunctionContext{})
	require.NoError(t, err)
	assert.NotEmpty(t, res)
}

func TestNanoidFunctionLength(t *testing.T) {
	e, err := InitFunctionHelper("nanoid", int64(54))
	require.NoError(t, err)

	res, err := e.Exec(FunctionContext{})
	require.NoError(t, err)
	assert.Len(t, res, 54)
}

func TestNanoidFunctionAlphabet(t *testing.T) {
	e, err := InitFunctionHelper("nanoid", int64(1), "a")
	require.NoError(t, err)

	res, err := e.Exec(FunctionContext{})
	require.NoError(t, err)
	assert.Equal(t, "a", res)
}

func TestKsuidFunction(t *testing.T) {
	e, err := InitFunctionHelper("ksuid")
	require.NoError(t, err)

	res, err := e.Exec(FunctionContext{})
	require.NoError(t, err)
	assert.NotEmpty(t, res)
}

func TestRandomInt(t *testing.T) {
	e, err := InitFunctionHelper("random_int")
	require.NoError(t, err)

	tallies := map[int64]int64{}

	for i := 0; i < 100; i++ {
		res, err := e.Exec(FunctionContext{})
		require.NoError(t, err)
		require.IsType(t, int64(0), res)
		tallies[res.(int64)]++
	}

	// Can't prove it ain't random, but I can kick up a fuss if something
	// stinks.
	assert.GreaterOrEqual(t, len(tallies), 20)
	for _, v := range tallies {
		assert.LessOrEqual(t, v, int64(10))
	}

	// Create a new random_int function with a different seed
	e, err = InitFunctionHelper("random_int", 10)
	require.NoError(t, err)

	secondTallies := map[int64]int64{}

	for i := 0; i < 100; i++ {
		res, err := e.Exec(FunctionContext{}.WithValue(i))
		require.NoError(t, err)
		require.IsType(t, int64(0), res)
		secondTallies[res.(int64)]++
	}

	assert.NotEqual(t, tallies, secondTallies)
	assert.GreaterOrEqual(t, len(secondTallies), 20)
	for _, v := range secondTallies {
		assert.LessOrEqual(t, v, int64(10))
	}
}

func TestRandomIntDynamic(t *testing.T) {
	idFn := NewFieldFunction("")

	e, err := InitFunctionHelper("random_int", idFn)
	require.NoError(t, err)

	tallies := map[int64]int64{}

	for i := 0; i < 100; i++ {
		res, err := e.Exec(FunctionContext{}.WithValue(i))
		require.NoError(t, err)
		require.IsType(t, int64(0), res)
		tallies[res.(int64)]++
	}

	// Can't prove it ain't random, but I can kick up a fuss if something
	// stinks.
	assert.GreaterOrEqual(t, len(tallies), 20)
	for _, v := range tallies {
		assert.LessOrEqual(t, v, int64(10))
	}

	// Create a new random_int function and feed the same values in
	e, err = InitFunctionHelper("random_int", idFn)
	require.NoError(t, err)

	secondTallies := map[int64]int64{}

	for i := 0; i < 100; i++ {
		res, err := e.Exec(FunctionContext{}.WithValue(i))
		require.NoError(t, err)
		require.IsType(t, int64(0), res)
		secondTallies[res.(int64)]++
	}

	assert.Equal(t, tallies, secondTallies)

	// Create a new random_int function and feed the first value in the same,
	// but following values are different.
	e, err = InitFunctionHelper("random_int", idFn)
	require.NoError(t, err)

	thirdTallies := map[int64]int64{}

	for i := 0; i < 100; i++ {
		input := i
		if input > 0 {
			input += 10
		}
		res, err := e.Exec(FunctionContext{}.WithValue(input))
		require.NoError(t, err)
		require.IsType(t, int64(0), res)
		thirdTallies[res.(int64)]++
	}

	assert.Equal(t, tallies, thirdTallies)
}

func TestRandomIntMilliDynamicParallel(t *testing.T) {
	tsFn, err := InitFunctionHelper("timestamp_unix_milli")
	require.NoError(t, err)

	e, err := InitFunctionHelper("random_int", tsFn)
	require.NoError(t, err)

	startChan := make(chan struct{})
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startChan
			for j := 0; j < 100; j++ {
				res, err := e.Exec(FunctionContext{})
				require.NoError(t, err)
				require.IsType(t, int64(0), res)
			}
		}()
	}

	close(startChan)
	wg.Wait()
}

func TestRandomIntMicroDynamicParallel(t *testing.T) {
	tsFn, err := InitFunctionHelper("timestamp_unix_micro")
	require.NoError(t, err)

	e, err := InitFunctionHelper("random_int", tsFn)
	require.NoError(t, err)

	startChan := make(chan struct{})
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startChan
			for j := 0; j < 100; j++ {
				res, err := e.Exec(FunctionContext{})
				require.NoError(t, err)
				require.IsType(t, int64(0), res)
			}
		}()
	}

	close(startChan)
	wg.Wait()
}

func TestRandomIntDynamicParallel(t *testing.T) {
	tsFn, err := InitFunctionHelper("timestamp_unix_nano")
	require.NoError(t, err)

	e, err := InitFunctionHelper("random_int", tsFn)
	require.NoError(t, err)

	startChan := make(chan struct{})
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startChan
			for j := 0; j < 100; j++ {
				res, err := e.Exec(FunctionContext{})
				require.NoError(t, err)
				require.IsType(t, int64(0), res)
			}
		}()
	}

	close(startChan)
	wg.Wait()
}

func TestRandomIntWithinRange(t *testing.T) {
	tsFn, err := InitFunctionHelper("timestamp_unix_nano")
	require.NoError(t, err)
	var minV, maxV int64 = 10, 20
	e, err := InitFunctionHelper("random_int", tsFn, minV, maxV)
	require.NoError(t, err)

	for i := 0; i < 1000; i++ {
		res, err := e.Exec(FunctionContext{})
		require.NoError(t, err)
		require.IsType(t, int64(0), res)
		assert.GreaterOrEqual(t, res.(int64), minV)
		assert.LessOrEqual(t, res.(int64), maxV)
	}

	// Create a new random_int function with one single possible value
	e, err = InitFunctionHelper("random_int", tsFn, 10, 10)
	require.NoError(t, err)

	for i := 0; i < 1000; i++ {
		res, err := e.Exec(FunctionContext{})
		require.NoError(t, err)
		require.IsType(t, int64(0), res)
		assert.Equal(t, int64(10), res.(int64))
	}

	// Create a new random_int function with an invalid range
	_, err = InitFunctionHelper("random_int", tsFn, 11, 10)
	require.Error(t, err)

	// Create a new random_int function with a negative nin value
	_, err = InitFunctionHelper("random_int", tsFn, -1, 10)
	require.Error(t, err)

	// Create a new random_int function with a max that will overflow
	_, err = InitFunctionHelper("random_int", tsFn, 0, math.MaxInt64)
	require.Error(t, err)
}

func TestErrorFunctions(t *testing.T) {
	tests := []struct {
		name           string
		fn             string
		err            string
		componentName  string
		componentLabel string
		componentPath  []string
		exp            string
	}{
		{
			name: "error() returns the message error if set",
			fn:   "error",
			err:  "kaboom!",
			exp:  "kaboom!",
		},
		{
			name: "error() returns null for non-errored messages",
			fn:   "error",
		},
		{
			name: "errored() returns true for errored messages",
			fn:   "errored",
			err:  "kaboom!",
		},
		{
			name: "errored() returns false for non-errored messages",
			fn:   "errored",
		},
		{
			name:          "error_source_name() returns the component name when the message is errored",
			fn:            "error_source_name",
			err:           "kaboom!",
			componentName: "foobar",
			exp:           "foobar",
		},
		{
			name: "error_source_name() returns null for non-errored messages",
			fn:   "error_source_name",
		},
		{
			name:           "error_source_label() returns the component label when the message is errored",
			fn:             "error_source_label",
			err:            "kaboom!",
			componentLabel: "foobar",
			exp:            "foobar",
		},
		{
			name: "error_source_label() returns null for non-errored messages",
			fn:   "error_source_label",
		},
		{
			name:          "error_source_path() returns the component path when the message is errored",
			fn:            "error_source_path",
			err:           "kaboom!",
			componentPath: []string{"foo", "bar"},
			exp:           "foo.bar",
		},
		{
			name: "error_source_path() returns null for non-errored messages",
			fn:   "error_source_path",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			batch := message.QuickBatch(nil)
			part := message.NewPart([]byte("foobar"))
			if test.err != "" {
				part.ErrorSet(&ComponentError{
					Err:   errors.New(test.err),
					Name:  test.componentName,
					Label: test.componentLabel,
					Path:  test.componentPath,
				})
			}
			batch = append(batch, part)

			e, err := InitFunctionHelper(test.fn)
			require.NoError(t, err)
			res, err := e.Exec(FunctionContext{
				Index:    0,
				MsgBatch: batch,
			})
			require.NoError(t, err)

			if test.fn == "errored" {
				b, ok := res.(bool)
				require.True(t, ok)
				assert.Equal(t, b, test.err != "")
				return
			}

			if test.err == "" {
				assert.Empty(t, res)
				return
			}

			str, ok := res.(string)
			require.True(t, ok)
			assert.Equal(t, test.exp, str)
		})
	}
}
