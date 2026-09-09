// Copyright 2026 Redpanda Data, Inc.

package scanner

import (
	"context"
	"io"

	"github.com/redpanda-data/benthos/v4/internal/message"
)

// AckFn is a function provided to a scanner that it should call once the
// derived io.ReadCloser is fully consumed.
type AckFn func(context.Context, error) error

// Scanner is an interface implemented by all scanner implementations once a
// creator has instantiated it on a byte stream.
type Scanner interface {
	Next(context.Context) (message.Batch, AckFn, error)
	Close(context.Context) error
}

// SourceDetails contains exclusively optional information which could be used
// by codec implementations in order to determine the underlying data format.
type SourceDetails struct {
	Name string

	// SizeHint is the total number of bytes of the source, when known, and is
	// zero otherwise. A source measured as empty is therefore
	// indistinguishable from one of unknown size, and implementations must
	// treat zero as unknown. It is a hint only, provided so that
	// implementations can pre-allocate buffers, and must never be relied upon
	// for correctness as the underlying source may change between it being
	// measured and read.
	//
	// A scanner that wraps a child scanner and materially transforms the
	// length of the stream (such as decompress) must clear the hint before
	// forwarding details, as it describes the stream the wrapper reads, not
	// the one the child does. A wrapper that shortens the stream by only a
	// bounded few bytes (such as skip_bom) may forward the hint unchanged, as
	// it remains a tight upper bound.
	SizeHint int64
}

// Creator is an interface implemented by all scanners, which allows components
// to construct a scanner from an unbounded io.ReadCloser.
type Creator interface {
	Create(rdr io.ReadCloser, aFn AckFn, details SourceDetails) (Scanner, error)
	Close(context.Context) error
}
