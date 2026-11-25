// Copyright 2025 Redpanda Data, Inc.

package io

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"
)

func TestFileCache(t *testing.T) {
	dir := t.TempDir()

	tCtx := t.Context()
	c := newFileCache(dir, service.MockResources())

	_, err := c.Get(tCtx, "foo")
	assert.Equal(t, service.ErrKeyNotFound, err)

	require.NoError(t, c.Set(tCtx, "foo", []byte("1"), nil))

	act, err := c.Get(tCtx, "foo")
	require.NoError(t, err)
	assert.Equal(t, "1", string(act))

	require.NoError(t, c.Add(tCtx, "bar", []byte("2"), nil))

	act, err = c.Get(tCtx, "bar")
	require.NoError(t, err)
	assert.Equal(t, "2", string(act))

	assert.Equal(t, service.ErrKeyAlreadyExists, c.Add(tCtx, "foo", []byte("2"), nil))

	require.NoError(t, c.Set(tCtx, "foo", []byte("3"), nil))

	act, err = c.Get(tCtx, "foo")
	require.NoError(t, err)
	assert.Equal(t, "3", string(act))

	require.NoError(t, c.Delete(tCtx, "foo"))

	_, err = c.Get(tCtx, "foo")
	assert.Equal(t, service.ErrKeyNotFound, err)
}

func TestFileCacheNestedKeys(t *testing.T) {
	for _, operation := range []string{"set", "add"} {
		t.Run(operation, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "cache")
			c := newFileCache(dir, service.MockResources())
			write := c.Set
			if operation == "add" {
				write = c.Add
			}
			ctx := t.Context()
			key := "tenant/reports/result.json"
			require.NoError(t, write(ctx, key, []byte("first"), nil))
			actual, err := c.Get(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, "first", string(actual))
			assert.ErrorIs(t, c.Add(ctx, key, []byte("duplicate"), nil), service.ErrKeyAlreadyExists)
			require.NoError(t, c.Set(ctx, key, []byte("updated"), nil))
			actual, err = c.Get(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, "updated", string(actual))
			require.NoError(t, c.Delete(ctx, key))
			_, err = c.Get(ctx, key)
			assert.ErrorIs(t, err, service.ErrKeyNotFound)
		})
	}
}

func TestFileCacheParentIsFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "parent"), []byte("keep"), 0o644))
	c := newFileCache(dir, service.MockResources())
	for _, write := range []func(context.Context, string, []byte, *time.Duration) error{c.Set, c.Add} {
		require.Error(t, write(t.Context(), "parent/child", []byte("value"), nil))
	}
	actual, err := os.ReadFile(filepath.Join(dir, "parent"))
	require.NoError(t, err)
	assert.Equal(t, "keep", string(actual))
}
