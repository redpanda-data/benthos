// Copyright 2025 Redpanda Data, Inc.

package pipeline_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/log"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/internal/pipeline"

	_ "github.com/redpanda-data/benthos/v4/internal/impl/pure"
)

// blockingProcessor blocks ProcessBatch until blockCh is closed, then passes
// messages through unchanged. closeFn is called on Close.
type blockingProcessor struct {
	blockCh chan struct{}
	closeFn func()
}

func (b *blockingProcessor) ProcessBatch(_ context.Context, msg message.Batch) ([]message.Batch, error) {
	<-b.blockCh
	return []message.Batch{msg}, nil
}

func (b *blockingProcessor) Close(_ context.Context) error {
	if b.closeFn != nil {
		b.closeFn()
	}
	return nil
}

func TestPoolBasic(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	mockProc := &mockMsgProcessor{dropChan: make(chan bool)}
	go func() {
		mockProc.dropChan <- true
	}()

	proc, err := pipeline.NewPool(1, log.Noop(), mockProc)
	require.NoError(t, err)

	tChan, resChan := make(chan message.Transaction), make(chan error)

	require.NoError(t, proc.Consume(tChan))
	assert.Error(t, proc.Consume(tChan))

	msg := message.QuickBatch([][]byte{
		[]byte(`one`),
		[]byte(`two`),
	})

	// First message should be dropped and return immediately
	select {
	case tChan <- message.NewTransaction(msg, resChan):
	case <-time.After(time.Second):
		t.Fatal("Timed out")
	}
	select {
	case _, open := <-proc.TransactionChan():
		if !open {
			t.Fatal("Closed early")
		} else {
			t.Fatal("Message was not dropped")
		}
	case res, open := <-resChan:
		if !open {
			t.Fatal("Closed early")
		}
		if res != errMockProc {
			t.Error(res)
		}
	case <-time.After(time.Second * 5):
		t.Fatal("Timed out")
	}

	// Do not drop next message
	go func() {
		mockProc.dropChan <- false
	}()

	// Send message
	select {
	case tChan <- message.NewTransaction(msg, resChan):
	case <-time.After(time.Second * 5):
		t.Fatal("Timed out")
	}

	var procT message.Transaction
	var open bool

	// Receive new message
	select {
	case procT, open = <-proc.TransactionChan():
		if !open {
			t.Error("Closed early")
		}
		if exp, act := [][]byte{[]byte("foo"), []byte("bar")}, message.GetAllBytes(procT.Payload); !reflect.DeepEqual(exp, act) {
			t.Errorf("Wrong message received: %s != %s", act, exp)
		}
	case <-time.After(time.Second * 5):
		t.Fatal("Timed out")
	}

	// Respond without error
	go func() {
		require.NoError(t, procT.Ack(ctx, nil))
	}()

	// Receive response
	select {
	case res, open := <-resChan:
		if !open {
			t.Error("Closed early")
		}
		if res != nil {
			t.Error(res)
		}
	case <-time.After(time.Second * 5):
		t.Fatal("Timed out")
	}

	proc.TriggerCloseNow()
	require.NoError(t, proc.WaitForClose(ctx))
}

func TestPoolMultiMsgs(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	mockProc := &mockSplitProcessor{}

	proc, err := pipeline.NewPool(1, log.Noop(), mockProc)
	require.NoError(t, err)

	tChan, resChan := make(chan message.Transaction), make(chan error)
	if err := proc.Consume(tChan); err != nil {
		t.Fatal(err)
	}

	for range 10 {
		expMsgs := map[string]struct{}{
			"foo test": {},
			"bar test": {},
			"baz test": {},
		}

		// Send message
		select {
		case tChan <- message.NewTransaction(message.Batch{
			message.NewPart([]byte(`foo`)),
			message.NewPart([]byte(`bar`)),
			message.NewPart([]byte(`baz`)),
		}, resChan):
		case <-time.After(time.Second * 5):
			t.Fatal("Timed out")
		}

		for range 3 {
			// Receive messages
			var procT message.Transaction
			var open bool
			select {
			case procT, open = <-proc.TransactionChan():
				if !open {
					t.Error("Closed early")
				}
				act := string(procT.Payload.Get(0).AsBytes())
				if _, exists := expMsgs[act]; !exists {
					t.Errorf("Unexpected result: %v", act)
				} else {
					delete(expMsgs, act)
				}
			case <-time.After(time.Second * 5):
				t.Fatal("Timed out")
			}

			// Respond with no error
			require.NoError(t, procT.Ack(ctx, nil))
		}

		// Receive response
		select {
		case res, open := <-resChan:
			if !open {
				t.Error("Closed early")
			} else if res != nil {
				t.Error(res)
			}
		case <-time.After(time.Second * 5):
			t.Fatal("Timed out")
		}

		if len(expMsgs) != 0 {
			t.Errorf("Expected messages were not received: %v", expMsgs)
		}
	}

	proc.TriggerCloseNow()
	require.NoError(t, proc.WaitForClose(ctx))
}

