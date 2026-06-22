// Copyright 2025 Redpanda Data, Inc.

package output

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"go.opentelemetry.io/otel/trace"

	"github.com/Jeffail/shutdown"

	"github.com/redpanda-data/benthos/v4/internal/batch"
	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/component/metrics"
	"github.com/redpanda-data/benthos/v4/internal/log"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/internal/tracing"
)

// AsyncSink is a type that writes Benthos messages to a third party sink. If
// the protocol supports a form of acknowledgement then it will be returned by
// the call to Write.
type AsyncSink interface {
	// ConnectionTest attempts to establish whether the component is capable of
	// creating a connection. This will potentially require and test network
	// connectivity, but does not require the component to be initialized.
	ConnectionTest(ctx context.Context) component.ConnectionTestResults

	// Connect attempts to establish a connection to the sink, if
	// unsuccessful returns an error. If the attempt is successful (or not
	// necessary) returns nil.
	Connect(ctx context.Context) error

	// WriteBatch should block until either the message is sent (and
	// acknowledged) to a sink, or a transport specific error has occurred, or
	// the Type is closed.
	WriteBatch(ctx context.Context, msg message.Batch) error

	// Close is a blocking call to wait until the component has finished
	// shutting down and cleaning up resources.
	Close(ctx context.Context) error
}

// AsyncWriter is an output type that writes messages to a writer.Type.
type AsyncWriter struct {
	connection atomic.Pointer[component.ConnectionStatus]

	typeStr     string
	maxInflight int
	strict      bool
	writer      AsyncSink

	mgr    component.Observability
	log    log.Modular
	stats  metrics.Type
	tracer trace.TracerProvider

	transactions <-chan message.Transaction

	startOnce sync.Once
	shutSig   *shutdown.Signaller
}

// NewAsyncWriter creates a Streamed implementation around an AsyncSink. When
// strict is true, messages that arrive already flagged as failed are rejected
// (nacked) rather than written, on a per-message basis.
func NewAsyncWriter(typeStr string, maxInflight int, strict bool, w AsyncSink, mgr component.Observability) (Streamed, error) {
	aWriter := &AsyncWriter{
		typeStr:      typeStr,
		maxInflight:  maxInflight,
		strict:       strict,
		writer:       w,
		mgr:          mgr,
		log:          mgr.Logger(),
		stats:        mgr.Metrics(),
		tracer:       mgr.Tracer(),
		transactions: nil,
		shutSig:      shutdown.NewSignaller(),
	}
	aWriter.connection.Store(component.ConnectionPending(mgr))
	return aWriter, nil
}

//------------------------------------------------------------------------------

func (w *AsyncWriter) latencyMeasuringWrite(ctx context.Context, msg message.Batch) (latencyNs int64, err error) {
	t0 := time.Now()
	err = w.writer.WriteBatch(ctx, msg)
	if latencyNs = time.Since(t0).Nanoseconds(); latencyNs < 1 {
		latencyNs = 1
	}
	return latencyNs, err
}

