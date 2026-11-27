// Copyright 2025 Redpanda Data, Inc.

package batcher_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	batchInternal "github.com/redpanda-data/benthos/v4/internal/batch"
	"github.com/redpanda-data/benthos/v4/internal/batch/policy"
	"github.com/redpanda-data/benthos/v4/internal/batch/policy/batchconfig"
	"github.com/redpanda-data/benthos/v4/internal/component/output/batcher"
	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"

	_ "github.com/redpanda-data/benthos/v4/internal/impl/pure"
)

func TestBatcherEarlyTermination(t *testing.T) {
	tInChan := make(chan message.Transaction)
	resChan := make(chan error)

	policyConf := batchconfig.NewConfig()
	policyConf.Count = 10
	policyConf.Period = "50ms"
	batchPol, err := policy.New(policyConf, mock.NewManager())
	require.NoError(t, err)

	out := &mock.OutputChanneled{}

	b := batcher.New(batchPol, out, mock.NewManager())
	require.NoError(t, b.Consume(tInChan))

	b.TriggerStartConsuming()

	ctx, done := context.WithTimeout(t.Context(), time.Second)
	done()

	require.Error(t, b.WaitForClose(ctx))

	select {
	case tInChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte("foo")}), resChan):
	case <-time.After(time.Second):
		t.Error("unexpected")
	}

	ctx, done = context.WithTimeout(t.Context(), time.Second)
	defer done()

	require.Error(t, b.WaitForClose(ctx))
}

func TestBatcherBasic(t *testing.T) {
	tInChan := make(chan message.Transaction)
	resChan := make(chan error)

	policyConf := batchconfig.NewConfig()
	policyConf.Count = 4
	batchPol, err := policy.New(policyConf, mock.NewManager())
	require.NoError(t, err)

	out := &mock.OutputChanneled{}

	b := batcher.New(batchPol, out, mock.NewManager())
	require.NoError(t, b.Consume(tInChan))

	b.TriggerStartConsuming()

	tOutChan := out.TChan

	var firstBatchExpected [][]byte
	var secondBatchExpected [][]byte
	var finalBatchExpected [][]byte
	for i := range 10 {
		inputBytes := fmt.Appendf(nil, "foo %v", i)
		if i < 4 {
			firstBatchExpected = append(firstBatchExpected, inputBytes)
		} else if i < 8 {
			secondBatchExpected = append(secondBatchExpected, inputBytes)
		} else {
			finalBatchExpected = append(finalBatchExpected, inputBytes)
		}
	}

	firstErr := errors.New("first error")
	secondErr := errors.New("second error")
	finalErr := errors.New("final error")

	wg := sync.WaitGroup{}
	wg.Go(func() {
		for _, batch := range firstBatchExpected {
			select {
			case tInChan <- message.NewTransaction(message.QuickBatch([][]byte{batch}), resChan):
			case <-time.After(time.Second):
				t.Error("timed out")
			}
		}
		for range firstBatchExpected {
			select {
			case actRes := <-resChan:
				assert.Equal(t, firstErr, actRes)
			case <-time.After(time.Second):
				t.Error("timed out")
			}
		}
		for _, batch := range secondBatchExpected {
			select {
			case tInChan <- message.NewTransaction(message.QuickBatch([][]byte{batch}), resChan):
			case <-time.After(time.Second):
				t.Error("timed out")
			}
		}
		for range secondBatchExpected {
			select {
			case actRes := <-resChan:
				assert.Equal(t, secondErr, actRes)
			case <-time.After(time.Second):
				t.Error("timed out")
			}
		}
		for _, batch := range finalBatchExpected {
			select {
			case tInChan <- message.NewTransaction(message.QuickBatch([][]byte{batch}), resChan):
			case <-time.After(time.Second):
				t.Error("timed out")
			}
		}
		close(tInChan)
		for range finalBatchExpected {
			select {
			case actRes := <-resChan:
				assert.Equal(t, finalErr, actRes)
			case <-time.After(time.Second):
				t.Error("timed out")
			}
		}
	})

	sendResponse := func(tran message.Transaction, err error) {
		sCtx, done := context.WithTimeout(t.Context(), time.Second)
		defer done()
		defer wg.Done()
		require.NoError(t, tran.Ack(sCtx, err))
	}

	// Receive first batch on output
	select {
	case outTr := <-tOutChan:
		if exp, act := firstBatchExpected, message.GetAllBytes(outTr.Payload); !reflect.DeepEqual(exp, act) {
			t.Errorf("Wrong result from batch: %s != %s", act, exp)
		}
		wg.Add(1)
		go sendResponse(outTr, firstErr)
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for message read")
	}

	// Receive second batch on output
	select {
	case outTr := <-tOutChan:
		if exp, act := secondBatchExpected, message.GetAllBytes(outTr.Payload); !reflect.DeepEqual(exp, act) {
			t.Errorf("Wrong result from batch: %s != %s", act, exp)
		}
		wg.Add(1)
		go sendResponse(outTr, secondErr)
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for message read")
	}

	// Receive final batch on output
	select {
	case outTr := <-tOutChan:
		if exp, act := finalBatchExpected, message.GetAllBytes(outTr.Payload); !reflect.DeepEqual(exp, act) {
			t.Errorf("Wrong result from batch: %s != %s", act, exp)
		}
		wg.Add(1)
		go sendResponse(outTr, finalErr)
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for message read")
	}

	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	require.NoError(t, b.WaitForClose(ctx))
	wg.Wait()
}

