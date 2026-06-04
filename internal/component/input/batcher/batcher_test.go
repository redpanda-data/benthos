// Copyright 2025 Redpanda Data, Inc.

package batcher_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ibatch "github.com/redpanda-data/benthos/v4/internal/batch"
	"github.com/redpanda-data/benthos/v4/internal/batch/policy"
	"github.com/redpanda-data/benthos/v4/internal/batch/policy/batchconfig"
	"github.com/redpanda-data/benthos/v4/internal/component/input/batcher"
	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	"github.com/redpanda-data/benthos/v4/internal/log"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"

	_ "github.com/redpanda-data/benthos/v4/internal/impl/pure"
)

func TestBatcherStandard(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*5)
	defer done()

	mockInput := &mock.Input{
		TChan: make(chan message.Transaction),
	}

	batchConf := batchconfig.NewConfig()
	batchConf.Count = 3

	batchPol, err := policy.New(batchConf, mock.NewManager())
	if err != nil {
		t.Fatal(err)
	}

	batcher := batcher.New(batchPol, mockInput, log.Noop())
	batcher.TriggerStartConsuming()

	testMsgs := []string{}
	testResChans := []chan error{}
	for i := range 8 {
		testMsgs = append(testMsgs, fmt.Sprintf("test%v", i))
		testResChans = append(testResChans, make(chan error))
	}

	resErrs := []error{}
	doneWritesChan := make(chan struct{})
	doneReadsChan := make(chan struct{})
	go func() {
		for i, m := range testMsgs {
			mockInput.TChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte(m)}), testResChans[i])
		}
		close(doneWritesChan)
		for _, rChan := range testResChans {
			resErrs = append(resErrs, (<-rChan))
		}
		close(doneReadsChan)
	}()

	resFns := []func(context.Context, error) error{}

	var tran message.Transaction
	select {
	case tran = <-batcher.TransactionChan():
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
	{
		tmpTran := tran
		resFns = append(resFns, tmpTran.Ack)
	}

	if exp, act := 3, tran.Payload.Len(); exp != act {
		t.Errorf("Wrong batch size: %v != %v", act, exp)
	}
	_ = tran.Payload.Iter(func(i int, part *message.Part) error {
		if exp, act := fmt.Sprintf("test%v", i), string(part.AsBytes()); exp != act {
			t.Errorf("Unexpected message part: %v != %v", act, exp)
		}
		return nil
	})

	select {
	case tran = <-batcher.TransactionChan():
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
	{
		tmpTran := tran
		resFns = append(resFns, tmpTran.Ack)
	}

	if exp, act := 3, tran.Payload.Len(); exp != act {
		t.Errorf("Wrong batch size: %v != %v", act, exp)
	}
	_ = tran.Payload.Iter(func(i int, part *message.Part) error {
		if exp, act := fmt.Sprintf("test%v", i+3), string(part.AsBytes()); exp != act {
			t.Errorf("Unexpected message part: %v != %v", act, exp)
		}
		return nil
	})

	select {
	case <-batcher.TransactionChan():
		t.Error("Unexpected batch received")
	default:
	}

	select {
	case <-doneWritesChan:
	case <-time.After(time.Second):
		t.Error("timed out")
	}
	batcher.TriggerStopConsuming()

	select {
	case tran = <-batcher.TransactionChan():
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
	{
		tmpTran := tran
		resFns = append(resFns, tmpTran.Ack)
	}

	if exp, act := 2, tran.Payload.Len(); exp != act {
		t.Errorf("Wrong batch size: %v != %v", act, exp)
	}
	_ = tran.Payload.Iter(func(i int, part *message.Part) error {
		if exp, act := fmt.Sprintf("test%v", i+6), string(part.AsBytes()); exp != act {
			t.Errorf("Unexpected message part: %v != %v", act, exp)
		}
		return nil
	})

	for i, rFn := range resFns {
		require.NoError(t, rFn(tCtx, fmt.Errorf("testerr%v", i)))
	}

	select {
	case <-doneReadsChan:
	case <-time.After(time.Second):
		t.Error("timed out")
	}

	for i, err := range resErrs {
		exp := "testerr0"
		if i >= 3 {
			exp = "testerr1"
		}
		if i >= 6 {
			exp = "testerr2"
		}
		if act := err.Error(); exp != act {
			t.Errorf("Unexpected error returned: %v != %v", act, exp)
		}
	}

	if err := batcher.WaitForClose(tCtx); err != nil {
		t.Error(err)
	}
}

// TestBatcherDroppedBatchMisattributesAck guards against pending transactions
// being acknowledged with the result of an unrelated, later batch. It mirrors
// the output batcher's regression test for the same accounting hazard.
//
// The batcher accumulates upstream transactions while buffering messages and
// resolves them once the resulting batch has been written. When a flush yields
// no batch — because the batch policy processors filtered every message away
// (exercised here), or because they returned an error — those transactions must
// be resolved against that flush rather than left to inherit a future batch's
// result.
//
// Scenario: batch one ("drop") is filtered to nothing by the policy processor,
// so its flush yields no batch. Batch two ("keep") forms a real batch which the
// downstream consumer nacks with errKeepFailed.
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

	mockInput := &mock.Input{TChan: make(chan message.Transaction)}

	batchConf := batchconfig.NewConfig()
	batchConf.Count = 2
	batchConf.Processors = append(batchConf.Processors, procConf)
	batchPol, err := policy.New(batchConf, mock.NewManager())
	require.NoError(t, err)

	b := batcher.New(batchPol, mockInput, log.Noop())
	b.TriggerStartConsuming()

	errKeepFailed := errors.New("keep batch write failed")

	// Buffered so the inline acknowledgement of filtered transactions never
	// blocks the batcher loop.
	dropRes := []chan error{make(chan error, 1), make(chan error, 1)}
	keepRes := []chan error{make(chan error, 1), make(chan error, 1)}

	// Batch one: two messages the policy processor filters away. count=2 fires
	// the flush, which yields no batch and is dropped.
	for _, rc := range dropRes {
		select {
		case mockInput.TChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte("drop")}), rc):
		case <-tCtx.Done():
			t.Fatal("timed out sending drop message")
		}
	}

	// Batch two: two messages that survive the processor and form a real batch.
	for i, rc := range keepRes {
		select {
		case mockInput.TChan <- message.NewTransaction(message.QuickBatch([][]byte{fmt.Appendf(nil, "keep%v", i)}), rc):
		case <-tCtx.Done():
			t.Fatal("timed out sending keep message")
		}
	}

	// Receive the keep batch and nack it.
	select {
	case tran := <-b.TransactionChan():
		assert.Equal(t, [][]byte{[]byte("keep0"), []byte("keep1")}, message.GetAllBytes(tran.Payload))
		require.NoError(t, tran.Ack(tCtx, errKeepFailed))
	case <-tCtx.Done():
		t.Fatal("timed out waiting for keep batch")
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
	// NOT inherit batch two's failure.
	for _, rc := range dropRes {
		select {
		case got := <-rc:
			assert.NoError(t, got, "drop transaction wrongly received the keep batch's result")
		case <-tCtx.Done():
			t.Fatal("drop transaction was never acked")
		}
	}

	mockInput.TriggerStopConsuming()
	require.NoError(t, b.WaitForClose(tCtx))
}

