// Copyright 2025 Redpanda Data, Inc.

package io

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"

	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/public/service"
	"github.com/redpanda-data/benthos/v4/public/service/codec"
)

type stdinChunk struct {
	data []byte
	err  error
}

// stdinRelay reads from os.Stdin in a background goroutine and sends chunks
// through an unbuffered channel. This decouples the blocking os.Stdin.Read from
// individual consumers so that a cancelled consumer does not race with a new one
// for the next stdin read.
type stdinRelay struct {
	ch chan stdinChunk
}

var (
	globalRelay     *stdinRelay
	globalRelayOnce sync.Once
)

func getGlobalRelay() *stdinRelay {
	globalRelayOnce.Do(func() {
		r := &stdinRelay{ch: make(chan stdinChunk)}
		globalRelay = r
		go func() {
			buf := make([]byte, 32*1024)
			for {
				n, err := os.Stdin.Read(buf)
				if n > 0 {
					chunk := make([]byte, n)
					copy(chunk, buf[:n])
					r.ch <- stdinChunk{data: chunk}
				}
				if err != nil {
					r.ch <- stdinChunk{err: err}
					return
				}
			}
		}()
	})
	return globalRelay
}

// cancelableStdinReader reads from the shared stdinRelay but returns
// immediately when its context is cancelled or Close is called. This allows
// a bufio.Scanner wrapping this reader to unblock without draining os.Stdin,
// so that a replacement consumer (after a watcher-mode config reload) can pick
// up where the previous one left off.
type cancelableStdinReader struct {
	relay     *stdinRelay
	buf       []byte
	done      chan struct{}
	closeOnce sync.Once
	// ctx is set before each Read by setContext; both are called from the same
	// goroutine (the async reader loop) so no mutex is needed.
	ctx context.Context
}

func newCancelableStdinReader(relay *stdinRelay) *cancelableStdinReader {
	return &cancelableStdinReader{
		relay: relay,
		done:  make(chan struct{}),
		ctx:   context.Background(),
	}
}

func (r *cancelableStdinReader) setContext(ctx context.Context) {
	r.ctx = ctx
}

func (r *cancelableStdinReader) Read(p []byte) (int, error) {
	if len(r.buf) > 0 {
		n := copy(p, r.buf)
		r.buf = r.buf[n:]
		return n, nil
	}

	select {
	case chunk := <-r.relay.ch:
		if chunk.err != nil {
			return 0, chunk.err
		}
		n := copy(p, chunk.data)
		if n < len(chunk.data) {
			r.buf = append(r.buf[:0], chunk.data[n:]...)
		}
		return n, nil
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	case <-r.done:
		return 0, io.EOF
	}
}

func (r *cancelableStdinReader) Close() error {
	r.closeOnce.Do(func() { close(r.done) })
	return nil
}

func init() {
	service.MustRegisterBatchInput(
		"stdin", service.NewConfigSpec().
			Stable().
			Categories("Local").
			Summary(`Consumes data piped to stdin, chopping it into individual messages according to the specified scanner.`).
			Fields(codec.DeprecatedCodecFields("lines")...).Field(service.NewAutoRetryNacksToggleField()),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchInput, error) {
			rdr, err := newStdinConsumerFromParsed(conf)
			if err != nil {
				return nil, err
			}
			return service.AutoRetryNacksBatchedToggled(conf, rdr)
		})
}

type stdinConsumer struct {
	reader  *cancelableStdinReader
	scanner codec.DeprecatedFallbackStream
}

func newStdinConsumerFromParsed(conf *service.ParsedConfig) (*stdinConsumer, error) {
	c, err := codec.DeprecatedCodecFromParsed(conf)
	if err != nil {
		return nil, err
	}

	reader := newCancelableStdinReader(getGlobalRelay())

	s, err := c.Create(reader, func(_ context.Context, err error) error {
		return nil
	}, service.NewScannerSourceDetails())
	if err != nil {
		return nil, err
	}
	return &stdinConsumer{reader: reader, scanner: s}, nil
}

func (s *stdinConsumer) Connect(ctx context.Context) error {
	return nil
}

func (s *stdinConsumer) ReadBatch(ctx context.Context) (service.MessageBatch, service.AckFunc, error) {
	s.reader.setContext(ctx)

	parts, codecAckFn, err := s.scanner.NextBatch(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			err = component.ErrTimeout
		}
		if err != component.ErrTimeout {
			s.scanner.Close(ctx)
		}
		if errors.Is(err, io.EOF) {
			return nil, nil, service.ErrEndOfInput
		}
		return nil, nil, err
	}
	_ = codecAckFn(ctx, nil)

	if len(parts) == 0 {
		return nil, nil, component.ErrTimeout
	}

	return parts, func(rctx context.Context, res error) error {
		return nil
	}, nil
}

func (s *stdinConsumer) Close(ctx context.Context) (err error) {
	if s.scanner != nil {
		err = s.scanner.Close(ctx)
	}
	return
}