func TestBatcherMaxInFlight(t *testing.T) {
	timeOutCtx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	tInChan := make(chan message.Transaction)

	policyConf := batchconfig.NewConfig()
	policyConf.Count = 2
	batchPol, err := policy.New(policyConf, mock.NewManager())
	require.NoError(t, err)

	out := &mock.OutputChanneled{}

	b := batcher.New(batchPol, out, mock.NewManager())
	require.NoError(t, b.Consume(tInChan))

	b.TriggerStartConsuming()

	tOutChan := out.TChan
	resChanOne, resChanTwo := make(chan error), make(chan error)

	select {
	case tInChan <- message.NewTransaction(message.QuickBatch([][]byte{
		[]byte("hello world 1"),
		[]byte("hello world 2"),
		[]byte("hello world 3"),
		[]byte("hello world 4"),
	}), resChanOne):
	case <-timeOutCtx.Done():
		t.Fatal("timed out")
	}

	var tranOne message.Transaction
	select {
	case tranOne = <-tOutChan:
	case <-timeOutCtx.Done():
		t.Fatal("timed out")
	}

	select {
	case tInChan <- message.NewTransaction(message.QuickBatch([][]byte{
		[]byte("hello world 5"),
		[]byte("hello world 6"),
		[]byte("hello world 7"),
		[]byte("hello world 8"),
	}), resChanTwo):
	case <-timeOutCtx.Done():
		t.Fatal("timed out")
	}

	var tranTwo message.Transaction
	select {
	case tranTwo = <-tOutChan:
	case <-timeOutCtx.Done():
		t.Fatal("timed out")
	}

	require.NoError(t, tranOne.Ack(timeOutCtx, nil))
	require.NoError(t, tranTwo.Ack(timeOutCtx, nil))

	select {
	case err := <-resChanOne:
		require.NoError(t, err)
	case <-timeOutCtx.Done():
		t.Fatal("timed out")
	}

	select {
	case err := <-resChanTwo:
		require.NoError(t, err)
	case <-timeOutCtx.Done():
		t.Fatal("timed out")
	}

	b.TriggerCloseNow()
	require.NoError(t, b.WaitForClose(timeOutCtx))
}