func TestPoolMultiThreads(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	conf := pipeline.NewConfig()
	conf.Threads = 2
	conf.Processors = append(conf.Processors, processor.NewConfig())

	proc, err := pipeline.New(conf, mock.NewManager())
	if err != nil {
		t.Fatal(err)
	}

	tChan, resChan := make(chan message.Transaction), make(chan error)
	if err := proc.Consume(tChan); err != nil {
		t.Fatal(err)
	}

	msg := message.QuickBatch([][]byte{
		[]byte(`one`),
		[]byte(`two`),
	})

	for j := 0; j < conf.Threads; j++ {
		// Send message
		select {
		case tChan <- message.NewTransaction(msg, resChan):
		case <-time.After(time.Second * 5):
			t.Fatal("Timed out")
		}
	}
	for j := 0; j < conf.Threads; j++ {
		// Receive messages
		var procT message.Transaction
		var open bool
		select {
		case procT, open = <-proc.TransactionChan():
			if !open {
				t.Error("Closed early")
			}
			if exp, act := [][]byte{[]byte("one"), []byte("two")}, message.GetAllBytes(procT.Payload); !reflect.DeepEqual(exp, act) {
				t.Errorf("Wrong message received: %s != %s", act, exp)
			}
		case <-time.After(time.Second * 5):
			t.Fatal("Timed out")
		}

		go func(tran message.Transaction) {
			// Respond with no error
			require.NoError(t, tran.Ack(ctx, nil))
		}(procT)
	}
	for j := 0; j < conf.Threads; j++ {
		// Receive response
		select {
		case res, open := <-resChan:
			if !open {
				t.Error("Closed early")
			} else if res != nil {
				t.Error(res)
			}
		case <-time.After(time.Second * 5):
			t.Fatal("Timed out")
		}
	}

	proc.TriggerCloseNow()
	require.NoError(t, proc.WaitForClose(ctx))
}

func TestPoolProcessorsNotClosedEarly(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	var (
		mu      sync.Mutex
		closed  bool
		closeCh = make(chan struct{})
		blockCh = make(chan struct{})
	)

	proc := &blockingProcessor{
		blockCh: blockCh,
		closeFn: func() {
			mu.Lock()
			defer mu.Unlock()
			closed = true
			close(closeCh)
		},
	}

	pool, err := pipeline.NewPool(2, log.Noop(), proc)
	require.NoError(t, err)

	tChan := make(chan message.Transaction)
	resChan := make(chan error, 1)

	require.NoError(t, pool.Consume(tChan))

	// Send one message — one worker picks it up and blocks, the other will
	// see the channel close and exit.
	select {
	case tChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte("hello")}), resChan):
	case <-ctx.Done():
		t.Fatal("timed out sending message")
	}

	// Close the input channel. One worker exits, but the other is blocked.
	close(tChan)

	// Give the exiting worker time to run its defer. If the bug were present,
	// it would call Close() on the shared processor here.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	closedEarly := closed
	mu.Unlock()
	require.False(t, closedEarly, "processor was closed while another worker was still processing")

	// Unblock the processing worker.
	close(blockCh)

	// Drain the output.
	select {
	case procT := <-pool.TransactionChan():
		require.NoError(t, procT.Ack(ctx, nil))
	case <-ctx.Done():
		t.Fatal("timed out reading output")
	}

	// Wait for close — this is where the pool should close processors.
	require.NoError(t, pool.WaitForClose(ctx))

	// Now the processor should be closed.
	select {
	case <-closeCh:
	case <-ctx.Done():
		t.Fatal("processor was never closed")
	}
}

func TestPoolMultiNaturalClose(t *testing.T) {
	conf := pipeline.NewConfig()
	conf.Threads = 2
	conf.Processors = append(conf.Processors, processor.NewConfig())

	proc, err := pipeline.New(conf, mock.NewManager())
	if err != nil {
		t.Fatal(err)
	}

	tChan := make(chan message.Transaction)
	if err := proc.Consume(tChan); err != nil {
		t.Fatal(err)
	}

	close(tChan)
	require.NoError(t, proc.WaitForClose(t.Context()))
}
