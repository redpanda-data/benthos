// Copyright 2025 Redpanda Data, Inc.

package manager

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Jeffail/shutdown"

	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/component/input"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

var _ input.Streamed = &InputWrapper{}

type inputCtrl struct {
	input         input.Streamed
	closedForSwap *int32
}

// InputWrapper is a wrapper for a streamed input.
type InputWrapper struct {
	ctrl      *inputCtrl
	inputLock sync.Mutex

	tranChan chan message.Transaction
	shutSig  *shutdown.Signaller
}

// WrapInput wraps a streamed input and starts the transaction processing loop.
func WrapInput(i input.Streamed) *InputWrapper {
	var s int32
	w := &InputWrapper{
		ctrl: &inputCtrl{
			input:         i,
			closedForSwap: &s,
		},
		tranChan: make(chan message.Transaction),
		shutSig:  shutdown.NewSignaller(),
	}
	go w.loop()
	return w
}

// CloseExistingInput instructs the wrapped input to stop consuming messages and
// waits for it to shut down.
func (w *InputWrapper) CloseExistingInput(ctx context.Context, forSwap bool) error {
	w.inputLock.Lock()
	tmpInput := w.ctrl.input
	if forSwap {
		atomic.StoreInt32(w.ctrl.closedForSwap, 1)
	} else {
		atomic.StoreInt32(w.ctrl.closedForSwap, 0)
	}
	w.inputLock.Unlock()

	if tmpInput == nil {
		return nil
	}

	tmpInput.TriggerStopConsuming()
	return tmpInput.WaitForClose(ctx)
}

// SwapInput swaps the wrapped input with another one.
func (w *InputWrapper) SwapInput(i input.Streamed) {
	var s int32
	w.inputLock.Lock()
	w.ctrl = &inputCtrl{
		input:         i,
		closedForSwap: &s,
	}
	w.inputLock.Unlock()
}

// TransactionChan returns a transactions channel for consuming messages from
// the wrapped input\.
func (w *InputWrapper) TransactionChan() <-chan message.Transaction {
	return w.tranChan
}

// ConnectionStatus returns the current status of the given component
// connection. The result is a slice in order to accommodate higher order
// components that wrap several others.
func (w *InputWrapper) ConnectionStatus() (s component.ConnectionStatuses) {
	w.inputLock.Lock()
	if w.ctrl.input != nil {
		s = w.ctrl.input.ConnectionStatus()
	}
	w.inputLock.Unlock()
	return
}

func (w *InputWrapper) loop() {
	defer func() {
		w.inputLock.Lock()
		tmpInput := w.ctrl.input
		w.inputLock.Unlock()

		if tmpInput != nil {
			tmpInput.TriggerStopConsuming()
			_ = tmpInput.WaitForClose(context.Background())
		}

		close(w.tranChan)
		w.shutSig.TriggerHasStopped()
	}()

	for {
		var tChan <-chan message.Transaction
		var closedForSwap *int32

		w.inputLock.Lock()
		if w.ctrl.input != nil {
			tChan = w.ctrl.input.TransactionChan()
			closedForSwap = w.ctrl.closedForSwap
		}
		w.inputLock.Unlock()

		var t message.Transaction
		var open bool

		if tChan != nil {
			select {
			case t, open = <-tChan:
				// If closed and is natural (not closed for swap) then exit
				// gracefully.
				if !open && atomic.LoadInt32(closedForSwap) == 0 {
					return
				}
			case <-w.shutSig.SoftStopChan():
				return
			}
		}

		if !open {
			select {
			case <-time.After(time.Millisecond * 100):
			case <-w.shutSig.SoftStopChan():
				return
			}
			continue
		}

		select {
		case w.tranChan <- t:
		case <-w.shutSig.SoftStopChan():
			ctx, done := w.shutSig.HardStopCtx(context.Background())
			_ = t.Ack(ctx, component.ErrTypeClosed)
			done()
			return
		}
	}
}

// TriggerStopConsuming instructs the wrapped input to start shutting down
// resources once all pending messages are delivered and acknowledged. This call
// does not block.
func (w *InputWrapper) TriggerStopConsuming() {
	w.shutSig.TriggerSoftStop()
}

// TriggerCloseNow triggers the shut down of the wrapped input but should not
// block the calling goroutine.
func (w *InputWrapper) TriggerCloseNow() {
	w.shutSig.TriggerHardStop()
}

// WaitForClose is a blocking call to wait until the wrapped input has finished
// shutting down and cleaning up resources.
func (w *InputWrapper) WaitForClose(ctx context.Context) error {
	select {
	case <-w.shutSig.HasStoppedChan():
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
