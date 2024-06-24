package pure

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/redpanda-data/benthos/v4/public/service"
	"github.com/stretchr/testify/require"
)

func keyedBufFromConf(t *testing.T, conf string, clock utcNowProvider) *keyedWindowBuffer {
	t.Helper()

	parsedConf, err := keyedWindowBufferConfig().ParseYAML(conf, nil)
	require.NoError(t, err)

	buf, err := newKeyedWindowBufferFromConf(parsedConf, service.MockResources(), clock)
	require.NoError(t, err)

	return buf
}

func kwbGetTestMessages() []*service.Message {
	return []*service.Message{
		service.NewMessage([]byte(`{"sequence": "test-1", "length": 4, "id":"1","ts":9.85}`)),
		service.NewMessage([]byte(`{"sequence": "test-1", "length": 4, "id":"2","ts":9.9}`)),
		service.NewMessage([]byte(`{"sequence": "test-1", "length": 4, "id":"3","ts":10.15}`)),
		service.NewMessage([]byte(`{"sequence": "test-1", "length": 4, "id":"4","ts":10.3}`)),
		service.NewMessage([]byte(`{"sequence": "test-2", "length": 7, "id":"5","ts":10.5}`)),
		service.NewMessage([]byte(`{"sequence": "test-2", "length": 7, "id":"6","ts":10.7}`)),
		service.NewMessage([]byte(`{"sequence": "test-2", "length": 7, "id":"7","ts":10.9}`)),
		service.NewMessage([]byte(`{"sequence": "test-2", "length": 7, "id":"8","ts":11.1}`)),
		service.NewMessage([]byte(`{"sequence": "test-2", "length": 7, "id":"9","ts":11.35}`)),
		service.NewMessage([]byte(`{"sequence": "test-2", "length": 7, "id":"10","ts":11.52}`)),
		service.NewMessage([]byte(`{"sequence": "test-2", "length": 7, "id":"11","ts":11.8}`)),
		service.NewMessage([]byte(`{"sequence": "test-3", "length": 7, "id":"11","ts":11.8}`)),
		service.NewMessage([]byte(`{"sequence": "test-3", "length": 7, "id":"11","ts":11.8}`)),
		service.NewMessage([]byte(`{"sequence": "test-3", "length": 7, "id":"11","ts":11.8}`)),
		service.NewMessage([]byte(`{"sequence": "test-3", "length": 7, "id":"11","ts":11.8}`)),
	}
}

type kwbFakeAck struct {
	ack  int
	nack int

	totalAck  *int
	totalNack *int
}

func (f *kwbFakeAck) Acknowledge(ctx context.Context, err error) error {
	if err == nil {
		*f.totalAck++
		f.ack++
	} else {
		*f.totalNack++
		f.nack++
	}
	return nil
}

func TestMaxPendingKeys(t *testing.T) {
	ctx := context.Background()

	timeProvider := time.Now

	clock := func() time.Time {
		return timeProvider().UTC()
	}

	block := keyedBufFromConf(t, `
key_mapping: ${! json("sequence") }
max_pending_keys: 2
timeout: 1s
`, clock)
	defer block.Close(ctx)

	messages := kwbGetTestMessages()

	totalAck := 0
	totalNack := 0

	for _, msg := range messages {
		ack := &kwbFakeAck{totalAck: &totalAck, totalNack: &totalNack}
		err := block.WriteBatch(
			ctx,
			service.MessageBatch{msg},
			ack.Acknowledge,
		)

		s, _ := msg.AsStructured()
		if s.(map[string]interface{})["sequence"] == "test-3" {
			// This batch should be rejected due to the buffer being full
			require.GreaterOrEqual(t, ack.nack, 1)
		} else {
			require.NoError(t, err)
		}
	}

	// Set the time to be 100 seconds in the future
	timeProvider = func() time.Time {
		return time.Now().Add(time.Second * 100)
	}

	block.refreshTimeoutChan <- struct{}{}

	t.Log("Reading batches")

	require.Len(t, block.pending, 2, "Expected 2 pending keys")
	require.Equal(t, 4, totalNack, "Expected 4 nacks")
}

func TestKeyedWindow(t *testing.T) {
	ctx := context.Background()

	timeProvider := time.Now

	clock := func() time.Time {
		return timeProvider().UTC()
	}

	block := keyedBufFromConf(t, `
key_mapping: ${! json("sequence") }
length_mapping: ${! json("length") }
max_pending_keys: 10
timeout: 1s
check: meta("batch_expected_length") == meta("batch_length")
`, clock)
	defer block.Close(ctx)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		messages := kwbGetTestMessages()
		for _, msg := range messages {
			err := block.WriteBatch(
				ctx,
				service.MessageBatch{msg},
				func(ctx context.Context, err error) error { return nil },
			)

			if err != nil {
				t.Error(err)
				require.NoError(t, err)
			}
		}
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()
		// First batch2 should be ready
		batch1, _, err := block.ReadBatch(ctx)
		require.NoError(t, err)

		// Second batch should be ready
		batch2, _, err := block.ReadBatch(ctx)
		require.NoError(t, err)

		// Require that either batch has 4 or 7 and the other has the other

		require.Condition(t, func() bool {
			return (len(batch2) == 4 && len(batch1) == 7) ||
				(len(batch2) == 7 && len(batch1) == 4)
		}, "Batch 1 expected 4 messages and batch 2 expected 7 messages")

		// Set the time to be 100 seconds in the future
		timeProvider = func() time.Time {
			return time.Now().Add(time.Second * 100)
		}

		// Trigger a refresh
		block.refreshTimeoutChan <- struct{}{}

		time.Sleep(time.Millisecond * 100)

		// Third partial batch should be ready
		batch3, _, err := block.ReadBatch(ctx)
		require.NoError(t, err)
		require.Len(t, batch3, 4)
	}()

	wg.Wait()
}
