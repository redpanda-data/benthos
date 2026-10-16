// Copyright 2025 Redpanda Data, Inc.

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

// unpooledCompress compresses with a freshly constructed writer, as the
// pre-pooling CompressFunc did, to serve as a reference output.
func unpooledCompress(t testing.TB, algName string, level int, b []byte) []byte {
	t.Helper()
	alg, err := strToCompressAlg(algName)
	require.NoError(t, err)
	var buf bytes.Buffer
	w, err := alg.CompressWriter(level, &buf)
	require.NoError(t, err)
	_, err = w.Write(b)
	require.NoError(t, err)
	require.NoError(t, w.(io.Closer).Close())
	return buf.Bytes()
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
		for level := -2; level <= 9; level++ {
			want := unpooledCompress(t, algName, level, body)
			for i := range 3 {
				got, err := fn(level, body)
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

func TestPooledCompressorTrimBuffer(t *testing.T) {
	pc := &pooledCompressor{}
	pc.buf.Grow(1024)
	pc.trimBuffer()
	assert.GreaterOrEqual(t, pc.buf.Cap(), 1024, "small buffers are kept")

	pc.buf.Grow(maxPooledBufferSize + 1)
	pc.trimBuffer()
	assert.Equal(t, 0, pc.buf.Cap(), "oversized buffers are dropped")
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