func TestBatcherBatchError(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*20)
	defer done()

	tInChan := make(chan message.Transaction)
	resChan := make(chan error)

	policyConf := batchconfig.NewConfig()
	policyConf.Count = 4
	batchPol, err := policy.New(policyConf, mock.NewManager())
	require.NoError(t, err)

	out := &mock.OutputChanneled{}

	b := batcher.New(batchPol, out, mock.NewManager())
	require.NoError(t, b.Consume(tInChan))

	b.TriggerStartConsuming()

	tOutChan := out.TChan

	wg := sync.WaitGroup{}

	wg.Go(func() {
		firstErr := errors.New("first error")
		thirdErr := errors.New("third error")

		// Receive first batch on output
		var outTr message.Transaction
		select {
		case outTr = <-tOutChan:
		case <-time.After(time.Second):
			t.Error("Timed out waiting for message read")
		}
		assert.Equal(t, [][]byte{
			[]byte("foo0"),
			[]byte("foo1"),
			[]byte("foo2"),
			[]byte("foo3"),
		}, message.GetAllBytes(outTr.Payload))

		batchErr := batchInternal.NewError(outTr.Payload, errors.New("foo")).
			Failed(0, firstErr).Failed(2, thirdErr)

		require.NoError(t, outTr.Ack(tCtx, batchErr))
	})

	for i := range 4 {
		data := fmt.Appendf(nil, "foo%v", i)
		select {
		case tInChan <- message.NewTransaction(message.QuickBatch([][]byte{data}), resChan):
		case <-time.After(time.Second):
			t.Fatal("timed out")
		}
	}
	for i := range 4 {
		var act error
		select {
		case actRes := <-resChan:
			act = actRes
		case <-time.After(time.Second):
			t.Fatal("timed out")
		}
		switch i {
		case 0:
			assert.EqualError(t, act, "first error")
		case 2:
			assert.EqualError(t, act, "third error")
		default:
			assert.NoError(t, act)
		}
	}

	close(tInChan)
	b.TriggerCloseNow()

	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	require.NoError(t, b.WaitForClose(ctx))
	wg.Wait()
}

// TestBatcherDroppedBatchMisattributesAck guards against pending transactions
// being acknowledged with the result of an unrelated, later batch.
//
// The batcher accumulates upstream transactions while buffering messages and
// resolves them once the resulting batch has been written. When a flush yields
// no batch — because the batch policy processors filtered every message away
// (exercised here), or because the flush context was cancelled — those
// transactions must be resolved against that flush rather than left to inherit
// a future batch's result. Otherwise data that was never delivered can be
// acked as if it succeeded.
//
// Scenario: batch one ("drop") is filtered to nothing by the policy processor,
// so its flush yields no batch. Batch two ("keep") forms a real batch which the
// output nacks with errKeepFailed.
//
// Expected: the "drop" transactions resolve with nil (their data was
// successfully consumed and intentionally filtered) independently of batch two,
// rather than receiving batch two's errKeepFailed result.
func TestBatcherDroppedBatchMisattributesAck(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*20)
	defer done()

	procConf, err := testutil.ProcessorFromYAML(`
mapping: |
  root = if content().string() == "drop" { deleted() }
`)
	require.NoError(t, err)

	policyConf := batchconfig.NewConfig()
	policyConf.Count = 2
	policyConf.Processors = append(policyConf.Processors, procConf)
	batchPol, err := policy.New(policyConf, mock.NewManager())
	require.NoError(t, err)

	out := &mock.OutputChanneled{}
	b := batcher.New(batchPol, out, mock.NewManager())

	tInChan := make(chan message.Transaction)
	require.NoError(t, b.Consume(tInChan))
	b.TriggerStartConsuming()

	errKeepFailed := errors.New("keep batch write failed")

	// The only batch that ever reaches the output is the "keep" batch, which we
	// nack with errKeepFailed.
	wg := sync.WaitGroup{}
	wg.Go(func() {
		select {
		case outTr := <-out.TChan:
			assert.Equal(t, [][]byte{[]byte("keep0"), []byte("keep1")}, message.GetAllBytes(outTr.Payload))
			require.NoError(t, outTr.Ack(tCtx, errKeepFailed))
		case <-tCtx.Done():
			t.Error("timed out waiting for keep batch on output")
		}
	})

	// Buffered so the batcher's ack goroutine never blocks regardless of the
	// order in which the test reads results.
	dropRes := []chan error{make(chan error, 1), make(chan error, 1)}
	keepRes := []chan error{make(chan error, 1), make(chan error, 1)}

	// Batch one: two messages the policy processor filters away. count=2 fires
	// the flush, which yields no batch and is dropped.
	for _, rc := range dropRes {
		select {
		case tInChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte("drop")}), rc):
		case <-tCtx.Done():
			t.Fatal("timed out sending drop message")
		}
	}

	// Batch two: two messages that survive the processor and form a real batch.
	for i, rc := range keepRes {
		select {
		case tInChan <- message.NewTransaction(message.QuickBatch([][]byte{fmt.Appendf(nil, "keep%v", i)}), rc):
		case <-tCtx.Done():
			t.Fatal("timed out sending keep message")
		}
	}

	// The keep transactions belong to the batch that failed, so they are
	// correctly nacked.
	for _, rc := range keepRes {
		select {
		case got := <-rc:
			assert.Equal(t, errKeepFailed, got)
		case <-tCtx.Done():
			t.Fatal("timed out waiting for keep ack")
		}
	}

	// The drop transactions were filtered in a separate, earlier flush and must
	// NOT inherit batch two's failure. On the buggy code they receive
	// errKeepFailed (or never resolve at all), demonstrating the pendingTrans
	// leak.
	for _, rc := range dropRes {
		select {
		case got := <-rc:
			assert.NoError(t, got, "drop transaction wrongly received the keep batch's result (pendingTrans leak)")
		case <-tCtx.Done():
			t.Fatal("drop transaction was never acked (leaked in pendingTrans)")
		}
	}

	close(tInChan)
	b.TriggerCloseNow()
	require.NoError(t, b.WaitForClose(tCtx))
	wg.Wait()
}

