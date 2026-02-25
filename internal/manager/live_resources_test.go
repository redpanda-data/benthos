// Copyright 2026 Redpanda Data, Inc.

package manager

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveResourcesRWalkCancelledContextReturnsError(t *testing.T) {
	var initCount atomic.Int64

	lr := newLiveResources[string]()
	lr.LazyAdd("foo", func(ctx context.Context) (*string, error) {
		initCount.Add(1)
		s := "hello"
		return &s, nil
	})

	// RWalk with a cancelled context now propagates the context.Canceled
	// error from RAccess (instead of silently skipping).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var walked []string
	err := lr.RWalk(ctx, func(name string, s string) error {
		walked = append(walked, name)
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)

	assert.Equal(t, int64(0), initCount.Load(), "lazy init should not have been triggered")
	assert.Empty(t, walked, "no resources should have been walked")

	// Resources should still be probeable even though they were not initialized.
	assert.True(t, lr.Probe("foo"))

	// Accessing individually with a live context should trigger init.
	err = lr.RAccess(context.Background(), "foo", func(s string) {
		assert.Equal(t, "hello", s)
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), initCount.Load(), "only the accessed resource should have been initialized")
}

func TestLiveResourcesRWalkLiveContextTriggersLazyInit(t *testing.T) {
	var initCount atomic.Int64

	lr := newLiveResources[string]()
	lr.LazyAdd("foo", func(ctx context.Context) (*string, error) {
		initCount.Add(1)
		s := "hello"
		return &s, nil
	})
	lr.LazyAdd("bar", func(ctx context.Context) (*string, error) {
		initCount.Add(1)
		s := "world"
		return &s, nil
	})

	// RWalk with a live context triggers lazy initialization.
	walked := map[string]string{}
	err := lr.RWalk(context.Background(), func(name string, s string) error {
		walked[name] = s
		return nil
	})
	require.NoError(t, err)

	assert.Equal(t, int64(2), initCount.Load(), "both resources should have been initialized")
	assert.Equal(t, map[string]string{"foo": "hello", "bar": "world"}, walked)
}

func TestLiveResourceRAccessCancelledContextSkipsLazyInit(t *testing.T) {
	var initCount atomic.Int64

	lr := &liveResource[string]{
		lazyRes: func(ctx context.Context) (*string, error) {
			initCount.Add(1)
			s := "hello"
			return &s, nil
		},
	}

	// RAccess with a cancelled context must not trigger lazy init.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ok, err := lr.RAccess(ctx, func(s string) {
		t.Fatal("callback should not be called")
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, ok)
	assert.Equal(t, int64(0), initCount.Load(), "lazy init should not have been triggered")

	// Subsequent access with a live context should succeed.
	ok, err = lr.RAccess(context.Background(), func(s string) {
		assert.Equal(t, "hello", s)
	})
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int64(1), initCount.Load())
}

func TestLiveResourceLazyInitRetryAfterError(t *testing.T) {
	var attempt atomic.Int64

	lr := &liveResource[string]{
		lazyRes: func(ctx context.Context) (*string, error) {
			n := attempt.Add(1)
			if n == 1 {
				return nil, assert.AnError
			}
			s := "recovered"
			return &s, nil
		},
	}

	// First access fails with an error.
	ok, err := lr.RAccess(context.Background(), func(s string) {
		t.Fatal("callback should not be called on error")
	})
	require.ErrorIs(t, err, assert.AnError)
	assert.False(t, ok)
	assert.Equal(t, int64(1), attempt.Load())

	// lazyRes is NOT cleared on error, so a subsequent access retries.
	ok, err = lr.RAccess(context.Background(), func(s string) {
		assert.Equal(t, "recovered", s)
	})
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int64(2), attempt.Load())

	// Now initialized; further accesses should not call lazyRes again.
	ok, err = lr.RAccess(context.Background(), func(s string) {
		assert.Equal(t, "recovered", s)
	})
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int64(2), attempt.Load(), "should not re-init after success")
}

