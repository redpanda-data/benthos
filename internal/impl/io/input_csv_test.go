// Copyright 2025 Redpanda Data, Inc.

package io

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"
)

func TestCSVReaderHappy(t *testing.T) {
	var handle bytes.Buffer

	for _, msg := range []string{
		"header1,header2,header3",
		"foo1,foo2,foo3",
		"bar1,bar2,bar3",
		"baz1,baz2,baz3",
	} {
		handle.WriteString(msg)
		handle.WriteString("\n")
	}

	dummyFile := "foo/bar.csv"
	dummyTimeUTC := time.Now().UTC()
	ctored := false
	f, err := newCSVReader(
		func(ctx context.Context) (csvScannerInfo, error) {
			if ctored {
				return csvScannerInfo{}, io.EOF
			}
			ctored = true
			return csvScannerInfo{
				handle:      &handle,
				currentPath: dummyFile,
				modTimeUTC:  dummyTimeUTC,
			}, nil
		},
		func(ctx context.Context) {},
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx, done := context.WithTimeout(t.Context(), time.Second*30)
		require.NoError(t, f.Close(ctx))
		done()
	})

	require.NoError(t, f.Connect(t.Context()))

	for _, exp := range []string{
		`{"header1":"foo1","header2":"foo2","header3":"foo3"}`,
		`{"header1":"bar1","header2":"bar2","header3":"bar3"}`,
		`{"header1":"baz1","header2":"baz2","header3":"baz3"}`,
	} {
		var resMsg service.MessageBatch
		resMsg, _, err = f.ReadBatch(t.Context())
		require.NoError(t, err)

		msgBytes, err := resMsg[0].AsBytes()
		require.NoError(t, err)
		assert.Equal(t, exp, string(msgBytes))

		m, _ := resMsg[0].MetaGet("path")
		assert.Equal(t, dummyFile, m)
		m, _ = resMsg[0].MetaGet("mod_time")
		assert.Equal(t, dummyTimeUTC.Format(time.RFC3339), m)
		m, _ = resMsg[0].MetaGet("mod_time_unix")
		assert.Equal(t, strconv.Itoa(int(dummyTimeUTC.Unix())), m)
	}

	_, _, err = f.ReadBatch(t.Context())
	assert.Equal(t, service.ErrEndOfInput, err)
}

func TestCSVReaderGroupCount(t *testing.T) {
	var handle bytes.Buffer

	for _, msg := range []string{
		"foo,bar,baz",
		"foo1,bar1,baz1",
		"foo2,bar2,baz2",
		"foo3,bar3,baz3",
		"foo4,bar4,baz4",
		"foo5,bar5,baz5",
		"foo6,bar6,baz6",
		"foo7,bar7,baz7",
	} {
		handle.WriteString(msg)
		handle.WriteString("\n")
	}

	ctored := false
	f, err := newCSVReader(
		func(ctx context.Context) (csvScannerInfo, error) {
			if ctored {
				return csvScannerInfo{}, io.EOF
			}
			ctored = true
			return csvScannerInfo{handle: &handle}, nil
		},
		func(ctx context.Context) {},
		optCSVSetGroupCount(3),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx, done := context.WithTimeout(t.Context(), time.Second*30)
		require.NoError(t, f.Close(ctx))
		done()
	})

	require.NoError(t, f.Connect(t.Context()))

	for _, exp := range [][]string{
		{
			`{"bar":"bar1","baz":"baz1","foo":"foo1"}`,
			`{"bar":"bar2","baz":"baz2","foo":"foo2"}`,
			`{"bar":"bar3","baz":"baz3","foo":"foo3"}`,
		},
		{
			`{"bar":"bar4","baz":"baz4","foo":"foo4"}`,
			`{"bar":"bar5","baz":"baz5","foo":"foo5"}`,
			`{"bar":"bar6","baz":"baz6","foo":"foo6"}`,
		},
		{
			`{"bar":"bar7","baz":"baz7","foo":"foo7"}`,
		},
	} {
		var resMsg service.MessageBatch
		resMsg, _, err = f.ReadBatch(t.Context())
		require.NoError(t, err)

		require.Len(t, resMsg, len(exp))
		for i := range exp {
			mBytes, err := resMsg[i].AsBytes()
			require.NoError(t, err)
			assert.Equal(t, exp[i], string(mBytes))
		}
	}

	_, _, err = f.ReadBatch(t.Context())
	assert.Equal(t, service.ErrEndOfInput, err)
}