func TestBatcherTimed(t *testing.T) {
	tInChan := make(chan message.Transaction)
	resChan := make(chan error)

	policyConf := batchconfig.NewConfig()
	policyConf.Period = "100ms"
	batchPol, err := policy.New(policyConf, mock.NewManager())
	if err != nil {
		t.Fatal(err)
	}

	out := &mock.OutputChanneled{}

	b := batcher.New(batchPol, out, mock.NewManager())
	if err := b.Consume(tInChan); err != nil {
		t.Fatal(err)
	}

	b.TriggerStartConsuming()

	tOutChan := out.TChan

	batchExpected := [][]byte{
		[]byte("foo1"),
		[]byte("foo2"),
		[]byte("foo3"),
	}

	select {
	case tInChan <- message.NewTransaction(message.QuickBatch(batchExpected), resChan):
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for message send")
	}

	// Receive first batch on output
	var outTr message.Transaction
	select {
	case outTr = <-tOutChan:
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for message read")
	}
	if exp, act := batchExpected, message.GetAllBytes(outTr.Payload); !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong result from batch: %s != %s", act, exp)
	}

	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	close(tInChan)
	b.TriggerCloseNow()
	require.NoError(t, b.WaitForClose(ctx))

	close(resChan)
}

// orderedBatch returns a batch whose parts each increment their own slot of
// the returned counters when their ordering is settled.
func orderedBatch(contents ...string) (message.Batch, []*atomic.Int64) {
	batch := make(message.Batch, len(contents))
	fired := make([]*atomic.Int64, len(contents))
	for i, c := range contents {
		counter := &atomic.Int64{}
		fired[i] = counter
		part := message.NewPart([]byte(c))
		batch[i] = part.WithContext(message.CtxOnOrderingSettled(part.GetContext(), func() {
			counter.Add(1)
		}))
	}
	return batch, fired
}

func requireFired(t *testing.T, fired []*atomic.Int64) {
	t.Helper()
	for i, f := range fired {
		require.Eventuallyf(t, func() bool {
			return f.Load() >= 1
		}, time.Second*5, time.Millisecond, "ordering callback of part %v", i)
	}
}

