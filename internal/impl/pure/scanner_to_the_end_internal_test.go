// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shortReader wraps a reader and caps every Read at max bytes, emulating the
// short reads returned by real files and sockets. A bytes.Reader satisfies the
// whole request in a single call and so never exercises the refill path.
type shortReader struct {
	r   io.Reader
	max int
}

func (s *shortReader) Read(p []byte) (int, error) {
	if len(p) > s.max {
		p = p[:s.max]
	}
	return s.r.Read(p)
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	// Fixed seed, so a failure reproduces exactly.
	rnd := rand.New(rand.NewSource(1))
	_, _ = rnd.Read(b)
	return b
}

// TestReadAllHintedMatchesReadAll asserts that readAllHinted is a drop in
// replacement for io.ReadAll for every hint value, correct or otherwise. The
// hint is an optimisation and must never be a correctness input, as a file can
// be appended to or truncated between Stat and read.
func TestReadAllHintedMatchesReadAll(t *testing.T) {
	sizes := []int{0, 1, 511, 512, 513, 4096, 1 << 20}

	for _, size := range sizes {
		content := randomBytes(size)

		hints := []int64{
			0,
			1,
			int64(size / 2),
			int64(size - 1),
			int64(size),
			int64(size + 1),
			int64(size * 2),
			-5,
		}

		for _, hint := range hints {
			for _, chunk := range []int{1, 7, 512, 1 << 16} {
				t.Run(fmt.Sprintf("size=%v/hint=%v/chunk=%v", size, hint, chunk), func(t *testing.T) {
					exp, err := io.ReadAll(&shortReader{r: bytes.NewReader(content), max: chunk})
					require.NoError(t, err)

					act, err := readAllHinted(&shortReader{r: bytes.NewReader(content), max: chunk}, hint)
					require.NoError(t, err)

					assert.Equal(t, exp, act)
					assert.Len(t, act, size)
				})
			}
		}
	}
}

// TestReadAllHintedPropagatesErrors asserts that a non-EOF error is returned
// rather than being swallowed, matching io.ReadAll.
func TestReadAllHintedPropagatesErrors(t *testing.T) {
	expErr := fmt.Errorf("nope")

	_, err := readAllHinted(io.MultiReader(
		bytes.NewReader([]byte("partial")),
		&errReader{err: expErr},
	), 1024)
	require.ErrorIs(t, err, expErr)
}

type errReader struct{ err error }

func (e *errReader) Read([]byte) (int, error) { return 0, e.err }

// TestReadAllHintedExactHintDoesNotRealloc asserts that an accurate hint at or
// above the default floor results in exactly one allocation, by checking the
// returned buffer still has the capacity it was created with. This is the test
// that catches a regression of the +1 subtlety: sizing the buffer to exactly the
// content length would find len == cap on the final iteration and grow once more.
func TestReadAllHintedExactHintDoesNotRealloc(t *testing.T) {
	// Sizes are >= defaultReadAllCap so the floor does not apply and the exact
	// hint is honoured; the sub-floor case is covered by TestReadAllHintedFloor.
	for _, size := range []int{defaultReadAllCap, 4096, 1 << 20} {
		t.Run(fmt.Sprintf("size=%v", size), func(t *testing.T) {
			content := randomBytes(size)

			act, err := readAllHinted(&shortReader{r: bytes.NewReader(content), max: 512}, int64(size))
			require.NoError(t, err)

			assert.Equal(t, content, act)
			assert.Len(t, act, size)
			assert.Equal(t, size+1, cap(act), "buffer was reallocated despite an exact hint")
		})
	}
}

// TestHintedCap asserts the hint is floored at the io.ReadAll default and capped
// so the +1 never overflows to a negative capacity. The overflow branch cannot
// be exercised through readAllHinted (allocating a MaxInt64 buffer would fail),
// so the clamping is unit tested in isolation here.
func TestHintedCap(t *testing.T) {
	for _, test := range []struct {
		name string
		hint int64
		exp  int64
	}{
		{"negative is floored", -5, defaultReadAllCap + 1},
		{"zero is floored", 0, defaultReadAllCap + 1},
		{"below default is floored", 100, defaultReadAllCap + 1},
		{"exactly default", defaultReadAllCap, defaultReadAllCap + 1},
		{"above default is honoured", 4096, 4097},
		{"max int64 does not overflow", math.MaxInt64, math.MaxInt64},
		{"max int64 minus one does not overflow", math.MaxInt64 - 1, math.MaxInt64},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := hintedCap(test.hint)
			assert.Equal(t, test.exp, got)
			assert.Positive(t, got, "capacity must stay positive (no overflow)")
		})
	}
}

// TestReadAllHintedFloor asserts that a small, absent, or negative hint still
// allocates the io.ReadAll-sized buffer rather than growing up from a single
// byte, so no-hint callers (sockets, stdin, etc.) keep io.ReadAll's profile.
func TestReadAllHintedFloor(t *testing.T) {
	const contentLen = 10 // < defaultReadAllCap, so it fits without any growth
	for _, hint := range []int64{-5, 0, 1, 100, defaultReadAllCap} {
		t.Run(fmt.Sprintf("hint=%v", hint), func(t *testing.T) {
			content := randomBytes(contentLen)

			act, err := readAllHinted(&shortReader{r: bytes.NewReader(content), max: 4}, hint)
			require.NoError(t, err)

			assert.Equal(t, content, act)
			assert.Equal(t, defaultReadAllCap+1, cap(act),
				"sub-floor hint should allocate the default-sized buffer")
		})
	}
}

// TestReadAllHintedGrowsWhenHintTooSmall asserts the buffer still grows to fit
// content larger than the hint, the case where a file is appended to between
// Stat and read.
func TestReadAllHintedGrowsWhenHintTooSmall(t *testing.T) {
	const size = 1 << 20
	content := randomBytes(size)

	act, err := readAllHinted(&shortReader{r: bytes.NewReader(content), max: 512}, 1024)
	require.NoError(t, err)
	assert.Equal(t, content, act)
}

//------------------------------------------------------------------------------

const benchSize = 64 * 1024 * 1024

// repeatReader yields n bytes in fixed size short reads without allocating a
// backing buffer of its own, so the benchmark measures only the read strategy.
type repeatReader struct {
	remaining int
	chunk     int
}

func (c *repeatReader) Read(p []byte) (int, error) {
	if c.remaining == 0 {
		return 0, io.EOF
	}
	n := min(min(len(p), c.chunk), c.remaining)
	for i := range p[:n] {
		p[i] = 'x'
	}
	c.remaining -= n
	return n, nil
}

func BenchmarkToTheEndReadAll(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(benchSize)
	for b.Loop() {
		buf, err := io.ReadAll(&repeatReader{remaining: benchSize, chunk: 32 * 1024})
		if err != nil {
			b.Fatal(err)
		}
		if len(buf) != benchSize {
			b.Fatalf("unexpected length %v", len(buf))
		}
	}
}

func BenchmarkToTheEndReadAllHinted(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(benchSize)
	for b.Loop() {
		buf, err := readAllHinted(&repeatReader{remaining: benchSize, chunk: 32 * 1024}, benchSize)
		if err != nil {
			b.Fatal(err)
		}
		if len(buf) != benchSize {
			b.Fatalf("unexpected length %v", len(buf))
		}
	}
}