func TestCSVReadersTwoFiles(t *testing.T) {
	var handleOne, handleTwo bytes.Buffer

	for _, msg := range []string{
		"header1,header2,header3",
		"foo1,foo2,foo3",
		"bar1,bar2,bar3",
		"baz1,baz2,baz3",
	} {
		handleOne.WriteString(msg)
		handleOne.WriteString("\n")
	}

	for _, msg := range []string{
		"header4,header5,header6",
		"foo1,foo2,foo3",
		"bar1,bar2,bar3",
		"baz1,baz2,baz3",
	} {
		handleTwo.WriteString(msg)
		handleTwo.WriteString("\n")
	}

	consumedFirst, consumedSecond := false, false

	f, err := newCSVReader(
		func(ctx context.Context) (csvScannerInfo, error) {
			if !consumedFirst {
				consumedFirst = true
				return csvScannerInfo{handle: &handleOne}, nil
			} else if !consumedSecond {
				consumedSecond = true
				return csvScannerInfo{handle: &handleTwo}, nil
			}
			return csvScannerInfo{}, io.EOF
		},
		func(ctx context.Context) {},
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx, done := context.WithTimeout(t.Context(), time.Second*30)
		require.NoError(t, f.Close(ctx))
		done()
	})

	require.NoError(t, f.Connect(t.Context()))

	for i, exp := range []string{
		`{"header1":"foo1","header2":"foo2","header3":"foo3"}`,
		`{"header1":"bar1","header2":"bar2","header3":"bar3"}`,
		`{"header1":"baz1","header2":"baz2","header3":"baz3"}`,
		`{"header4":"foo1","header5":"foo2","header6":"foo3"}`,
		`{"header4":"bar1","header5":"bar2","header6":"bar3"}`,
		`{"header4":"baz1","header5":"baz2","header6":"baz3"}`,
	} {
		var resMsg service.MessageBatch
		var ackFn service.AckFunc
		resMsg, ackFn, err = f.ReadBatch(t.Context())
		require.NoError(t, err, i)

		mBytes, err := resMsg[0].AsBytes()
		require.NoError(t, err)
		assert.Equal(t, exp, string(mBytes), i)
		_ = ackFn(t.Context(), nil)
	}

	_, _, err = f.ReadBatch(t.Context())
	assert.Equal(t, service.ErrEndOfInput, err)
}

func TestCSVReaderCustomComma(t *testing.T) {
	var handle bytes.Buffer

	for _, msg := range []string{
		"header1|header2|header3",
		"foo1|foo2|foo3",
		"bar1|bar2|bar3",
		"baz1|baz2|baz3",
	} {
		handle.WriteString(msg)
		handle.WriteString("\n")
	}

	ctored := false
	f, err := newCSVReader(
		func(ctx context.Context) (csvScannerInfo, error) {
			if ctored {
				return csvScannerInfo{}, io.EOF
			}
			ctored = true
			return csvScannerInfo{handle: &handle}, nil
		},
		func(ctx context.Context) {},
		optCSVSetComma('|'),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx, done := context.WithTimeout(t.Context(), time.Second*30)
		require.NoError(t, f.Close(ctx))
		done()
	})

	require.NoError(t, f.Connect(t.Context()))

	for _, exp := range []string{
		`{"header1":"foo1","header2":"foo2","header3":"foo3"}`,
		`{"header1":"bar1","header2":"bar2","header3":"bar3"}`,
		`{"header1":"baz1","header2":"baz2","header3":"baz3"}`,
	} {
		var resMsg service.MessageBatch
		resMsg, _, err = f.ReadBatch(t.Context())
		require.NoError(t, err)

		mBytes, err := resMsg[0].AsBytes()
		require.NoError(t, err)

		assert.Equal(t, exp, string(mBytes))
	}

	_, _, err = f.ReadBatch(t.Context())
	assert.Equal(t, service.ErrEndOfInput, err)
}

func TestCSVReaderRelaxed(t *testing.T) {
	var handle bytes.Buffer

	for _, msg := range []string{
		"header1,header2,header3",
		"foo1,foo2,foo3",
		"bar1,bar2,bar3,bar4",
		"baz1,baz2,baz3",
		"buz1,buz2",
	} {
		handle.WriteString(msg)
		handle.WriteString("\n")
	}

	ctored := false
	f, err := newCSVReader(
		func(ctx context.Context) (csvScannerInfo, error) {
			if ctored {
				return csvScannerInfo{}, io.EOF
			}
			ctored = true
			return csvScannerInfo{handle: &handle}, nil
		},
		func(ctx context.Context) {},
		optCSVSetStrict(false),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx, done := context.WithTimeout(t.Context(), time.Second*30)
		require.NoError(t, f.Close(ctx))
		done()
	})

	require.NoError(t, f.Connect(t.Context()))

	for _, exp := range []string{
		`{"header1":"foo1","header2":"foo2","header3":"foo3"}`,
		`["bar1","bar2","bar3","bar4"]`,
		`{"header1":"baz1","header2":"baz2","header3":"baz3"}`,
		`{"header1":"buz1","header2":"buz2"}`,
	} {
		var resMsg service.MessageBatch
		resMsg, _, err = f.ReadBatch(t.Context())
		require.NoError(t, err)

		mBytes, err := resMsg[0].AsBytes()
		require.NoError(t, err)

		assert.Equal(t, exp, string(mBytes))
	}

	_, _, err = f.ReadBatch(t.Context())
	assert.Equal(t, service.ErrEndOfInput, err)
}