func TestBatcherOrderingSettledOnAdd(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	tInChan := make(chan message.Transaction)

	policyConf := batchconfig.NewConfig()
	policyConf.Count = 5
	batchPol, err := policy.New(policyConf, mock.NewManager())
	require.NoError(t, err)

	out := &mock.OutputChanneled{}
	b := batcher.New(batchPol, out, mock.NewManager(), batcher.OptChildWritesInOrder())
	require.NoError(t, b.Consume(tInChan))
	b.TriggerStartConsuming()

	send := func(resChan chan error, contents ...string) []*atomic.Int64 {
		t.Helper()
		batch, fired := orderedBatch(contents...)
		select {
		case tInChan <- message.NewTransaction(batch, resChan):
		case <-tCtx.Done():
			t.Fatal("timed out")
		}
		return fired
	}

	// Two transactions that do not fill the batch: every part is settled
	// without a flush, and nothing is acknowledged.
	resChanOne, resChanTwo, resChanThree := make(chan error), make(chan error), make(chan error)
	requireFired(t, send(resChanOne, "a"))
	requireFired(t, send(resChanTwo, "b", "c", "d"))

	select {
	case <-out.TChan:
		t.Fatal("unexpected flush")
	case err := <-resChanOne:
		t.Fatalf("unexpected ack: %v", err)
	case err := <-resChanTwo:
		t.Fatalf("unexpected ack: %v", err)
	case <-time.After(time.Millisecond * 50):
	}

	// The transaction that fills the batch is settled as well, including the
	// part that overshoots the count.
	firedThree := send(resChanThree, "e", "f")

	var tran message.Transaction
	select {
	case tran = <-out.TChan:
	case <-tCtx.Done():
		t.Fatal("timed out")
	}
	requireFired(t, firedThree)

	var contents []string
	for _, p := range tran.Payload {
		contents = append(contents, string(p.AsBytes()))
	}
	assert.Equal(t, []string{"a", "b", "c", "d", "e", "f"}, contents)

	// Settling is not an acknowledgement: sources are only acked once the
	// flushed batch is.
	select {
	case err := <-resChanOne:
		t.Fatalf("unexpected ack: %v", err)
	case <-time.After(time.Millisecond * 50):
	}

	require.NoError(t, tran.Ack(tCtx, nil))
	for _, rChan := range []chan error{resChanOne, resChanTwo, resChanThree} {
		select {
		case err := <-rChan:
			require.NoError(t, err)
		case <-tCtx.Done():
			t.Fatal("timed out")
		}
	}

	b.TriggerCloseNow()
	require.NoError(t, b.WaitForClose(tCtx))
}

func TestBatcherOrderingSettledWithNack(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	tInChan := make(chan message.Transaction)

	policyConf := batchconfig.NewConfig()
	policyConf.Count = 2
	batchPol, err := policy.New(policyConf, mock.NewManager())
	require.NoError(t, err)

	out := &mock.OutputChanneled{}
	b := batcher.New(batchPol, out, mock.NewManager(), batcher.OptChildWritesInOrder())
	require.NoError(t, b.Consume(tInChan))
	b.TriggerStartConsuming()

	batch, fired := orderedBatch("a", "b")
	resChan := make(chan error)
	select {
	case tInChan <- message.NewTransaction(batch, resChan):
	case <-tCtx.Done():
		t.Fatal("timed out")
	}

	var tran message.Transaction
	select {
	case tran = <-out.TChan:
	case <-tCtx.Done():
		t.Fatal("timed out")
	}
	requireFired(t, fired)

	// A settled batch can still fail, and the source still hears about it.
	require.NoError(t, tran.Ack(tCtx, errors.New("nope")))
	select {
	case err := <-resChan:
		require.EqualError(t, err, "nope")
	case <-tCtx.Done():
		t.Fatal("timed out")
	}

	b.TriggerCloseNow()
	require.NoError(t, b.WaitForClose(tCtx))
}

func TestBatcherOrderingSettledPeriodOnly(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	tInChan := make(chan message.Transaction)

	policyConf := batchconfig.NewConfig()
	policyConf.Period = "1h"
	batchPol, err := policy.New(policyConf, mock.NewManager())
	require.NoError(t, err)

	out := &mock.OutputChanneled{}
	b := batcher.New(batchPol, out, mock.NewManager(), batcher.OptChildWritesInOrder())
	require.NoError(t, b.Consume(tInChan))
	b.TriggerStartConsuming()

	// Many transactions are accepted and settled long before the period
	// elapses.
	for i := range 20 {
		batch, fired := orderedBatch(fmt.Sprintf("foo %v", i))
		select {
		case tInChan <- message.NewTransaction(batch, make(chan error)):
		case <-tCtx.Done():
			t.Fatal("timed out")
		}
		requireFired(t, fired)
	}

	select {
	case <-out.TChan:
		t.Fatal("unexpected flush")
	case <-time.After(time.Millisecond * 50):
	}

	b.TriggerCloseNow()
	require.NoError(t, b.WaitForClose(tCtx))
}

