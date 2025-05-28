// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"
)

func TestMultilevelCacheErrors(t *testing.T) {
	resBuilder := service.NewResourceBuilder()
	require.NoError(t, resBuilder.AddCacheYAML(`
label: testing
multilevel: []
`))

	res, closeFn, err := resBuilder.Build()
	require.NoError(t, err)

	err = res.AccessCache(t.Context(), "testing", func(c service.Cache) {
		t.Error("unexpected access of bad cache")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected at least two cache levels")

	require.NoError(t, closeFn(t.Context()))

	resBuilder = service.NewResourceBuilder()
	require.NoError(t, resBuilder.AddCacheYAML(`
label: foo
memory: {}
`))
	require.NoError(t, resBuilder.AddCacheYAML(`
label: testing
multilevel:
  - foo
`))

	res, closeFn, err = resBuilder.Build()
	require.NoError(t, err)

	err = res.AccessCache(t.Context(), "testing", func(c service.Cache) {
		t.Error("unexpected access of bad cache")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected at least two cache levels")

	require.NoError(t, closeFn(t.Context()))
}

type mockCacheProv struct {
	caches map[string]service.Cache
}

func (m *mockCacheProv) AccessCache(ctx context.Context, name string, fn func(c service.Cache)) error {
	c, ok := m.caches[name]
	if !ok {
		return errors.New("cache not found")
	}
	fn(c)
	return nil
}

func TestMultilevelCacheGetting(t *testing.T) {
	memCache1 := newMemCache(time.Minute, 0, 1, nil)
	memCache2 := newMemCache(time.Minute, 0, 1, nil)
	p := &mockCacheProv{
		caches: map[string]service.Cache{
			"foo": memCache1,
			"bar": memCache2,
		},
	}

	c, err := newMultilevelCache([]string{"foo", "bar"}, p, nil)
	require.NoError(t, err)

	ctx := t.Context()

	_, err = c.Get(ctx, "not_exist")
	assert.Equal(t, err, service.ErrKeyNotFound)

	require.NoError(t, memCache2.Set(ctx, "foo", []byte("test value 1"), nil))

	val, err := c.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	val, err = memCache1.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	require.NoError(t, memCache2.Delete(ctx, "foo"))

	val, err = memCache1.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	_, err = memCache2.Get(ctx, "foo")
	assert.Equal(t, err, service.ErrKeyNotFound)
}

func TestMultilevelCacheSet(t *testing.T) {
	memCache1 := newMemCache(time.Minute, 0, 1, nil)
	memCache2 := newMemCache(time.Minute, 0, 1, nil)
	p := &mockCacheProv{
		caches: map[string]service.Cache{
			"foo": memCache1,
			"bar": memCache2,
		},
	}

	c, err := newMultilevelCache([]string{"foo", "bar"}, p, nil)
	require.NoError(t, err)

	ctx := t.Context()

	require.NoError(t, c.Set(ctx, "foo", []byte("test value 1"), nil))

	val, err := memCache1.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	val, err = memCache2.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	err = c.Set(ctx, "foo", []byte("test value 2"), nil)
	require.NoError(t, err)
	require.NoError(t, err)

	val, err = memCache1.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 2"))

	val, err = memCache2.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 2"))
}

func TestMultilevelCacheDelete(t *testing.T) {
	memCache1 := newMemCache(time.Minute, 0, 1, nil)
	memCache2 := newMemCache(time.Minute, 0, 1, nil)
	p := &mockCacheProv{
		caches: map[string]service.Cache{
			"foo": memCache1,
			"bar": memCache2,
		},
	}

	c, err := newMultilevelCache([]string{"foo", "bar"}, p, nil)
	require.NoError(t, err)

	ctx := t.Context()

	require.NoError(t, memCache2.Set(ctx, "foo", []byte("test value 1"), nil))

	require.NoError(t, c.Delete(ctx, "foo"))

	_, err = memCache1.Get(ctx, "foo")
	assert.Equal(t, err, service.ErrKeyNotFound)

	_, err = memCache2.Get(ctx, "foo")
	assert.Equal(t, err, service.ErrKeyNotFound)

	require.NoError(t, memCache1.Set(ctx, "foo", []byte("test value 1"), nil))
	require.NoError(t, memCache2.Set(ctx, "foo", []byte("test value 2"), nil))

	err = c.Delete(ctx, "foo")
	require.NoError(t, err)

	_, err = memCache1.Get(ctx, "foo")
	assert.Equal(t, err, service.ErrKeyNotFound)

	_, err = memCache2.Get(ctx, "foo")
	assert.Equal(t, err, service.ErrKeyNotFound)
}

func TestMultilevelCacheAdd(t *testing.T) {
	memCache1 := newMemCache(time.Minute, 0, 1, nil)
	memCache2 := newMemCache(time.Minute, 0, 1, nil)
	p := &mockCacheProv{
		caches: map[string]service.Cache{
			"foo": memCache1,
			"bar": memCache2,
		},
	}

	c, err := newMultilevelCache([]string{"foo", "bar"}, p, nil)
	require.NoError(t, err)

	ctx := t.Context()

	err = c.Add(ctx, "foo", []byte("test value 1"), nil)
	require.NoError(t, err)

	val, err := memCache1.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	val, err = memCache2.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	err = c.Add(ctx, "foo", []byte("test value 2"), nil)
	assert.Equal(t, err, service.ErrKeyAlreadyExists)

	val, err = memCache1.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	val, err = memCache2.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	err = memCache2.Delete(ctx, "foo")
	require.NoError(t, err)

	err = c.Add(ctx, "foo", []byte("test value 3"), nil)
	assert.Equal(t, err, service.ErrKeyAlreadyExists)

	err = memCache1.Delete(ctx, "foo")
	require.NoError(t, err)

	err = c.Add(ctx, "foo", []byte("test value 4"), nil)
	require.NoError(t, err)

	val, err = memCache1.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 4"))

	val, err = memCache2.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 4"))

	err = memCache1.Delete(ctx, "foo")
	require.NoError(t, err)

	err = c.Add(ctx, "foo", []byte("test value 5"), nil)
	assert.Equal(t, err, service.ErrKeyAlreadyExists)
}

func TestMultilevelCacheAddMoreCaches(t *testing.T) {
	memCache1 := newMemCache(time.Minute, 0, 1, nil)
	memCache2 := newMemCache(time.Minute, 0, 1, nil)
	memCache3 := newMemCache(time.Minute, 0, 1, nil)
	p := &mockCacheProv{
		caches: map[string]service.Cache{
			"foo": memCache1,
			"bar": memCache2,
			"baz": memCache3,
		},
	}

	c, err := newMultilevelCache([]string{"foo", "bar", "baz"}, p, nil)
	require.NoError(t, err)

	ctx := t.Context()

	err = c.Add(ctx, "foo", []byte("test value 1"), nil)
	require.NoError(t, err)

	val, err := memCache1.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	val, err = memCache2.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	val, err = memCache3.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	err = c.Add(ctx, "foo", []byte("test value 2"), nil)
	assert.Equal(t, err, service.ErrKeyAlreadyExists)

	val, err = memCache1.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	val, err = memCache2.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	val, err = memCache3.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 1"))

	err = memCache1.Delete(ctx, "foo")
	require.NoError(t, err)

	err = memCache2.Delete(ctx, "foo")
	require.NoError(t, err)

	err = c.Add(ctx, "foo", []byte("test value 3"), nil)
	assert.Equal(t, err, service.ErrKeyAlreadyExists)

	err = memCache3.Delete(ctx, "foo")
	require.NoError(t, err)

	err = c.Add(ctx, "foo", []byte("test value 4"), nil)
	require.NoError(t, err)

	val, err = memCache1.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 4"))

	val, err = memCache2.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 4"))

	val, err = memCache3.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, val, []byte("test value 4"))
}
