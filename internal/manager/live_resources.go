// Copyright 2025 Redpanda Data, Inc.

package manager

import (
	"context"
	"sync"
)

type liveResource[T any] struct {
	res     *T
	lazyRes func(context.Context) (*T, error)
	m       sync.RWMutex
}

// Access the underlying resource in writeable mode, where mutations within the
// provided closure are safe on the resource, and the set function can be used
// to change or delete (nil argument) the underlying resource. Returns true if
// the resource remains non-nil after it was accessed.
func (l *liveResource[T]) Access(ctx context.Context, fn func(t *T, set func(t *T), err error)) bool {
	if l == nil {
		return false
	}

	l.m.Lock()
	defer l.m.Unlock()

	if l.res == nil && l.lazyRes != nil {
		// Never call the initialization function if the context is already
		// cancelled.
		if ctx.Err() != nil {
			return true
		}

		res, err := l.lazyRes(ctx)
		if err != nil {
			fn(nil, func(t *T) {
				l.res = t
				l.lazyRes = nil
			}, err)
			return l.res != nil || l.lazyRes != nil
		}

		l.res = res
		l.lazyRes = nil
	}

	fn(l.res, func(t *T) {
		l.res = t
		l.lazyRes = nil
	}, nil)
	return l.res != nil
}

// RAccess grants a closure function access to the underlying resource, but
// mutations must not be performed on the resource itself. Returns true if the
// resource is non-nil.
func (l *liveResource[T]) RAccess(ctx context.Context, fn func(t T)) (bool, error) {
	if l == nil {
		return false, nil
	}

	l.m.RLock()
	defer l.m.RUnlock()

	if l.res == nil {
		if l.lazyRes == nil {
			return false, nil
		}

		// Never call the initialization function if the context is already
		// cancelled.
		if ctx.Err() != nil {
			return false, ctx.Err()
		}

		res, err := l.lazyRes(ctx)
		if err != nil {
			return false, err
		}
		l.res = res
		l.lazyRes = nil
	}

	fn(*l.res)
	return true, nil
}

//------------------------------------------------------------------------------

type liveResources[T any] struct {
	resources map[string]*liveResource[T]
	m         sync.RWMutex
}

func newLiveResources[T any]() *liveResources[T] {
	return &liveResources[T]{
		resources: map[string]*liveResource[T]{},
	}
}

// Probe checks whether a given resource name is known. This, however, does not
// check that the underlying resource exists at this moment in time.
func (l *liveResources[T]) Probe(name string) bool {
	l.m.RLock()
	_, exists := l.resources[name]
	l.m.RUnlock()
	return exists
}

// LazyAdd registers a resource under a given label and a lazily evaluated
// initialization function. This function is only called if the resource is
// accessed.
func (l *liveResources[T]) LazyAdd(name string, fn func(context.Context) (*T, error)) {
	l.m.Lock()
	l.resources[name] = &liveResource[T]{
		lazyRes: fn,
	}
	l.m.Unlock()
}

// Walk all resources, executing a closure function that is permitted to mutate
// (or delete) the resource.
func (l *liveResources[T]) Walk(ctx context.Context, fn func(name string, t *T, set func(t *T)) error) (err error) {
	l.m.Lock()
	defer l.m.Unlock()

	for k, v := range l.resources {
		if exists := v.Access(ctx, func(t *T, set func(t *T), ierr error) {
			if t != nil {
				err = fn(k, t, set)
			}
		}); !exists {
			delete(l.resources, k)
		}
		if err != nil {
			return
		}
	}
	return nil
}

// RWalk walks all resources, executing a closure function with each resource.
func (l *liveResources[T]) RWalk(ctx context.Context, fn func(name string, t T) error) (err error) {
	l.m.RLock()
	defer l.m.RUnlock()

	for k, v := range l.resources {
		_, _ = v.RAccess(ctx, func(t T) {
			err = fn(k, t)
		})
		if err != nil {
			return
		}
	}
	return nil
}

// Access a resource by name in writeable mode, where mutations within the
// provided closure are safe on the resource, and the set function can be used
// to change or delete (nil argument) the underlying resource. If create is set
// to true the resource is created if it does not yet exist.
func (l *liveResources[T]) Access(ctx context.Context, name string, create bool, fn func(t *T, set func(t *T), err error)) error {
	l.m.RLock()
	rl, exists := l.resources[name]
	l.m.RUnlock()

	if !exists {
		if !create {
			return ErrResourceNotFound(name)
		}
		l.m.Lock()
		rl = &liveResource[T]{}
		l.resources[name] = rl
		l.m.Unlock()
	}

	if rl.Access(ctx, fn) {
		return nil
	}

	// If the underlying resource is deleted we can clean up the resources
	// map and prevent unbounded growth.
	l.m.Lock()
	if !l.resources[name].Access(ctx, func(*T, func(t *T), error) {}) {
		delete(l.resources, name)
	}
	l.m.Unlock()
	return nil
}

// RAccess grants a closure function access to a named resource, but mutations
// must not be performed on the resource itself.
func (l *liveResources[T]) RAccess(ctx context.Context, name string, fn func(t T)) error {
	l.m.RLock()
	rl, exists := l.resources[name]
	l.m.RUnlock()

	if !exists {
		return ErrResourceNotFound(name)
	}

	exists, err := rl.RAccess(ctx, fn)
	if err != nil {
		return err
	}
	if !exists {
		return ErrResourceNotFound(name)
	}
	return nil
}