func TestBatcherOrderingSettledWithFilteringBatchProcessors(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	tInChan := make(chan message.Transaction)

	// The batch processors filter every message on flush, so no batch is sent
	// and the source is acked. The parts are settled when they join the batch,
	// before the processors run.
	policyConf := batchconfig.NewConfig()
	policyConf.Count = 3
	procConf, err := testutil.ProcessorFromYAML(`
mapping: 'root = deleted()'
`)
	require.NoError(t, err)
	policyConf.Processors = append(policyConf.Processors, procConf)
	batchPol, err := policy.New(policyConf, mock.NewManager())
	require.NoError(t, err)

	out := &mock.OutputChanneled{}
	b := batcher.New(batchPol, out, mock.NewManager(), batcher.OptChildWritesInOrder())
	require.NoError(t, b.Consume(tInChan))
	b.TriggerStartConsuming()

	batchOne, firedOne := orderedBatch("a", "b")
	resChanOne := make(chan error)
	select {
	case tInChan <- message.NewTransaction(batchOne, resChanOne):
	case <-tCtx.Done():
		t.Fatal("timed out")
	}
	requireFired(t, firedOne)

	select {
	case err := <-resChanOne:
		t.Fatalf("unexpected ack: %v", err)
	case <-time.After(time.Millisecond * 50):
	}

	batchTwo, firedTwo := orderedBatch("c")
	resChanTwo := make(chan error)
	select {
	case tInChan <- message.NewTransaction(batchTwo, resChanTwo):
	case <-tCtx.Done():
		t.Fatal("timed out")
	}
	requireFired(t, firedTwo)

	for _, rChan := range []chan error{resChanOne, resChanTwo} {
		select {
		case err := <-rChan:
			require.NoError(t, err)
		case tran := <-out.TChan:
			t.Fatalf("unexpected batch of %v", tran.Payload.Len())
		case <-tCtx.Done():
			t.Fatal("timed out")
		}
	}

	b.TriggerCloseNow()
	require.NoError(t, b.WaitForClose(tCtx))
}

// Without the option the child may write batches in parallel, so joining a
// batch says nothing about ordering and no callback is called.
func TestBatcherOrderingNotSettledByDefault(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	tInChan := make(chan message.Transaction)

	policyConf := batchconfig.NewConfig()
	policyConf.Count = 2
	batchPol, err := policy.New(policyConf, mock.NewManager())
	require.NoError(t, err)

	out := &mock.OutputChanneled{}
	b := batcher.New(batchPol, out, mock.NewManager())
	require.NoError(t, b.Consume(tInChan))
	b.TriggerStartConsuming()

	batch, fired := orderedBatch("a", "b")
	resChan := make(chan error)
	select {
	case tInChan <- message.NewTransaction(batch, resChan):
	case <-tCtx.Done():
		t.Fatal("timed out")
	}

	var tran message.Transaction
	select {
	case tran = <-out.TChan:
	case <-tCtx.Done():
		t.Fatal("timed out")
	}
	require.NoError(t, tran.Ack(tCtx, nil))
	select {
	case err := <-resChan:
		require.NoError(t, err)
	case <-tCtx.Done():
		t.Fatal("timed out")
	}

	for i, f := range fired {
		assert.Equal(t, int64(0), f.Load(), "part %v", i)
	}

	b.TriggerCloseNow()
	require.NoError(t, b.WaitForClose(tCtx))
}

