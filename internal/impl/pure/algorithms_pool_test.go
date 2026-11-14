// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/klauspost/compress/gzip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var pooledAlgorithms = []string{"gzip", "zlib", "flate"}

// unpooledCompress mirrors the pre-pooling CompressFunc (fresh writer,
// io.Copy, Close) to serve as a reference output.
func unpooledCompress(t testing.TB, algName string, level int, b []byte) []byte {
	t.Helper()
	out, err := unpooledCompressErr(algName, level, b)
	require.NoError(t, err)
	return out
}

func unpooledCompressErr(algName string, level int, b []byte) ([]byte, error) {
	alg, err := strToCompressAlg(algName)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w, err := alg.CompressWriter(level, &buf)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(w, bytes.NewReader(b)); err != nil {
		return nil, err
	}
	if err := w.(io.Closer).Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func unpooledDecompress(t testing.TB, algName string, b []byte) []byte {
	t.Helper()
	alg, err := strToCompressAlg(algName)
	require.NoError(t, err)
	r, err := alg.DecompressReader(bytes.NewReader(b))
	require.NoError(t, err)
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return out
}

func TestPooledCompressMatchesUnpooled(t *testing.T) {
	body := benchJSONBody(t)
	for _, algName := range pooledAlgorithms {
		fn, err := strToCompressFunc(algName)
		require.NoError(t, err)
		// -3 is klauspost's stateless mode for gzip; levels an algorithm
		// rejects must be rejected by the pooled path too.
		for level := -3; level <= 9; level++ {
			want, wantErr := unpooledCompressErr(algName, level, body)
			for i := range 3 {
				got, err := fn(level, body)
				if wantErr != nil {
					assert.Error(t, err, "alg=%v level=%v call=%v", algName, level, i)
					continue
				}
				require.NoError(t, err)
				assert.Equal(t, want, got, "alg=%v level=%v call=%v", algName, level, i)
			}
		}
	}
}

func TestPooledCompressConcurrentMixedLevels(t *testing.T) {
	base := benchJSONBody(t)
	levels := []int{-1, 1, 5, 9}

	for _, algName := range pooledAlgorithms {
		t.Run(algName, func(t *testing.T) {
			fn, err := strToCompressFunc(algName)
			require.NoError(t, err)

			type job struct {
				level int
				input []byte
				want  []byte
			}
			var jobs []job
			for g := range 8 {
				input := append(bytes.Clone(base), fmt.Sprintf(`{"goroutine":%d}`, g)...)
				for _, level := range levels {
					jobs = append(jobs, job{level, input, unpooledCompress(t, algName, level, input)})
				}
			}

			var wg sync.WaitGroup
			for _, j := range jobs {
				wg.Go(func() {
					for range 50 {
						got, err := fn(j.level, j.input)
						if !assert.NoError(t, err) {
							return
						}
						// Byte equality with a fresh writer at the same level
						// catches level cross-contamination, which a round
						// trip alone would not.
						if !assert.Equal(t, j.want, got, "level %v", j.level) {
							return
						}
						if !assert.Equal(t, j.input, unpooledDecompress(t, algName, got)) {
							return
						}
					}
				})
			}
			wg.Wait()
		})
	}
}

func TestPooledCompressOutputDoesNotAlias(t *testing.T) {
	fn, err := strToCompressFunc("gzip")
	require.NoError(t, err)

	first, err := fn(6, []byte("first payload first payload first payload"))
	require.NoError(t, err)
	firstCopy := bytes.Clone(first)

	for range 10 {
		_, err = fn(6, []byte("a different second payload that would overwrite a shared buffer"))
		require.NoError(t, err)
	}
	assert.Equal(t, firstCopy, first)
}

func TestPooledCompressInvalidLevel(t *testing.T) {
	p := &pooledCompress{ctor: func(level int, w io.Writer) (ResettableWriter, error) {
		return gzip.NewWriterLevel(w, level)
	}}
	for _, level := range []int{42, -42, 1000} {
		_, err := p.compress(level, []byte("hello"))
		require.Error(t, err)
		assert.Nil(t, p.pools.load(level), "invalid level %v must not create a pool", level)
	}

	_, err := p.compress(6, []byte("hello"))
	require.NoError(t, err)
	assert.NotNil(t, p.pools.load(6))
}

func TestPooledCompressLargeMessage(t *testing.T) {
	fn, err := strToCompressFunc("gzip")
	require.NoError(t, err)

	large := make([]byte, 4*maxPooledBufferSize)
	_, _ = rand.Read(large)
	for range 3 {
		out, err := fn(1, large)
		require.NoError(t, err)
		assert.Equal(t, large, unpooledDecompress(t, "gzip", out))
	}

	small := []byte("small payload after a large one")
	out, err := fn(1, small)
	require.NoError(t, err)
	assert.Equal(t, small, unpooledDecompress(t, "gzip", out))
}

func TestTrimPooledBuffer(t *testing.T) {
	var buf bytes.Buffer
	buf.Grow(1024)
	trimPooledBuffer(&buf)
	assert.GreaterOrEqual(t, buf.Cap(), 1024, "small buffers are kept")

	buf.Grow(maxPooledBufferSize + 1)
	trimPooledBuffer(&buf)
	assert.Equal(t, 0, buf.Cap(), "oversized buffers are dropped")
}

func TestLevelPools(t *testing.T) {
	var l levelPools
	for _, level := range []int{levelPoolsMin - 1, levelPoolsMin, -1, 0, 9, levelPoolsMax, levelPoolsMax + 1, 1 << 20} {
		assert.Nil(t, l.load(level), "level %v", level)
		p := l.loadOrCreate(level)
		require.NotNil(t, p)
		assert.Same(t, p, l.load(level), "level %v", level)
		assert.Same(t, p, l.loadOrCreate(level), "level %v", level)
	}
	assert.NotSame(t, l.loadOrCreate(1), l.loadOrCreate(2))
}

var errInjected = errors.New("injected failure")

// faultyWriter wraps a real gzip writer and fails Write or Close for inputs
// with a magic prefix, recording any reuse after a failure.
type faultyWriter struct {
	*gzip.Writer
	poisoned   bool
	failClose  bool
	reusedFail *atomic.Int64
}

func (f *faultyWriter) Reset(w io.Writer) {
	if f.poisoned {
		f.reusedFail.Add(1)
	}
	f.failClose = false
	f.Writer.Reset(w)
}

func (f *faultyWriter) Write(b []byte) (int, error) {
	switch {
	case bytes.HasPrefix(b, []byte("FAILWRITE")):
		f.poisoned = true
		return 0, errInjected
	case bytes.HasPrefix(b, []byte("FAILCLOSE")):
		f.failClose = true
	}
	return f.Writer.Write(b)
}

func (f *faultyWriter) Close() error {
	if f.failClose {
		f.poisoned = true
		_ = f.Writer.Close()
		return errInjected
	}
	return f.Writer.Close()
}

func TestPooledCompressErrorDoesNotPoisonPool(t *testing.T) {
	var reusedFail, constructed atomic.Int64
	fn := newPooledCompressFunc(func(level int, w io.Writer) (ResettableWriter, error) {
		constructed.Add(1)
		gw, err := gzip.NewWriterLevel(w, level)
		if err != nil {
			return nil, err
		}
		return &faultyWriter{Writer: gw, reusedFail: &reusedFail}, nil
	})

	good := benchJSONBody(t)
	var wg sync.WaitGroup
	for g := range 16 {
		wg.Go(func() {
			for i := range 200 {
				var input []byte
				switch (g + i) % 5 {
				case 0:
					input = append([]byte("FAILWRITE"), good...)
				case 1:
					input = append([]byte("FAILCLOSE"), good...)
				default:
					input = good
				}
				out, err := fn(1+(i%3)*4, input)
				if input[0] == 'F' {
					assert.ErrorIs(t, err, errInjected)
					assert.Nil(t, out)
					continue
				}
				if !assert.NoError(t, err) {
					return
				}
				assert.Equal(t, good, unpooledDecompress(t, "gzip", out))
			}
		})
	}
	wg.Wait()

	assert.Zero(t, reusedFail.Load(), "a writer that failed was returned to the pool")
	assert.Less(t, constructed.Load(), int64(16*200), "writers were never reused, the test proves nothing")
}

func TestPooledDecompressConcurrentRoundTrip(t *testing.T) {
	base := benchJSONBody(t)
	for _, algName := range pooledAlgorithms {
		t.Run(algName, func(t *testing.T) {
			dfn, err := strToDecompressFunc(algName)
			require.NoError(t, err)

			type job struct{ want, compressed []byte }
			var jobs []job
			for g := range 8 {
				input := append(bytes.Clone(base), fmt.Sprintf(`{"goroutine":%d}`, g)...)
				for _, level := range []int{1, 5, 9} {
					jobs = append(jobs, job{input, unpooledCompress(t, algName, level, input)})
				}
			}

			var wg sync.WaitGroup
			for _, j := range jobs {
				wg.Go(func() {
					for range 50 {
						got, err := dfn(j.compressed)
						if !assert.NoError(t, err) || !assert.Equal(t, j.want, got) {
							return
						}
					}
				})
			}
			wg.Wait()
		})
	}
}

func TestPooledDecompressOutputDoesNotAlias(t *testing.T) {
	dfn, err := strToDecompressFunc("gzip")
	require.NoError(t, err)

	firstIn := []byte("first payload first payload first payload")
	first, err := dfn(unpooledCompress(t, "gzip", 6, firstIn))
	require.NoError(t, err)

	second := unpooledCompress(t, "gzip", 6, []byte("a different second payload that would overwrite a shared buffer"))
	for range 10 {
		_, err = dfn(second)
		require.NoError(t, err)
	}
	assert.Equal(t, firstIn, first)
}

func TestPooledDecompressLargeMessage(t *testing.T) {
	dfn, err := strToDecompressFunc("gzip")
	require.NoError(t, err)

	large := make([]byte, 4*maxPooledBufferSize)
	_, _ = rand.Read(large)
	compressed := unpooledCompress(t, "gzip", 1, large)
	for range 3 {
		out, err := dfn(compressed)
		require.NoError(t, err)
		assert.Equal(t, large, out)
	}
}

func TestPooledDecompressGzipMultistream(t *testing.T) {
	dfn, err := strToDecompressFunc("gzip")
	require.NoError(t, err)

	stream := append(unpooledCompress(t, "gzip", 6, []byte("hello ")), unpooledCompress(t, "gzip", 6, []byte("world"))...)
	for range 3 {
		out, err := dfn(stream)
		require.NoError(t, err)
		assert.Equal(t, "hello world", string(out))
	}
}

func TestPooledDecompressCorruptInputDoesNotPoisonPool(t *testing.T) {
	good := benchJSONBody(t)
	for _, algName := range pooledAlgorithms {
		t.Run(algName, func(t *testing.T) {
			dfn, err := strToDecompressFunc(algName)
			require.NoError(t, err)

			valid := unpooledCompress(t, algName, 6, good)
			truncated := valid[:len(valid)/2]
			garbage := []byte("this is not a compressed stream at all")

			var wg sync.WaitGroup
			for g := range 16 {
				wg.Go(func() {
					for i := range 200 {
						switch (g + i) % 4 {
						case 0:
							_, err := dfn(truncated)
							assert.Error(t, err)
						case 1:
							_, err := dfn(garbage)
							assert.Error(t, err)
						default:
							out, err := dfn(valid)
							if !assert.NoError(t, err) || !assert.Equal(t, good, out) {
								return
							}
						}
					}
				})
			}
			wg.Wait()
		})
	}
}

// faultyReader wraps a real gzip reader and records reuse after a failure.
type faultyReader struct {
	*gzip.Reader
	poisoned   bool
	reusedFail *atomic.Int64
}

func (f *faultyReader) Reset(r io.Reader) error {
	if f.poisoned {
		f.reusedFail.Add(1)
	}
	err := f.Reader.Reset(r)
	f.poisoned = err != nil
	return err
}

func (f *faultyReader) Read(b []byte) (int, error) {
	n, err := f.Reader.Read(b)
	if err != nil && err != io.EOF {
		f.poisoned = true
	}
	return n, err
}

func TestPooledDecompressErrorDoesNotReuseReader(t *testing.T) {
	var reusedFail, constructed atomic.Int64
	dfn := newPooledDecompressFunc(func(r io.Reader) (ResettableReader, error) {
		constructed.Add(1)
		gr, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		return &faultyReader{Reader: gr, reusedFail: &reusedFail}, nil
	})

	good := benchJSONBody(t)
	valid := unpooledCompress(t, "gzip", 6, good)
	truncated := valid[:len(valid)/2]

	var wg sync.WaitGroup
	for g := range 16 {
		wg.Go(func() {
			for i := range 200 {
				if (g+i)%3 == 0 {
					_, err := dfn(truncated)
					assert.Error(t, err)
					continue
				}
				out, err := dfn(valid)
				if !assert.NoError(t, err) || !assert.Equal(t, good, out) {
					return
				}
			}
		})
	}
	wg.Wait()

	assert.Zero(t, reusedFail.Load(), "a reader that failed was returned to the pool")
	assert.Less(t, constructed.Load(), int64(16*200), "readers were never reused, the test proves nothing")
}

func TestDictResettableReaderRejectsNonResetter(t *testing.T) {
	_, err := newDictResettableReader(io.NopCloser(bytes.NewReader(nil)))
	require.Error(t, err)
}

func TestPooledEmptyPayloadParity(t *testing.T) {
	for _, algName := range pooledAlgorithms {
		cfn, err := strToCompressFunc(algName)
		require.NoError(t, err)
		dfn, err := strToDecompressFunc(algName)
		require.NoError(t, err)

		for range 3 {
			compressed, err := cfn(6, []byte{})
			require.NoError(t, err)
			assert.Equal(t, unpooledCompress(t, algName, 6, []byte{}), compressed, algName)

			out, err := dfn(compressed)
			require.NoError(t, err)
			assert.NotNil(t, out, "%v: io.ReadAll returned a non-nil empty slice before pooling", algName)
			assert.Empty(t, out, algName)
		}
	}
}

func TestPooledCodecsReuse(t *testing.T) {
	const calls = 100
	body := benchJSONBody(t)

	var compressCtors atomic.Int64
	cfn := newPooledCompressFunc(func(level int, w io.Writer) (ResettableWriter, error) {
		compressCtors.Add(1)
		return gzip.NewWriterLevel(w, level)
	})
	var compressed []byte
	for range calls {
		var err error
		compressed, err = cfn(6, body)
		require.NoError(t, err)
	}
	// sync.Pool may drop entries (randomly under -race), so assert reuse
	// happened rather than that it always happens.
	assert.Less(t, compressCtors.Load(), int64(calls), "compressors were never reused")

	var decompressCtors atomic.Int64
	dfn := newPooledDecompressFunc(func(r io.Reader) (ResettableReader, error) {
		decompressCtors.Add(1)
		return gzip.NewReader(r)
	})
	for range calls {
		out, err := dfn(compressed)
		require.NoError(t, err)
		require.Equal(t, body, out)
	}
	assert.Less(t, decompressCtors.Load(), int64(calls), "decompressors were never reused")
}
