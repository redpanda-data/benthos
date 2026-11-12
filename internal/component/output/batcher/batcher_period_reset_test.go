// Copyright 2025 Redpanda Data, Inc.

package batcher_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/batch/policy"
	"github.com/redpanda-data/benthos/v4/internal/batch/policy/batchconfig"
	"github.com/redpanda-data/benthos/v4/internal/component/output/batcher"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

// A count-triggered flush starts a new period: the next period flush must come a full
// period after it, not at the deadline that was armed before it.
func TestBatcherCountFlushResetsPeriod(t *testing.T) {
	const period = 200 * time.Millisecond

	tInChan := make(chan message.Transaction)
	resChan := make(chan error, 2)

	policyConf := batchconfig.NewConfig()
	policyConf.Count = 4
	policyConf.Period = period.String()
	batchPol, err := policy.New(policyConf, mock.NewManager())
	require.NoError(t, err)

	out := &mock.OutputChanneled{}
	b := batcher.New(batchPol, out, mock.NewManager())
	require.NoError(t, b.Consume(tInChan))
	b.TriggerStartConsuming()
	t.Cleanup(func() {
		b.TriggerCloseNow()
	})

	// The first period deadline is now+period. Fill the count before it.
	time.Sleep(period * 6 / 10)
	tInChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d")}), resChan)

	var countFlushAt time.Time
	select {
	case tr := <-out.TChan:
		countFlushAt = time.Now()
		require.Equal(t, 4, tr.Payload.Len())
		require.NoError(t, tr.Ack(t.Context(), nil))
	case <-time.After(time.Second):
		t.Fatal("no count flush")
	}

	// One more message: it must wait for a full period after the count flush.
	tInChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte("e")}), resChan)
	select {
	case tr := <-out.TChan:
		waited := time.Since(countFlushAt)
		require.Equal(t, 1, tr.Payload.Len())
		require.NoError(t, tr.Ack(t.Context(), nil))
		require.GreaterOrEqual(t, waited, period*8/10,
			"period flush came %v after the count flush; the pre-flush deadline was still armed", waited)
	case <-time.After(2 * time.Second):
		t.Fatal("no period flush")
	}
}