func TestBatcherErrorTracking(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*5)
	defer done()

	mockInput := &mock.Input{
		TChan: make(chan message.Transaction),
	}

	batchConf := batchconfig.NewConfig()
	batchConf.Count = 3

	batchPol, err := policy.New(batchConf, mock.NewManager())
	require.NoError(t, err)

	batcher := batcher.New(batchPol, mockInput, log.Noop())

	testMsgs := []string{}
	testResChans := []chan error{}
	for i := range 3 {
		testMsgs = append(testMsgs, fmt.Sprintf("test%v", i))
		testResChans = append(testResChans, make(chan error))
	}

	batcher.TriggerStartConsuming()

	resErrs := []error{}
	doneReadsChan := make(chan struct{})
	go func() {
		for i, m := range testMsgs {
			mockInput.TChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte(m)}), testResChans[i])
		}
		for _, rChan := range testResChans {
			resErrs = append(resErrs, (<-rChan))
		}
		close(doneReadsChan)
	}()

	var tran message.Transaction
	select {
	case tran = <-batcher.TransactionChan():
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	assert.Equal(t, 3, tran.Payload.Len())
	_ = tran.Payload.Iter(func(i int, part *message.Part) error {
		assert.Equal(t, fmt.Sprintf("test%v", i), string(part.AsBytes()))
		return nil
	})

	batchErr := ibatch.NewError(tran.Payload, errors.New("ignore this"))
	batchErr.Failed(1, errors.New("message specific error"))
	require.NoError(t, tran.Ack(tCtx, batchErr))

	select {
	case <-doneReadsChan:
	case <-time.After(time.Second * 5):
		t.Fatal("timed out")
	}

	require.Len(t, resErrs, 3)
	assert.NoError(t, resErrs[0])
	assert.EqualError(t, resErrs[1], "message specific error")
	assert.NoError(t, resErrs[2])

	mockInput.TriggerStopConsuming()
	require.NoError(t, batcher.WaitForClose(tCtx))
}

