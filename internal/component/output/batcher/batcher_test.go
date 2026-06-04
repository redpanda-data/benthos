// Copyright 2025 Redpanda Data, Inc.

package batcher_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
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
