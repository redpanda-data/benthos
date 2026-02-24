// Copyright 2025 Redpanda Data, Inc.

package manager

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveResourceRAccessConcurrentLazyInit(t *testing.T) {
	var initCount atomic.Int64

	lr := &liveResource[string]{
		lazyRes: func(ctx context.Context) (*string, error) {
			initCount.Add(1)
			s := "hello"
			return &s, nil
		},
	}

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	results := make([]string, numGoroutines)
	errs := make([]error, numGoroutines)
	oks := make([]bool, numGoroutines)

	for i := range numGoroutines {
		go func(idx int) {
			defer wg.Done()
			ok, err := lr.RAccess(context.Background(), func(s string) {
				results[idx] = s
			})
			oks[idx] = ok
			errs[idx] = err
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int64(1), initCount.Load(), "initializer should be called exactly once")
	for i := range numGoroutines {
		require.NoError(t, errs[i], "goroutine %d", i)
		assert.True(t, oks[i], "goroutine %d", i)
		assert.Equal(t, "hello", results[i], "goroutine %d", i)
	}
}