// loop is an internal loop that brokers incoming messages to output pipe.
func (w *AsyncWriter) loop() {
	// Metrics paths
	var (
		mSent       = w.stats.GetCounter("output_sent")
		mBatchSent  = w.stats.GetCounter("output_batch_sent")
		mError      = w.stats.GetCounter("output_error")
		mRejected   = w.stats.GetCounter("output_rejected")
		mLatency    = w.stats.GetTimer("output_latency_ns")
		mConn       = w.stats.GetCounter("output_connection_up")
		mFailedConn = w.stats.GetCounter("output_connection_failed")
		mLostConn   = w.stats.GetCounter("output_connection_lost")

		traceName = "output_" + w.typeStr
	)

	defer func() {
		_ = w.writer.Close(context.Background())

		w.connection.Store(component.ConnectionClosed(w.mgr))
		w.shutSig.TriggerHasStopped()
	}()

	connBackoff := backoff.NewExponentialBackOff()
	connBackoff.InitialInterval = time.Millisecond * 500
	connBackoff.MaxInterval = time.Second
	connBackoff.MaxElapsedTime = 0

	closeLeisureCtx, done := w.shutSig.SoftStopCtx(context.Background())
	defer done()

	initConnection := func() bool {
		for {
			if err := w.writer.Connect(closeLeisureCtx); err != nil {
				if w.shutSig.IsSoftStopSignalled() || errors.Is(err, component.ErrTypeClosed) {
					return false
				}
				w.connection.Store(component.ConnectionFailing(w.mgr, err))
				w.log.Error("Failed to connect to %v: %v\n", w.typeStr, err)
				mFailedConn.Incr(1)

				var nextBoff time.Duration

				var ebo *component.ErrBackOff
				if errors.As(err, &ebo) {
					nextBoff = ebo.Wait
				} else {
					nextBoff = connBackoff.NextBackOff()
				}

				if sleepWithCancellation(closeLeisureCtx, nextBoff) != nil {
					return false
				}
			} else {
				connBackoff.Reset()
				return true
			}
		}
	}
	if !initConnection() {
		return
	}

	w.log.Info("Output type %v is now active", w.typeStr)
	mConn.Incr(1)
	w.connection.Store(component.ConnectionActive(w.mgr))

	wg := sync.WaitGroup{}
	wg.Add(w.maxInflight)

	connectMut := sync.Mutex{}
	connectLoop := func(msg message.Batch) (latency int64, err error) {
		w.connection.Store(component.ConnectionFailing(w.mgr, component.ErrNotConnected))

		connectMut.Lock()
		defer connectMut.Unlock()

		// If another goroutine got here first and we're able to send over the
		// connection, then we gracefully accept defeat.
		if w.connection.Load().Connected {
			if latency, err = w.latencyMeasuringWrite(closeLeisureCtx, msg); err != component.ErrNotConnected {
				return
			} else if err != nil {
				mError.Incr(1)
			}
		}
		mLostConn.Incr(1)

		// Continue to try to reconnect while still active.
		for {
			if !initConnection() {
				err = component.ErrTypeClosed
				return
			}
			if latency, err = w.latencyMeasuringWrite(closeLeisureCtx, msg); err != component.ErrNotConnected {
				w.connection.Store(component.ConnectionActive(w.mgr))
				mConn.Incr(1)
				return
			} else if err != nil {
				mError.Incr(1)
			}
		}
	}

	writerLoop := func() {
		defer wg.Done()

		for {
			var ts message.Transaction
			var open bool
			select {
			case ts, open = <-w.transactions:
				if !open {
					return
				}
			case <-w.shutSig.SoftStopChan():
				return
			}

			payload := ts.Payload
			ackFn := ts.Ack

			// In strict mode, messages that have already failed a processing step
			// are rejected (nacked) rather than written, on a per-message basis.
			if w.strict {
				var rejectErr *batch.Error
				var sampleErr error
				for i, m := range payload {
					mErr := m.ErrorGet()
					if mErr == nil {
						continue
					}
					if sampleErr == nil {
						sampleErr = mErr
					}
					mErr = fmt.Errorf("rejected due to failed processing: %w", mErr)
					if rejectErr == nil {
						rejectErr = batch.NewError(payload, mErr)
					}
					rejectErr.Failed(i, mErr)
				}

				if rejectErr != nil {
					mRejected.Incr(int64(rejectErr.IndexedErrors()))
					w.log.Warn("Rejecting %v of %v message(s) for output '%v' because they failed a processing step and strict error handling is enabled (example error: %v). To recover from expected errors and allow these messages through, wrap the failing step within a try_catch (or retry) processor; otherwise they will be nacked and retried by the input.\n", rejectErr.IndexedErrors(), len(payload), w.typeStr, sampleErr)

					if rejectErr.IndexedErrors() == len(payload) {
						// Every message failed: nack the whole batch without writing.
						_ = ts.Ack(closeLeisureCtx, rejectErr)
						continue
					}

					// Mixed batch: write only the messages that did not fail, and
					// merge any write failure back into the rejection error.
					sortGroup, sortedBatch := message.NewSortGroup(payload)
					forwardBatch := make(message.Batch, 0, len(payload)-rejectErr.IndexedErrors())
					rejectErr.WalkPartsNaively(func(i int, _ *message.Part, err error) bool {
						if err == nil {
							forwardBatch = append(forwardBatch, sortedBatch[i])
						}
						return true
					})
					payload = forwardBatch
					ackFn = func(ctx context.Context, werr error) error {
						if werr == nil {
							return ts.Ack(ctx, rejectErr)
						}
						var tmpBatchErr *batch.Error
						if errors.As(werr, &tmpBatchErr) {
							tmpBatchErr.WalkPartsBySource(sortGroup, sortedBatch, func(i int, _ *message.Part, err error) bool {
								if err != nil {
									rejectErr.Failed(i, err)
								}
								return true
							})
							return ts.Ack(ctx, rejectErr)
						}
						for _, p := range forwardBatch {
							if i := sortGroup.GetIndex(p); i >= 0 {
								rejectErr.Failed(i, werr)
							}
						}
						return ts.Ack(ctx, rejectErr)
					}
				}
			}

			w.log.Trace("Attempting to write %v messages to '%v'.\n", payload.Len(), w.typeStr)
			_, spans := tracing.WithChildSpans(w.tracer, traceName, payload)

			latency, err := w.latencyMeasuringWrite(closeLeisureCtx, payload)

			// If our writer says it is not connected.
			if errors.Is(err, component.ErrNotConnected) {
				latency, err = connectLoop(payload)
			} else if err != nil {
				mError.Incr(1)
			}

			// Close immediately if our writer is closed.
			if errors.Is(err, component.ErrTypeClosed) {
				return
			}

			if err != nil {
				if w.typeStr != "reject" {
					// TODO: Maybe reintroduce a sleep here if we encounter a
					// busy retry loop.
					w.log.Error("Failed to send message to %v: %v\n", w.typeStr, err)
				} else {
					w.log.Debug("Rejecting message: %v\n", err)
				}
			} else {
				mBatchSent.Incr(1)
				mSent.Incr(int64(batch.MessageCollapsedCount(payload)))
				mLatency.Timing(latency)
				w.log.Trace("Successfully wrote %v messages to '%v'.\n", payload.Len(), w.typeStr)
			}

			for _, s := range spans {
				s.Finish()
			}

			_ = ackFn(closeLeisureCtx, err)
		}
	}

	for i := 0; i < w.maxInflight; i++ {
		go writerLoop()
	}
	wg.Wait()
}

