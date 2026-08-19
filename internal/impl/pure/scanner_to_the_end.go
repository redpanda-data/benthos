// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"context"
	"io"
	"math"

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
	// The size hint is used to pre-allocate the read buffer only, and is absent
	// for sources of unknown length.
	var sizeHint int64
	if details != nil {
		sizeHint = details.SizeHint()
	}
	return service.AutoAggregateBatchScannerAcks(&toTheEndScanner{r: rdr, sizeHint: sizeHint}, aFn), nil
}

func (l *toTheEndScannerCreator) Close(context.Context) error {
	return nil
}

type toTheEndScanner struct {
	r        io.ReadCloser
	sizeHint int64
}

// defaultReadAllCap matches the initial buffer capacity io.ReadAll uses. Sources
// without a useful size hint (sockets, stdin, and other unbounded streams that
// route through this scanner) therefore keep io.ReadAll's allocation profile
// rather than growing up from a single byte.
const defaultReadAllCap = 512

// hintedCap converts a caller-supplied size hint into the capacity to
// pre-allocate.
//
// The hint is floored at defaultReadAllCap so that a small, absent, or negative
// hint never allocates worse than io.ReadAll would, and capped just below
// math.MaxInt64 so the +1 below cannot overflow to a negative capacity —
// SetSizeHint is exported, so the hint is untrusted input.
//
// The +1 is deliberate: the read loop can only Read while cap > len, so a buffer
// sized to exactly the content would find len == cap on the final iteration and
// grow once more, reintroducing the reallocation this avoids.
func hintedCap(hint int64) int64 {
	if hint < defaultReadAllCap {
		hint = defaultReadAllCap
	}
	if hint > math.MaxInt64-1 {
		hint = math.MaxInt64 - 1
	}
	return hint + 1
}

// readAllHinted is io.ReadAll with a starting capacity hint.
//
// Semantics are identical to io.ReadAll; the hint is purely an optimisation.
// If the source is larger than the hint the buffer grows exactly as before,
// paying the copy only for the excess; if smaller, the read ends early. This
// matters because a file can be appended to between Stat and read.
func readAllHinted(r io.Reader, hint int64) ([]byte, error) {
	buf := make([]byte, 0, hintedCap(hint))
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