func TestCSVReaderStrict(t *testing.T) {
	var handle bytes.Buffer

	for _, msg := range []string{
		"header1,header2,header3",
		"foo1,foo2,foo3",
		"bar1,bar2,bar3,bar4",
		"baz1,baz2,baz3",
		"buz1,buz2",
	} {
		handle.WriteString(msg)
		handle.WriteString("\n")
	}

	ctored := false
	f, err := newCSVReader(
		func(ctx context.Context) (csvScannerInfo, error) {
			if ctored {
				return csvScannerInfo{}, io.EOF
			}
			ctored = true
			return csvScannerInfo{handle: &handle}, nil
		},
		func(ctx context.Context) {},
		optCSVSetStrict(true),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx, done := context.WithTimeout(t.Context(), time.Second*30)
		require.NoError(t, f.Close(ctx))
		done()
	})

	require.NoError(t, f.Connect(t.Context()))

	for _, exp := range []any{
		`{"header1":"foo1","header2":"foo2","header3":"foo3"}`,
		errors.New("record on line 3: wrong number of fields"),
		`{"header1":"baz1","header2":"baz2","header3":"baz3"}`,
		errors.New("record on line 5: wrong number of fields"),
	} {
		var resMsg service.MessageBatch
		resMsg, _, err = f.ReadBatch(t.Context())

		switch expT := exp.(type) {
		case string:
			require.NoError(t, err)

			mBytes, err := resMsg[0].AsBytes()
			require.NoError(t, err)

			assert.Equal(t, expT, string(mBytes))

		case error:
			assert.EqualError(t, err, expT.Error())
		}
	}

	_, _, err = f.ReadBatch(t.Context())
	assert.Equal(t, service.ErrEndOfInput, err)
}

func TestCSVReaderLazyQuotes(t *testing.T) {
	tests := []struct {
		name        string
		lazyQuotes  bool
		input       string
		expected    string
		errContains string
	}{
		{
			name:       "quotes in unquoted field w/ LazyQuotes = true",
			input:      `f"oo"1,f"oo"2,f"oo"3`,
			expected:   `["f\"oo\"1","f\"oo\"2","f\"oo\"3"]`,
			lazyQuotes: true,
		},
		{
			name:        "quotes in unquoted field w/ LazyQuotes = false",
			input:       `f"oo"1,f"oo"2,f"oo"3`,
			errContains: `bare " in non-quoted-field`,
			lazyQuotes:  false,
		},
		{
			name:       "non-doubled quote in quoted field w/ LazyQuotes = true",
			input:      `"f"oo1","f"oo2","f"oo3"`,
			expected:   `["f\"oo1","f\"oo2","f\"oo3"]`,
			lazyQuotes: true,
		},
		{
			name:        "non-doubled quote in quoted field w/ LazyQuotes = false",
			input:       `f"oo1,"f'oo'2","f'oo'3"`,
			errContains: `bare " in non-quoted-field`,
			lazyQuotes:  false,
		},
		{
			name:       "quotes in unquoted field AND non-doubled quote in quoted field w/ LazyQuotes = true",
			input:      `f"oo"1,"f"oo2",f"oo"3`,
			expected:   `[\"f"oo"1\","f"oo2",\"f"oo"3\"]`,
			lazyQuotes: true,
		},
		{
			name:        "quotes in unquoted field AND non-doubled quote in quoted field w/ LazyQuotes = false",
			input:       `f"oo"1,"f"oo2",f"oo"3`,
			errContains: `bare " in non-quoted-field`,
			lazyQuotes:  false,
		},
	}
	for _, test := range tests {
		var handle bytes.Buffer

		handle.WriteString(test.input)

		f, err := newCSVReader(
			func(ctx context.Context) (csvScannerInfo, error) {
				return csvScannerInfo{handle: &handle}, nil
			},
			func(ctx context.Context) {},
			optCSVSetExpectHeader(false),
			optCSVSetLazyQuotes(test.lazyQuotes),
		)
		require.NoError(t, err, test.name)
		t.Cleanup(func() {
			ctx, done := context.WithTimeout(t.Context(), time.Second*30)
			require.NoError(t, f.Close(ctx))
			done()
		})

		require.NoError(t, f.Connect(t.Context()), test.name)

		resMsg, _, err := f.ReadBatch(t.Context())
		if test.errContains != "" {
			require.Contains(t, err.Error(), test.errContains, test.name)
			return
		}
		require.NoError(t, err, test.name)

		mBytes, err := resMsg[0].AsBytes()
		require.NoError(t, err)

		assert.Equal(t, test.expected, string(mBytes), test.name)
	}
}

