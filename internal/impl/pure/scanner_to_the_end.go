// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"context"
	"io"

	"github.com/redpanda-data/benthos/v4/public/service"
)

func toTheEndScannerSpec() *service.ConfigSpec {
	return service.NewConfigSpec().
		Stable().
		Summary("Read the input stream all the way until the end and deliver it as a single message.").
		Description(`
[CAUTION]
====
Some sources of data may not have a logical end, therefore caution should be made to exclusively use this scanner when the end of an input stream is clearly defined (and well within memory).
====
`).
		Field(service.NewObjectField("").Default(map[string]any{}))
}

func init() {
	service.MustRegisterBatchScannerCreator("to_the_end", toTheEndScannerSpec(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchScannerCreator, error) {
			return toTheEndScannerCreatorFromParsed(conf)
		})
}

func toTheEndScannerCreatorFromParsed(conf *service.ParsedConfig) (s *toTheEndScannerCreator, err error) {
	s = &toTheEndScannerCreator{}
	return
}

type toTheEndScannerCreator struct{}

func (l *toTheEndScannerCreator) Create(rdr io.ReadCloser, aFn service.AckFunc, details *service.ScannerSourceDetails) (service.BatchScanner, error) {
	// The size hint is used to pre-allocate the read buffer only, and is zero
	// for sources of unknown length.
	return service.AutoAggregateBatchScannerAcks(&toTheEndScanner{r: rdr, sizeHint: details.SizeHint()}, aFn), nil
}

func (l *toTheEndScannerCreator) Close(context.Context) error {
	return nil
}

type toTheEndScanner struct {
	r        io.ReadCloser
	sizeHint int64
}

const (
	// defaultReadAllCap matches the initial buffer capacity io.ReadAll uses.
	// It floors the pre-allocation so a hint that underestimates wildly (a
	// file appended to after being measured) doesn't begin with
	// pathologically small reads.
	defaultReadAllCap = 512

	// maxPreallocHint caps the capacity pre-allocated from a size hint. The
	// hint originates from external data (typically a Stat) and can be wildly
	// wrong: procfs files report multi-terabyte sizes, and a custom
	// filesystem can return anything, including values for which make()
	// panics outright (above the runtime's allocation limit, or overflowing
	// hint+1). io.ReadAll bounded memory by growing incrementally, so
	// pre-allocation must not turn a bogus hint into a memory spike that can
	// take out a constrained deployment before a single byte is read. 128 MiB
	// covers typical whole-stream messages while bounding the blast radius of
	// a wrong hint; content beyond the clamp reads correctly, growing by
	// doubling.
	maxPreallocHint = 128 << 20

	// maxReturnWaste is the largest gap between the returned buffer's
	// capacity and its content that readAllHinted will leave in place. Beyond
	// it the content is copied down, as callers hold the returned bytes for
	// the lifetime of the message and would otherwise pin the excess. Below
	// it a copy costs more than the memory it reclaims.
	maxReturnWaste = 64 << 10
)

// hintedCap converts a positive size hint into the capacity to pre-allocate:
// the hint floored at io.ReadAll's own starting size, capped at the
// pre-allocation clamp, plus one. SetSizeHint is exported, so the hint is
// untrusted input; the clamp keeps a bogus value from panicking make() or
// spiking memory before a byte is read.
//
// The +1 is deliberate: the read loop can only Read while cap > len, so a
// buffer sized to exactly the content would find len == cap on the final
// iteration and grow once more, reintroducing the reallocation this avoids.
func hintedCap(hint int64) int64 {
	return max(min(hint, maxPreallocHint), defaultReadAllCap) + 1
}

// readAllHinted is io.ReadAll with a starting capacity hint.
//
// The bytes and error returned are identical to io.ReadAll for every hint
// value; the hint only shapes allocation. A hint of zero or below means the
// size is unknown, in which case io.ReadAll is used directly.
//
// An accurate hint reads the content into a single allocation returned as-is.
// A hint that falls short (the source grew after being measured, or exceeds
// the clamp) grows by doubling, keeping cumulative allocation linear in the
// content size much like io.ReadAll's own chunking. A buffer returned with
// meaningfully more capacity than content (the source shrank, or the
// measurement was wrong) is copied down first, so a wrong hint can't pin
// excess memory for the lifetime of the returned bytes.
func readAllHinted(r io.Reader, hint int64) ([]byte, error) {
	if hint <= 0 {
		return io.ReadAll(r)
	}
	buf := make([]byte, 0, hintedCap(hint))
	for {
		if len(buf) == cap(buf) {
			// The +1 serves the same purpose as in hintedCap: content ending
			// exactly at a doubled capacity can observe EOF in the spare byte
			// rather than forcing one more doubling.
			grown := make([]byte, len(buf), 2*cap(buf)+1)
			copy(grown, buf)
			buf = grown
		}
		n, err := r.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			// The error path skips the copy-down: the only caller discards
			// the buffer when err != nil, and io.ReadAll's contract covers
			// contents and error, not capacity.
			if err == nil && cap(buf)-len(buf) > maxReturnWaste {
				buf = append(make([]byte, 0, len(buf)), buf...)
			}
			return buf, err
		}
	}
}

func (t *toTheEndScanner) NextBatch(ctx context.Context) (service.MessageBatch, error) {
	if t.r == nil {
		return nil, io.EOF
	}
	mBytes, err := readAllHinted(t.r, t.sizeHint)
	if err != nil {
		return nil, err
	}
	_ = t.r.Close()
	t.r = nil
	return service.MessageBatch{service.NewMessage(mBytes)}, err
}

func (t *toTheEndScanner) Close(ctx context.Context) error {
	if t.r == nil {
		return nil
	}
	return t.r.Close()
}
