// Copyright 2026 Redpanda Data, Inc.

package ordering_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/redpanda-data/benthos/v4/public/service"
	"github.com/redpanda-data/benthos/v4/public/utils/ordering"
)

func TestSettledWithoutCallbacks(t *testing.T) {
	msg := service.NewMessage([]byte("foo"))
	assert.NotPanics(t, func() {
		ordering.Settled(msg)
	})

	// A nil callback is ignored rather than left to panic when the message is
	// settled.
	withNil := ordering.OnSettled(msg, nil)
	assert.Same(t, msg, withNil)
	assert.NotPanics(t, func() {
		ordering.Settled(withNil)
	})
}

func TestOnSettledCallbacks(t *testing.T) {
	var fired []string
	record := func(s string) func() {
		return func() { fired = append(fired, s) }
	}

	msg := service.NewMessage([]byte("foo"))
	first := ordering.OnSettled(msg, record("first"))
	second := ordering.OnSettled(first, record("second"))

	// The original message is not changed by adding a callback to a copy.
	ordering.Settled(msg)
	assert.Empty(t, fired)

	ordering.Settled(first)
	assert.Equal(t, []string{"first"}, fired)

	// A callback is called at most once, however many times the message or a
	// copy of it is settled.
	ordering.Settled(second.Copy())
	ordering.Settled(second)
	ordering.Settled(first)
	assert.Equal(t, []string{"first", "second"}, fired)
}

func TestOnSettledOnceAcrossGoroutines(t *testing.T) {
	var fired atomic.Int64
	msg := ordering.OnSettled(service.NewMessage([]byte("foo")), func() {
		fired.Add(1)
	})

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			ordering.Settled(msg.Copy())
		})
	}
	wg.Wait()

	assert.Equal(t, int64(1), fired.Load())
}

// The callbacks travel on the context of a message, so a component that
// gives a message an unrelated context drops them.
func TestSettledAfterContextReplaced(t *testing.T) {
	var fired int
	msg := ordering.OnSettled(service.NewMessage([]byte("foo")), func() { fired++ })

	type fooKeyType int
	var fooKey fooKeyType

	// A context derived from the message's own keeps the callbacks.
	derived := msg.WithContext(context.WithValue(msg.Context(), fooKey, "bar"))
	// One that is not derived from it does not.
	replaced := msg.WithContext(context.Background())

	ordering.Settled(replaced)
	assert.Equal(t, 0, fired)

	ordering.Settled(derived)
	assert.Equal(t, 1, fired)
}