// Consume assigns a messages channel for the output to read.
func (w *AsyncWriter) Consume(ts <-chan message.Transaction) error {
	if w.transactions != nil {
		return component.ErrAlreadyStarted
	}
	w.transactions = ts
	return nil
}

// ConnectionTest attempts to establish whether the component is capable of
// creating a connection. This will potentially require and test network
// connectivity, but does not require the component to be initialized.
func (w *AsyncWriter) ConnectionTest(ctx context.Context) component.ConnectionTestResults {
	return w.writer.ConnectionTest(ctx)
}

// ConnectionStatus returns the status of the given output connection.
func (w *AsyncWriter) ConnectionStatus() component.ConnectionStatuses {
	return component.ConnectionStatuses{
		w.connection.Load(),
	}
}

// TriggerStartConsuming initiates async connection and consumption.
func (w *AsyncWriter) TriggerStartConsuming() {
	w.startOnce.Do(func() {
		go w.loop()
	})
}

// TriggerCloseNow shuts down the output and stops processing messages.
func (w *AsyncWriter) TriggerCloseNow() {
	w.shutSig.TriggerHardStop()
}

// WaitForClose blocks until the File output has closed down.
func (w *AsyncWriter) WaitForClose(ctx context.Context) error {
	select {
	case <-w.shutSig.HasStoppedChan():
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func sleepWithCancellation(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
