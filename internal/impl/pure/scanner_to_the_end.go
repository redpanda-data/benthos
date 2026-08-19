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

// maxPreallocHint caps the capacity pre-allocated from a size hint. The hint
// originates from external data (typically a Stat) and can be wildly wrong:
// procfs files report multi-terabyte sizes, and a custom filesystem can return
// anything, including values for which make() panics outright (above the
// runtime's allocation limit, or overflowing hint+1). io.ReadAll bounded
// memory by growing incrementally, so pre-allocation must not turn a bogus
// hint into an enormous up-front allocation. Content beyond the clamp still
// reads correctly, paying only the incremental growth it always used to.
const maxPreallocHint = 1 << 30

// readAllHinted is io.ReadAll with a starting capacity hint.
//
// Semantics are identical to io.ReadAll; the hint is purely an optimisation.
// If the source is larger than the hint the buffer grows exactly as before,
// paying the copy only for the excess; if smaller, the read ends early. This
// matters because a file can be appended to between Stat and read.
//
// A hint of zero or below means the size is unknown, in which case io.ReadAll
// is used directly: its growth strategy starts at 512 bytes, which beats
// growing from an empty buffer.
//
// The +1 on capacity is deliberate: the loop can only Read while cap > len, so
// a buffer sized exactly to the content would find len == cap on the final
// iteration and grow once more, reintroducing the reallocation this avoids.
func readAllHinted(r io.Reader, hint int64) ([]byte, error) {
	if hint <= 0 {
		return io.ReadAll(r)
	}
	buf := make([]byte, 0, min(hint, maxPreallocHint)+1)
	for {
		if len(buf) == cap(buf) {
			buf = append(buf, 0)[:len(buf)]
		}
		n, err := r.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
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