// newOrderedBatcher starts a batcher whose child is declared to write in
// order, and returns the channel to feed it and the child to read from.
func newOrderedBatcher(t *testing.T, conf batchconfig.Config) (chan message.Transaction, *mock.OutputChanneled, func(context.Context)) {
	t.Helper()

	batchPol, err := policy.New(conf, mock.NewManager())
	require.NoError(t, err)

	tInChan := make(chan message.Transaction)
	out := &mock.OutputChanneled{}
	b := batcher.New(batchPol, out, mock.NewManager(), batcher.OptChildWritesInOrder())
	require.NoError(t, b.Consume(tInChan))
	b.TriggerStartConsuming()

	return tInChan, out, func(ctx context.Context) {
		b.TriggerCloseNow()
		require.NoError(t, b.WaitForClose(ctx))
	}
}

// Ordering is settled as a message joins the batch, whichever condition of
// the policy is the one that will flush it.
func TestBatcherOrderingSettledForEveryPolicyTrigger(t *testing.T) {
	tests := []struct {
		name string
		conf func(*batchconfig.Config)
	}{
		{name: "count", conf: func(c *batchconfig.Config) { c.Count = 100 }},
		{name: "byte size", conf: func(c *batchconfig.Config) { c.ByteSize = 1000 }},
		{name: "check", conf: func(c *batchconfig.Config) { c.Check = `content() == "end"` }},
		{name: "period", conf: func(c *batchconfig.Config) { c.Period = "1h" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tCtx, done := context.WithTimeout(t.Context(), time.Second*30)
			defer done()

			conf := batchconfig.NewConfig()
			test.conf(&conf)
			tInChan, out, closeFn := newOrderedBatcher(t, conf)

			for i := range 5 {
				batch, fired := orderedBatch(fmt.Sprintf("foo %v", i))
				select {
				case tInChan <- message.NewTransaction(batch, make(chan error)):
				case <-tCtx.Done():
					t.Fatal("timed out")
				}
				requireFired(t, fired)
			}

			select {
			case <-out.TChan:
				t.Fatal("unexpected flush")
			case <-time.After(time.Millisecond * 50):
			}

			closeFn(tCtx)
		})
	}
}

// A message joining a batch is settled even while the previous batch has not
// been resolved, as the child will still write them in order.
func TestBatcherOrderingSettledWhilePreviousBatchUnresolved(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	tInChan, out, closeFn := newOrderedBatcher(t, func() batchconfig.Config {
		conf := batchconfig.NewConfig()
		conf.Count = 2
		return conf
	}())

	send := func(resChan chan error, contents ...string) []*atomic.Int64 {
		t.Helper()
		batch, fired := orderedBatch(contents...)
		select {
		case tInChan <- message.NewTransaction(batch, resChan):
		case <-tCtx.Done():
			t.Fatal("timed out")
		}
		return fired
	}
	receive := func() message.Transaction {
		t.Helper()
		select {
		case tran := <-out.TChan:
			return tran
		case <-tCtx.Done():
			t.Fatal("timed out")
		}
		return message.Transaction{}
	}

	resOne, resTwo := make(chan error, 1), make(chan error, 1)
	requireFired(t, send(resOne, "a", "b"))
	tranOne := receive()

	// The first batch is with the child and unresolved.
	requireFired(t, send(resTwo, "c", "d"))
	tranTwo := receive()

	require.NoError(t, tranOne.Ack(tCtx, nil))
	require.NoError(t, tranTwo.Ack(tCtx, nil))
	for _, rChan := range []chan error{resOne, resTwo} {
		select {
		case err := <-rChan:
			require.NoError(t, err)
		case <-tCtx.Done():
			t.Fatal("timed out")
		}
	}

	closeFn(tCtx)
}