func TestLiveResourcesWalkCancelledContextSkipsLazyInit(t *testing.T) {
	var initCount atomic.Int64

	lr := newLiveResources[string]()
	lr.LazyAdd("foo", func(ctx context.Context) (*string, error) {
		initCount.Add(1)
		s := "hello"
		return &s, nil
	})
	lr.LazyAdd("bar", func(ctx context.Context) (*string, error) {
		initCount.Add(1)
		s := "world"
		return &s, nil
	})

	// Walk with a cancelled context must not trigger lazy initialization.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var walked []string
	err := lr.Walk(ctx, func(name string, s *string, set func(*string)) error {
		walked = append(walked, name)
		return nil
	})
	require.NoError(t, err)

	assert.Equal(t, int64(0), initCount.Load(), "lazy init should not have been triggered")
	assert.Empty(t, walked, "no resources should have been walked")

	// Resources should still be probeable.
	assert.True(t, lr.Probe("foo"))
	assert.True(t, lr.Probe("bar"))
}

func TestLiveResourcesRWalkInitialized(t *testing.T) {
	var initCount atomic.Int64

	lr := newLiveResources[string]()

	// Pre-initialize "foo" by accessing it.
	lr.LazyAdd("foo", func(ctx context.Context) (*string, error) {
		initCount.Add(1)
		s := "hello"
		return &s, nil
	})
	lr.LazyAdd("bar", func(ctx context.Context) (*string, error) {
		initCount.Add(1)
		s := "world"
		return &s, nil
	})

	// Initialize only "foo".
	err := lr.RAccess(context.Background(), "foo", func(s string) {
		assert.Equal(t, "hello", s)
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), initCount.Load())

	// RWalkInitialized should only visit "foo", skipping the lazy "bar".
	walked := map[string]string{}
	err = lr.RWalkInitialized(func(name string, s string) error {
		walked[name] = s
		return nil
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"foo": "hello"}, walked)
	assert.Equal(t, int64(1), initCount.Load(), "lazy init should not have been triggered for bar")

	// "bar" should still be probeable.
	assert.True(t, lr.Probe("bar"))
}

func TestLiveResourcesWalkInitializedSkipsLazy(t *testing.T) {
	lr := newLiveResources[string]()

	lr.LazyAdd("foo", func(ctx context.Context) (*string, error) {
		s := "hello"
		return &s, nil
	})
	lr.LazyAdd("bar", func(ctx context.Context) (*string, error) {
		s := "world"
		return &s, nil
	})

	// Initialize only "foo".
	err := lr.RAccess(context.Background(), "foo", func(s string) {})
	require.NoError(t, err)

	// WalkInitialized should visit only "foo", skipping the uninitialized "bar".
	walked := map[string]string{}
	err = lr.WalkInitialized(func(name string, s *string, set func(*string)) error {
		walked[name] = *s
		set(nil) // explicitly remove
		return nil
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"foo": "hello"}, walked)

	// "foo" was explicitly set(nil) so it's gone.
	assert.False(t, lr.Probe("foo"))
	// "bar" was never initialized — it should still be in the map.
	assert.True(t, lr.Probe("bar"))
}

func TestLiveResourcesRWalkSnapshotAllowsProbe(t *testing.T) {
	lr := newLiveResources[string]()

	lr.LazyAdd("foo", func(ctx context.Context) (*string, error) {
		// During lazy init, probe the same map — this would deadlock
		// with the old RWalk that held the outer RLock during iteration.
		assert.True(t, lr.Probe("bar"))
		s := "hello"
		return &s, nil
	})
	lr.LazyAdd("bar", func(ctx context.Context) (*string, error) {
		s := "world"
		return &s, nil
	})

	// RWalk with a live context triggers lazy init for "foo", whose
	// constructor probes "bar" on the same map. With the snapshot pattern
	// this must not deadlock.
	walked := map[string]string{}
	err := lr.RWalk(context.Background(), func(name string, s string) error {
		walked[name] = s
		return nil
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"foo": "hello", "bar": "world"}, walked)
}

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