func TestBatcherTiming(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*5)
	defer done()

	mockInput := &mock.Input{
		TChan: make(chan message.Transaction),
	}

	batchConf := batchconfig.NewConfig()
	batchConf.Count = 0
	batchConf.Period = "1ms"

	batchPol, err := policy.New(batchConf, mock.NewManager())
	if err != nil {
		t.Fatal(err)
	}

	batcher := batcher.New(batchPol, mockInput, log.Noop())
	batcher.TriggerStartConsuming()

	resChan := make(chan error)
	select {
	case mockInput.TChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte("foo1")}), resChan):
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	var tran message.Transaction
	select {
	case tran = <-batcher.TransactionChan():
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	if exp, act := 1, tran.Payload.Len(); exp != act {
		t.Errorf("Wrong batch size: %v != %v", act, exp)
	}
	if exp, act := "foo1", string(tran.Payload.Get(0).AsBytes()); exp != act {
		t.Errorf("Unexpected message part: %v != %v", act, exp)
	}

	errSend := errors.New("this is a test error")
	require.NoError(t, tran.Ack(tCtx, errSend))
	select {
	case err := <-resChan:
		if err != errSend {
			t.Errorf("Unexpected error: %v != %v", err, errSend)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	select {
	case mockInput.TChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte("foo2")}), resChan):
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	select {
	case tran = <-batcher.TransactionChan():
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	if exp, act := 1, tran.Payload.Len(); exp != act {
		t.Errorf("Wrong batch size: %v != %v", act, exp)
	}
	if exp, act := "foo2", string(tran.Payload.Get(0).AsBytes()); exp != act {
		t.Errorf("Unexpected message part: %v != %v", act, exp)
	}

	batcher.TriggerStopConsuming()

	require.NoError(t, tran.Ack(tCtx, errSend))
	select {
	case err := <-resChan:
		if err != errSend {
			t.Errorf("Unexpected error: %v != %v", err, errSend)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	if err := batcher.WaitForClose(tCtx); err != nil {
		t.Error(err)
	}
}

func TestBatcherFinalFlush(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*5)
	defer done()

	mockInput := &mock.Input{
		TChan: make(chan message.Transaction),
	}

	batchConf := batchconfig.NewConfig()
	batchConf.Count = 10

	batchPol, err := policy.New(batchConf, mock.NewManager())
	require.NoError(t, err)

	batcher := batcher.New(batchPol, mockInput, log.Noop())

	batcher.TriggerStartConsuming()

	resChan := make(chan error, 1)
	select {
	case mockInput.TChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte("foo1")}), resChan):
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	mockInput.TriggerStopConsuming()

	var tran message.Transaction
	select {
	case tran = <-batcher.TransactionChan():
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	if exp, act := 1, tran.Payload.Len(); exp != act {
		t.Errorf("Wrong batch size: %v != %v", act, exp)
	}
	if exp, act := "foo1", string(tran.Payload.Get(0).AsBytes()); exp != act {
		t.Errorf("Unexpected message part: %v != %v", act, exp)
	}

	batcher.TriggerStopConsuming()
	require.NoError(t, tran.Ack(tCtx, nil))

	if err := batcher.WaitForClose(tCtx); err != nil {
		t.Error(err)
	}
}