// csvHandleCtor returns a handle constructor that serves the given readers in
// order, then returns io.EOF.
func csvHandleCtor(handles ...io.Reader) func(context.Context) (csvScannerInfo, error) {
	i := 0
	return func(context.Context) (csvScannerInfo, error) {
		if i >= len(handles) {
			return csvScannerInfo{}, io.EOF
		}
		info := csvScannerInfo{
			handle:      handles[i],
			currentPath: "file" + strconv.Itoa(i) + ".csv",
		}
		i++
		return info, nil
	}
}

func csvReadAll(t *testing.T, f *csvReader) [][]string {
	t.Helper()

	var batches [][]string
	for {
		resMsg, ackFn, err := f.ReadBatch(t.Context())
		if errors.Is(err, service.ErrEndOfInput) {
			return batches
		}
		require.NoError(t, err)
		require.NotEmpty(t, resMsg, "ReadBatch must not return an empty batch")

		var batch []string
		for _, m := range resMsg {
			mBytes, err := m.AsBytes()
			require.NoError(t, err)
			batch = append(batch, string(mBytes))
		}
		batches = append(batches, batch)
		require.NoError(t, ackFn(t.Context(), nil))
	}
}

func TestCSVReaderGroupCountDoesNotSpanFiles(t *testing.T) {
	handleOne := bytes.NewBufferString("a,b\n1,2\n3,4\n5,6\n")
	handleTwo := bytes.NewBufferString("c,d\n7,8\n")

	f, err := newCSVReader(
		csvHandleCtor(handleOne, handleTwo),
		func(ctx context.Context) {},
		optCSVSetGroupCount(2),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close(context.Background())) })

	require.NoError(t, f.Connect(t.Context()))

	assert.Equal(t, [][]string{
		{`{"a":"1","b":"2"}`, `{"a":"3","b":"4"}`},
		// The first file ends here, so this batch is short instead of
		// pulling the first record of the second file.
		{`{"a":"5","b":"6"}`},
		{`{"c":"7","d":"8"}`},
	}, csvReadAll(t, f))
}

func TestCSVReaderSkipsEmptyFile(t *testing.T) {
	handleOne := bytes.NewBufferString("a,b\n1,2\n")
	handleEmpty := &bytes.Buffer{}
	handleThree := bytes.NewBufferString("c,d\n3,4\n")

	f, err := newCSVReader(
		csvHandleCtor(handleOne, handleEmpty, handleThree),
		func(ctx context.Context) {},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close(context.Background())) })

	require.NoError(t, f.Connect(t.Context()))

	assert.Equal(t, [][]string{
		{`{"a":"1","b":"2"}`},
		{`{"c":"3","d":"4"}`},
	}, csvReadAll(t, f))
}

func TestCSVReaderSkipsHeaderOnlyFile(t *testing.T) {
	handleOne := bytes.NewBufferString("a,b\n1,2\n")
	handleHeaderOnly := bytes.NewBufferString("x,y\n")
	handleThree := bytes.NewBufferString("c,d\n3,4\n")
	handleLast := bytes.NewBufferString("z\n")

	f, err := newCSVReader(
		csvHandleCtor(handleOne, handleHeaderOnly, handleThree, handleLast),
		func(ctx context.Context) {},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close(context.Background())) })

	require.NoError(t, f.Connect(t.Context()))

	// The header-only file in the middle is skipped, and the header-only
	// file at the end yields ErrEndOfInput rather than an empty batch.
	assert.Equal(t, [][]string{
		{`{"a":"1","b":"2"}`},
		{`{"c":"3","d":"4"}`},
	}, csvReadAll(t, f))
}

func TestCSVReaderContextCancelledMidRotation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	const (
		totalFiles   = 10
		cancelAtFile = 3
	)

	var calls int
	handleCtor := func(context.Context) (csvScannerInfo, error) {
		calls++
		if calls > totalFiles {
			// caps the loop in case the code were to stop respecting ctx cancellation
			return csvScannerInfo{}, io.EOF
		}
		if calls == cancelAtFile {
			// Cancel partway through rotation, as if the pipeline were
			// shutting down while working through a long run of empty files.
			cancel()
		}
		return csvScannerInfo{handle: &bytes.Buffer{}}, nil
	}

	f, err := newCSVReader(handleCtor, func(context.Context) {})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close(context.Background())) })

	require.NoError(t, f.Connect(ctx))

	_, _, err = f.ReadBatch(ctx)
	assert.ErrorIs(t, err, context.Canceled)

	// Rotation must stop as soon as cancellation is observed, not run
	// through the rest of the remaining empty files first.
	assert.Equal(t, cancelAtFile, calls)
}