// Every callback a message carries is called, in the order they were added.
func TestBatcherOrderingSettledWithSeveralCallbacks(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	conf := batchconfig.NewConfig()
	conf.Count = 10
	tInChan, _, closeFn := newOrderedBatcher(t, conf)

	var mut sync.Mutex
	var called []string
	record := func(s string) func() {
		return func() {
			mut.Lock()
			called = append(called, s)
			mut.Unlock()
		}
	}

	part := message.NewPart([]byte("a"))
	ctx := message.CtxOnOrderingSettled(part.GetContext(), record("first"))
	ctx = message.CtxOnOrderingSettled(ctx, record("second"))

	select {
	case tInChan <- message.NewTransaction(message.Batch{part.WithContext(ctx)}, make(chan error)):
	case <-tCtx.Done():
		t.Fatal("timed out")
	}

	require.Eventually(t, func() bool {
		mut.Lock()
		defer mut.Unlock()
		return len(called) == 2
	}, time.Second*5, time.Millisecond)
	mut.Lock()
	assert.Equal(t, []string{"first", "second"}, called)
	mut.Unlock()

	closeFn(tCtx)
}

// Messages without callbacks pass through a batcher that settles ordering as
// they do through any other.
func TestBatcherOrderingSettledWithPlainMessages(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	conf := batchconfig.NewConfig()
	conf.Count = 3
	tInChan, out, closeFn := newOrderedBatcher(t, conf)

	// One message with a callback between two without.
	ordered, fired := orderedBatch("b")
	batch := message.Batch{message.NewPart([]byte("a")), ordered[0], message.NewPart([]byte("c"))}
	resChan := make(chan error, 1)
	select {
	case tInChan <- message.NewTransaction(batch, resChan):
	case <-tCtx.Done():
		t.Fatal("timed out")
	}

	select {
	case tran := <-out.TChan:
		require.Equal(t, 3, tran.Payload.Len())
		require.NoError(t, tran.Ack(tCtx, nil))
	case <-tCtx.Done():
		t.Fatal("timed out")
	}
	requireFired(t, fired)
	select {
	case err := <-resChan:
		require.NoError(t, err)
	case <-tCtx.Done():
		t.Fatal("timed out")
	}

	closeFn(tCtx)
}

// A partial batch that is flushed because the input ended was settled when
// its messages joined it, and is still acknowledged on delivery.
func TestBatcherOrderingSettledAtEndOfInput(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	conf := batchconfig.NewConfig()
	conf.Count = 10
	tInChan, out, _ := newOrderedBatcher(t, conf)

	batch, fired := orderedBatch("a", "b")
	resChan := make(chan error, 1)
	select {
	case tInChan <- message.NewTransaction(batch, resChan):
	case <-tCtx.Done():
		t.Fatal("timed out")
	}
	requireFired(t, fired)
	close(tInChan)

	select {
	case tran := <-out.TChan:
		require.Equal(t, 2, tran.Payload.Len())
		require.NoError(t, tran.Ack(tCtx, nil))
	case <-tCtx.Done():
		t.Fatal("timed out")
	}
	select {
	case err := <-resChan:
		require.NoError(t, err)
	case <-tCtx.Done():
		t.Fatal("timed out")
	}
	for _, f := range fired {
		assert.Equal(t, int64(1), f.Load())
	}
}

// A batcher that is closed while it holds settled messages shuts down without
// settling them again.
func TestBatcherClosesWithSettledMessagesPending(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	conf := batchconfig.NewConfig()
	conf.Count = 10
	tInChan, _, closeFn := newOrderedBatcher(t, conf)

	batch, fired := orderedBatch("a", "b")
	select {
	case tInChan <- message.NewTransaction(batch, make(chan error)):
	case <-tCtx.Done():
		t.Fatal("timed out")
	}
	requireFired(t, fired)

	closeFn(tCtx)
	for _, f := range fired {
		assert.Equal(t, int64(1), f.Load())
	}
}

// With no batch policy there is no batcher, so the option has nothing to act
// on and the child is returned as it is.
func TestBatcherFromConfigNoopPolicyIgnoresOption(t *testing.T) {
	out := &mock.OutputChanneled{}

	res, err := batcher.NewFromConfig(batchconfig.NewConfig(), out, mock.NewManager(), batcher.OptChildWritesInOrder())
	require.NoError(t, err)
	assert.Same(t, out, res)

	conf := batchconfig.NewConfig()
	conf.Count = 2
	res, err = batcher.NewFromConfig(conf, out, mock.NewManager(), batcher.OptChildWritesInOrder())
	require.NoError(t, err)
	assert.NotSame(t, out, res)
}
