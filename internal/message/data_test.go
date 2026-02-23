// Copyright 2025 Redpanda Data, Inc.

package message

import (
	"sync"
	"testing"

	"github.com/Jeffail/gabs/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrentMutationsFromNil(t *testing.T) {
	source := newMessageBytes(nil)
	kickOffChan := make(chan struct{})

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			<-kickOffChan

			local := source.ShallowCopy()
			local.MetaSetMut("foo", "bar")
			local.MetaSetMut("bar", "baz")
			_ = local.MetaIterMut(func(k string, v any) error {
				return nil
			})
			local.MetaDelete("foo")

			local.SetBytes([]byte(`new thing`))
			local.SetStructuredMut(map[string]any{
				"foo": "bar",
			})

			vThing, err := local.AsStructuredMut()
			require.NoError(t, err)

			_, err = gabs.Wrap(vThing).Set("baz", "foo")
			require.NoError(t, err)

			vBytes := local.AsBytes()
			assert.Equal(t, `{"foo":"baz"}`, string(vBytes))
		})
	}

	close(kickOffChan)
	wg.Wait()
}

func TestConcurrentMutationsFromStructured(t *testing.T) {
	source := newMessageBytes(nil)
	source.MetaSetMut("foo", "foo1")
	source.MetaSetMut("bar", "bar1")
	source.SetStructuredMut(map[string]any{
		"foo": "bar",
	})

	kickOffChan := make(chan struct{})

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			<-kickOffChan

			local := source.ShallowCopy()
			local.MetaSetMut("foo", "foo2")

			v, exists := local.MetaGetMut("foo")
			assert.True(t, exists)
			assert.Equal(t, "foo2", v)

			v, exists = local.MetaGetMut("bar")
			assert.True(t, exists)
			assert.Equal(t, "bar1", v)

			_ = local.MetaIterMut(func(k string, v any) error {
				return nil
			})
			local.MetaDelete("foo")

			_, exists = local.MetaGetMut("foo")
			assert.False(t, exists)

			vThing, err := local.AsStructuredMut()
			require.NoError(t, err)

			_, err = gabs.Wrap(vThing).Set("baz", "foo")
			require.NoError(t, err)

			vBytes := local.AsBytes()
			assert.Equal(t, `{"foo":"baz"}`, string(vBytes))

			vThingMore, err := local.AsStructuredMut()
			require.NoError(t, err)

			_, err = gabs.Wrap(vThingMore).Set("meow", "foo")
			require.NoError(t, err)

			vBytes = local.AsBytes()
			assert.Equal(t, `{"foo":"meow"}`, string(vBytes))
		})
	}

	close(kickOffChan)
	wg.Wait()
}

// mockImmut is a minimal immutableMeta implementation for testing.
type mockImmut struct{ val any }

func (m mockImmut) Copy() any { return m.val }

func TestMetaValueSemantics(t *testing.T) {
	t.Run("mutable get returns value directly", func(t *testing.T) {
		d := newMessageBytes(nil)
		d.MetaSetMut("k", "hello")
		v, ok := d.MetaGetMut("k")
		assert.True(t, ok)
		assert.Equal(t, "hello", v)
	})

	t.Run("immutable get returns ImmutableValue without Copy", func(t *testing.T) {
		d := newMessageBytes(nil)
		im := mockImmut{val: []any{"a", "b"}}
		d.MetaSetImmut("k", im)
		// MetaGetImmut returns the stored ImmutableValue itself
		v, ok := d.MetaGetImmut("k")
		assert.True(t, ok)
		assert.Equal(t, im, v)
	})

	t.Run("MetaGetMut on immutable calls Copy", func(t *testing.T) {
		d := newMessageBytes(nil)
		im := mockImmut{val: "original"}
		d.MetaSetImmut("k", im)
		v, ok := d.MetaGetMut("k")
		assert.True(t, ok)
		assert.Equal(t, "original", v)
	})

	t.Run("MetaGetMut on mutable does not call Copy", func(t *testing.T) {
		d := newMessageBytes(nil)
		obj := map[string]any{"x": 1}
		d.MetaSetMut("k", obj)
		v, ok := d.MetaGetMut("k")
		assert.True(t, ok)
		// Same pointer — no copy performed
		assert.Equal(t, obj, v.(map[string]any))
	})

	t.Run("MetaIterMut calls Copy for immutable entries only", func(t *testing.T) {
		d := newMessageBytes(nil)
		d.MetaSetMut("mut", "plain")
		d.MetaSetImmut("immut", mockImmut{val: int64(42)})

		seen := map[string]any{}
		require.NoError(t, d.MetaIterMut(func(k string, v any) error {
			seen[k] = v
			return nil
		}))
		assert.Equal(t, "plain", seen["mut"])
		assert.Equal(t, int64(42), seen["immut"])
	})

	t.Run("ShallowCopy COW preserves immutable flag", func(t *testing.T) {
		src := newMessageBytes(nil)
		src.MetaSetImmut("k", mockImmut{val: "immut"})
		src.MetaSetMut("m", "mut")

		cp := src.ShallowCopy()
		// Write to cp does not affect src
		cp.MetaSetMut("m", "mutated")
		v, _ := src.MetaGetMut("m")
		assert.Equal(t, "mut", v)

		// Immutable flag preserved in copy
		raw, ok := cp.MetaGetImmut("k")
		assert.True(t, ok)
		assert.Equal(t, mockImmut{val: "immut"}, raw)
	})

	t.Run("DeepCopy shares immutable entries", func(t *testing.T) {
		im := mockImmut{val: []any{"shared"}}
		src := newMessageBytes(nil)
		src.MetaSetImmut("k", im)

		dc := src.DeepCopy()
		// The stored ImmutableValue pointer is shared (safe: Copy is lazy)
		raw, ok := dc.MetaGetImmut("k")
		assert.True(t, ok)
		assert.Equal(t, im, raw)
	})

	t.Run("DeepCopy deep-clones mutable entries", func(t *testing.T) {
		obj := map[string]any{"x": "original"}
		src := newMessageBytes(nil)
		src.MetaSetMut("k", obj)

		dc := src.DeepCopy()
		obj["x"] = "mutated"

		v, ok := dc.MetaGetMut("k")
		assert.True(t, ok)
		assert.Equal(t, "original", v.(map[string]any)["x"])
	})

	t.Run("missing key returns false", func(t *testing.T) {
		d := newMessageBytes(nil)
		_, ok := d.MetaGetMut("missing")
		assert.False(t, ok)
		_, ok = d.MetaGetImmut("missing")
		assert.False(t, ok)
	})
}

func TestSetNil(t *testing.T) {
	source := newMessageBytes(nil)
	source.SetStructured(map[string]any{
		"foo": "bar",
	})

	v, err := source.AsStructured()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"foo": "bar"}, v)

	source.SetStructured(nil)

	v, err = source.AsStructured()
	require.NoError(t, err)
	assert.Nil(t, v)
}
