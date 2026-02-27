// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ibloblang "github.com/redpanda-data/benthos/v4/internal/bloblang"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

func TestMessageMetaImmut(t *testing.T) {
	m := NewMessage(nil)

	// Not found returns nil, false
	v, ok := m.MetaGetImmut("missing")
	assert.False(t, ok)
	assert.Nil(t, v)

	// Scalar string: MetaGetImmut returns the ImmutableAny wrapper itself
	m.MetaSetImmut("str", ImmutableAny{V: "hello"})
	v, ok = m.MetaGetImmut("str")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: "hello"}, v)

	// Integer value
	m.MetaSetImmut("int", ImmutableAny{V: 42})
	v, ok = m.MetaGetImmut("int")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: 42}, v)

	// Slice value: MetaGetImmut returns the wrapper; MetaGetMut returns a copy
	m.MetaSetImmut("slice", ImmutableAny{V: []any{"a", "b", "c"}})
	v, ok = m.MetaGetImmut("slice")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: []any{"a", "b", "c"}}, v)

	// MetaGetMut delivers a distinct copy — mutating it does not affect the stored value
	mutV, ok := m.MetaGetMut("slice")
	assert.True(t, ok)
	assert.Equal(t, []any{"a", "b", "c"}, mutV)
	mutV.([]any)[0] = "x"
	immutV, _ := m.MetaGetImmut("slice")
	assert.Equal(t, []any{"a", "b", "c"}, immutV.(ImmutableAny).V.([]any), "original slice must be unaffected")

	// Map value
	m.MetaSetImmut("map", ImmutableAny{V: map[string]any{"foo": "bar"}})
	v, ok = m.MetaGetImmut("map")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: map[string]any{"foo": "bar"}}, v)

	// MetaSetImmut overwrites a prior value set by MetaSetMut
	m.MetaSetMut("key", "mutable")
	m.MetaSetImmut("key", ImmutableAny{V: "immutable"})
	v, ok = m.MetaGetImmut("key")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: "immutable"}, v)

	// MetaSetMut overwrites a prior value set by MetaSetImmut
	m.MetaSetImmut("key2", ImmutableAny{V: "immutable"})
	m.MetaSetMut("key2", "mutable")
	v, ok = m.MetaGetImmut("key2")
	assert.True(t, ok)
	assert.Equal(t, "mutable", v)

	// Immutable values yielded via MetaWalkMut receive Copy() results
	m2 := NewMessage(nil)
	m2.MetaSetImmut("a", ImmutableAny{V: int64(1)})
	m2.MetaSetImmut("b", ImmutableAny{V: "two"})
	seen := map[string]any{}
	err := m2.MetaWalkMut(func(k string, v any) error {
		seen[k] = v
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"a": int64(1), "b": "two"}, seen)
}

func TestMessageImmutCopyIsolation(t *testing.T) {
	original := NewMessage([]byte(`{"data":"test"}`))
	original.MetaSetImmut("tags", ImmutableAny{V: []any{"alpha", "beta"}})
	original.MetaSetImmut("count", ImmutableAny{V: int64(42)})
	original.MetaSetMut("mut", "original_mut")

	copied := original.Copy()

	// Both see the same immutable values
	v, ok := copied.MetaGetImmut("tags")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: []any{"alpha", "beta"}}, v)

	v, ok = copied.MetaGetImmut("count")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: int64(42)}, v)

	// MetaGetMut on copy triggers Copy() — mutation doesn't affect original
	mutV, ok := copied.MetaGetMut("tags")
	assert.True(t, ok)
	mutV.([]any)[0] = "MUTATED"

	origV, ok := original.MetaGetImmut("tags")
	assert.True(t, ok)
	assert.Equal(t, []any{"alpha", "beta"}, origV.(ImmutableAny).V)

	// Overwriting mutable metadata on copy doesn't affect original
	copied.MetaSetMut("mut", "changed")
	mv, ok := original.MetaGetMut("mut")
	assert.True(t, ok)
	assert.Equal(t, "original_mut", mv)
}

func TestMessageImmutDeepCopySharing(t *testing.T) {
	original := NewMessage([]byte(`{"data":"test"}`))
	immutVal := ImmutableAny{V: map[string]any{"nested": "data"}}
	original.MetaSetImmut("shared", immutVal)
	original.MetaSetMut("cloned", map[string]any{"nested": "mutable"})

	deep := original.DeepCopy()

	// Immutable entry is shared across deep copies
	v, ok := deep.MetaGetImmut("shared")
	assert.True(t, ok)
	assert.Equal(t, immutVal, v)

	// Mutable entry is deep-cloned — mutating original doesn't affect deep copy
	origMut, ok := original.MetaGetMut("cloned")
	assert.True(t, ok)
	origMut.(map[string]any)["nested"] = "MUTATED"

	deepMut, ok := deep.MetaGetMut("cloned")
	assert.True(t, ok)
	assert.Equal(t, "mutable", deepMut.(map[string]any)["nested"])
}

func TestMessageImmutBloblangRoundTrip(t *testing.T) {
	msg := NewMessage(nil)
	msg.SetStructured(map[string]any{"content": "hello"})
	msg.MetaSetImmut("trace_id", ImmutableAny{V: "abc-123"})
	msg.MetaSetImmut("count", ImmutableAny{V: int64(42)})
	msg.MetaSetMut("region", "us-east-1")

	// Bloblang's meta() reads metadata — exercises the MetaGetStr → MetaGetMut → Copy() path
	blobl, err := bloblang.Parse(`
root.trace = meta("trace_id")
root.count = meta("count")
root.region = meta("region")
`)
	require.NoError(t, err)

	res, err := msg.BloblangQuery(blobl)
	require.NoError(t, err)

	resI, err := res.AsStructured()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"trace":  "abc-123",
		"count":  "42",
		"region": "us-east-1",
	}, resI)

	// Original immutable values must be unchanged after mapping
	v, ok := msg.MetaGetImmut("trace_id")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: "abc-123"}, v)

	v, ok = msg.MetaGetImmut("count")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: int64(42)}, v)
}

func TestMessageImmutPluginProcessorFlow(t *testing.T) {
	// Simulates a plugin input that sets immutable metadata, followed
	// by processing through branch-like copy/modify patterns.
	msg := NewMessage([]byte(`{"content":"hello world"}`))
	msg.MetaSetImmut("trace_id", ImmutableAny{V: "trace-001"})
	msg.MetaSetImmut("headers", ImmutableAny{V: map[string]any{
		"content-type": "application/json",
		"x-request-id": "req-456",
	}})
	msg.MetaSetMut("region", "us-east-1")

	// Simulate branch processor: shallow copy for each branch
	branch1 := msg.Copy()
	branch2 := msg.Copy()

	// Branch 1: reads metadata via bloblang
	blobl1, err := bloblang.Parse(`
root = this
root.trace = meta("trace_id")
root.region = meta("region")
`)
	require.NoError(t, err)

	res1, err := branch1.BloblangQuery(blobl1)
	require.NoError(t, err)

	// Branch 2: modifies mutable metadata, then reads
	branch2.MetaSetMut("region", "eu-west-1")
	blobl2, err := bloblang.Parse(`
root = this
root.trace = meta("trace_id")
root.region = meta("region")
`)
	require.NoError(t, err)

	res2, err := branch2.BloblangQuery(blobl2)
	require.NoError(t, err)

	// Verify branch results
	s1, err := res1.AsStructured()
	require.NoError(t, err)
	assert.Equal(t, "trace-001", s1.(map[string]any)["trace"])
	assert.Equal(t, "us-east-1", s1.(map[string]any)["region"])

	s2, err := res2.AsStructured()
	require.NoError(t, err)
	assert.Equal(t, "trace-001", s2.(map[string]any)["trace"])
	assert.Equal(t, "eu-west-1", s2.(map[string]any)["region"])

	// Original immutable metadata unchanged
	v, ok := msg.MetaGetImmut("trace_id")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: "trace-001"}, v)

	// Original mutable metadata unchanged (COW)
	mv, ok := msg.MetaGetMut("region")
	assert.True(t, ok)
	assert.Equal(t, "us-east-1", mv)

	// Immutable reference-type metadata also unchanged
	hv, ok := msg.MetaGetImmut("headers")
	assert.True(t, ok)
	assert.Equal(t, map[string]any{
		"content-type": "application/json",
		"x-request-id": "req-456",
	}, hv.(ImmutableAny).V)
}

func TestMessageImmutConcurrentCopyAccess(t *testing.T) {
	original := NewMessage([]byte(`{}`))
	original.MetaSetImmut("shared", ImmutableAny{V: map[string]any{"key": "value"}})
	original.MetaSetMut("counter", "0")

	// Create copies sequentially (ShallowCopy is not safe for concurrent calls)
	copies := make([]*Message, 100)
	for i := range copies {
		copies[i] = original.Copy()
	}

	var wg sync.WaitGroup
	for _, cp := range copies {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Read immutable (no copy triggered)
			v, ok := cp.MetaGetImmut("shared")
			assert.True(t, ok)
			assert.Equal(t, ImmutableAny{V: map[string]any{"key": "value"}}, v)

			// Read mutable (triggers Copy on immutable entry)
			mv, ok := cp.MetaGetMut("shared")
			assert.True(t, ok)
			assert.Equal(t, map[string]any{"key": "value"}, mv)

			// Mutate the returned copy — must not affect shared state
			mv.(map[string]any)["key"] = "mutated"

			// Write new metadata (triggers COW on shared map)
			cp.MetaSetMut("counter", "updated")
		}()
	}
	wg.Wait()

	// Original must be unchanged
	v, ok := original.MetaGetImmut("shared")
	assert.True(t, ok)
	assert.Equal(t, map[string]any{"key": "value"}, v.(ImmutableAny).V)

	mv, ok := original.MetaGetMut("counter")
	assert.True(t, ok)
	assert.Equal(t, "0", mv)
}

func TestMessageImmutStringCompat(t *testing.T) {
	msg := NewMessage(nil)
	msg.MetaSetImmut("str", ImmutableAny{V: "hello"})
	msg.MetaSetImmut("int", ImmutableAny{V: int64(42)})
	msg.MetaSetImmut("float", ImmutableAny{V: float64(3.14)})
	msg.MetaSetImmut("bool", ImmutableAny{V: true})
	msg.MetaSetMut("plain", "world")

	// MetaGet (string API) returns string representations
	v, ok := msg.MetaGet("str")
	assert.True(t, ok)
	assert.Equal(t, "hello", v)

	v, ok = msg.MetaGet("int")
	assert.True(t, ok)
	assert.Equal(t, "42", v)

	v, ok = msg.MetaGet("float")
	assert.True(t, ok)
	assert.Equal(t, "3.14", v)

	v, ok = msg.MetaGet("bool")
	assert.True(t, ok)
	assert.Equal(t, "true", v)

	v, ok = msg.MetaGet("plain")
	assert.True(t, ok)
	assert.Equal(t, "world", v)

	// MetaWalk (string API) iterates all entries as strings
	seen := map[string]string{}
	err := msg.MetaWalk(func(k, v string) error {
		seen[k] = v
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"str":   "hello",
		"int":   "42",
		"float": "3.14",
		"bool":  "true",
		"plain": "world",
	}, seen)
}

// customImmutValue is a test implementation of ImmutableValue with a
// type-specific Copy() that correctly clones []string.
type customImmutValue struct{ items []string }

func (c customImmutValue) Copy() any {
	cp := make([]string, len(c.items))
	copy(cp, c.items)
	return cp
}

func TestMessageImmutCustomValue(t *testing.T) {
	msg := NewMessage(nil)
	custom := customImmutValue{items: []string{"a", "b", "c"}}
	msg.MetaSetImmut("custom", custom)

	// MetaGetImmut returns the original
	v, ok := msg.MetaGetImmut("custom")
	assert.True(t, ok)
	assert.Equal(t, custom, v)

	// MetaGetMut returns a copy via Copy()
	mutV, ok := msg.MetaGetMut("custom")
	assert.True(t, ok)
	cp := mutV.([]string)
	assert.Equal(t, []string{"a", "b", "c"}, cp)

	// Mutate the copy — original unaffected
	cp[0] = "X"
	v2, ok := msg.MetaGetImmut("custom")
	assert.True(t, ok)
	assert.Equal(t, []string{"a", "b", "c"}, v2.(customImmutValue).items)

	// Copy the message and verify custom value works
	copied := msg.Copy()
	cMutV, ok := copied.MetaGetMut("custom")
	assert.True(t, ok)
	assert.Equal(t, []string{"a", "b", "c"}, cMutV.([]string))

	// DeepCopy shares the immutable custom value
	deep := msg.DeepCopy()
	dv, ok := deep.MetaGetImmut("custom")
	assert.True(t, ok)
	assert.Equal(t, custom, dv)
}

func TestMessageImmutDeleteAfterCopy(t *testing.T) {
	original := NewMessage(nil)
	original.MetaSetImmut("keep", ImmutableAny{V: "keep_val"})
	original.MetaSetImmut("remove", ImmutableAny{V: "remove_val"})
	original.MetaSetMut("mut_keep", "mut_keep_val")
	original.MetaSetMut("mut_remove", "mut_remove_val")

	copied := original.Copy()

	// Delete immutable and mutable entries on the copy
	copied.MetaDelete("remove")
	copied.MetaDelete("mut_remove")

	// Copy should no longer have deleted entries
	_, ok := copied.MetaGetImmut("remove")
	assert.False(t, ok)
	_, ok = copied.MetaGetMut("mut_remove")
	assert.False(t, ok)

	// Copy should still have kept entries
	v, ok := copied.MetaGetImmut("keep")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: "keep_val"}, v)
	v2, ok := copied.MetaGetMut("mut_keep")
	assert.True(t, ok)
	assert.Equal(t, "mut_keep_val", v2)

	// Original should still have all entries (COW)
	v, ok = original.MetaGetImmut("remove")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: "remove_val"}, v)
	v, ok = original.MetaGetImmut("keep")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: "keep_val"}, v)
	v2, ok = original.MetaGetMut("mut_remove")
	assert.True(t, ok)
	assert.Equal(t, "mut_remove_val", v2)
	v2, ok = original.MetaGetMut("mut_keep")
	assert.True(t, ok)
	assert.Equal(t, "mut_keep_val", v2)
}

func TestMessageImmutWalkAfterCopy(t *testing.T) {
	original := NewMessage(nil)
	original.MetaSetImmut("immut", ImmutableAny{V: []any{1, 2, 3}})
	original.MetaSetMut("mut", "plain")

	copied := original.Copy()

	// Walk on the copy — immutable entries should yield Copy() results
	seen := map[string]any{}
	err := copied.MetaWalkMut(func(k string, v any) error {
		seen[k] = v
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, []any{1, 2, 3}, seen["immut"])
	assert.Equal(t, "plain", seen["mut"])

	// Mutate the walked immutable value — original must be unaffected
	seen["immut"].([]any)[0] = 999

	origV, ok := original.MetaGetImmut("immut")
	assert.True(t, ok)
	assert.Equal(t, []any{1, 2, 3}, origV.(ImmutableAny).V)
}

func TestMessageImmutBloblangMetaOverwrite(t *testing.T) {
	msg := NewMessage([]byte(`{"content":"hello"}`))
	msg.MetaSetImmut("immut1", ImmutableAny{V: "preserved"})
	msg.MetaSetImmut("immut2", ImmutableAny{V: int64(99)})
	msg.MetaSetMut("mut1", "mutable_val")

	// Bloblang meta = {...} replaces all metadata (clears existing, sets new)
	blobl, err := bloblang.Parse(`meta = {"new_key": "new_value", "another": "thing"}`)
	require.NoError(t, err)

	res, err := msg.BloblangMutate(blobl)
	require.NoError(t, err)

	// Old metadata (both immutable and mutable) should be gone
	_, ok := res.MetaGetImmut("immut1")
	assert.False(t, ok)
	_, ok = res.MetaGetImmut("immut2")
	assert.False(t, ok)
	_, ok = res.MetaGetMut("mut1")
	assert.False(t, ok)

	// New metadata should be present (as mutable, since meta = {...} uses MetaSetMut)
	v, ok := res.MetaGetMut("new_key")
	assert.True(t, ok)
	assert.Equal(t, "new_value", v)

	v, ok = res.MetaGetMut("another")
	assert.True(t, ok)
	assert.Equal(t, "thing", v)

	// Only the two new keys should exist
	seen := map[string]any{}
	err = res.MetaWalkMut(func(k string, v any) error {
		seen[k] = v
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{
		"new_key": "new_value",
		"another": "thing",
	}, seen)
}

func TestMessageImmutOverwriteImmutAfterCopy(t *testing.T) {
	msg := NewMessage(nil)
	msg.MetaSetImmut("key", ImmutableAny{V: "first"})

	copied := msg.Copy()

	// Overwrite with a different ImmutableValue on the copy
	copied.MetaSetImmut("key", ImmutableAny{V: "second"})

	// Copy has the new value
	v, ok := copied.MetaGetImmut("key")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: "second"}, v)

	// Original has the old value (COW)
	v, ok = msg.MetaGetImmut("key")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: "first"}, v)

	// Also test overwriting with a different ImmutableValue type
	msg2 := NewMessage(nil)
	msg2.MetaSetImmut("key", ImmutableAny{V: "plain"})

	copied2 := msg2.Copy()
	copied2.MetaSetImmut("key", customImmutValue{items: []string{"x", "y"}})

	v2, ok := copied2.MetaGetImmut("key")
	assert.True(t, ok)
	assert.Equal(t, customImmutValue{items: []string{"x", "y"}}, v2)

	// MetaGetMut on the new value should call the new impl's Copy()
	mutV, ok := copied2.MetaGetMut("key")
	assert.True(t, ok)
	assert.Equal(t, []string{"x", "y"}, mutV)

	// Original unchanged
	v3, ok := msg2.MetaGetImmut("key")
	assert.True(t, ok)
	assert.Equal(t, ImmutableAny{V: "plain"}, v3)
}

func TestMessageCopyAirGap(t *testing.T) {
	p := message.NewPart([]byte("hello world"))
	p.MetaSetMut("foo", "bar")
	g1 := NewInternalMessage(p.ShallowCopy())
	g2 := g1.Copy()

	b := p.AsBytes()
	v, _ := p.MetaGetMut("foo")
	assert.Equal(t, "hello world", string(b))
	assert.Equal(t, "bar", v)

	b, err := g1.AsBytes()
	v, _ = g1.MetaGet("foo")
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(b))
	assert.Equal(t, "bar", v)

	b, err = g2.AsBytes()
	v, _ = g2.MetaGetMut("foo")
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(b))
	assert.Equal(t, "bar", v)

	g2.SetBytes([]byte("and now this"))
	g2.MetaSetMut("foo", "baz")

	b = p.AsBytes()
	v, _ = p.MetaGetMut("foo")
	assert.Equal(t, "hello world", string(b))
	assert.Equal(t, "bar", v)

	b, err = g1.AsBytes()
	v, _ = g1.MetaGetMut("foo")
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(b))
	assert.Equal(t, "bar", v)

	b, err = g2.AsBytes()
	v, _ = g2.MetaGetMut("foo")
	require.NoError(t, err)
	assert.Equal(t, "and now this", string(b))
	assert.Equal(t, "baz", v)

	g1.SetBytes([]byte("but not this"))
	g1.MetaSetMut("foo", "buz")

	b = p.AsBytes()
	v, _ = p.MetaGetMut("foo")
	assert.Equal(t, "hello world", string(b))
	assert.Equal(t, "bar", v)

	b, err = g1.AsBytes()
	v, _ = g1.MetaGetMut("foo")
	require.NoError(t, err)
	assert.Equal(t, "but not this", string(b))
	assert.Equal(t, "buz", v)

	b, err = g2.AsBytes()
	v, _ = g2.MetaGetMut("foo")
	require.NoError(t, err)
	assert.Equal(t, "and now this", string(b))
	assert.Equal(t, "baz", v)
}

func TestMessageQuery(t *testing.T) {
	p := message.NewPart([]byte(`{"foo":"bar"}`))
	p.MetaSetMut("foo", "bar")
	p.MetaSetMut("bar", "baz")
	g1 := NewInternalMessage(p)

	b, err := g1.AsBytes()
	assert.NoError(t, err)
	assert.Equal(t, `{"foo":"bar"}`, string(b))

	s, err := g1.AsStructured()
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"foo": "bar"}, s)

	m, ok := g1.MetaGetMut("foo")
	assert.True(t, ok)
	assert.Equal(t, "bar", m)

	seen := map[string]any{}
	err = g1.MetaWalkMut(func(k string, v any) error {
		seen[k] = v
		return errors.New("stop")
	})
	assert.EqualError(t, err, "stop")
	assert.Len(t, seen, 1)

	seen = map[string]any{}
	err = g1.MetaWalkMut(func(k string, v any) error {
		seen[k] = v
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{
		"foo": "bar",
		"bar": "baz",
	}, seen)
}

func TestMessageQueryValue(t *testing.T) {
	msg := NewMessage(nil)
	msg.SetStructured(map[string]any{
		"content": "hello world",
	})

	tests := map[string]struct {
		mapping string
		exp     any
		err     string
	}{
		"returns string": {
			mapping: `root = json("content")`,
			exp:     "hello world",
		},
		"returns integer": {
			mapping: `root = json("content").length()`,
			exp:     int64(11),
		},
		"returns float": {
			mapping: `root = json("content").length() / 2`,
			exp:     float64(5.5),
		},
		"returns bool": {
			mapping: `root = json("content").length() > 0`,
			exp:     true,
		},
		"returns bytes": {
			mapping: `root = content()`,
			exp:     []byte(`{"content":"hello world"}`),
		},
		"returns nil": {
			mapping: `root = null`,
			exp:     nil,
		},
		"returns null string": {
			mapping: `root = "null"`,
			exp:     "null",
		},
		"returns an array": {
			mapping: `root = [ json("content") ]`,
			exp:     []any{"hello world"},
		},
		"returns an object": {
			mapping: `root.new_content = json("content")`,
			exp:     map[string]any{"new_content": "hello world"},
		},
		"returns an error if the mapping throws": {
			mapping: `root = throw("kaboom")`,
			exp:     nil,
			err:     "failed assignment (line 1): kaboom",
		},
		"returns an error if the root is deleted": {
			mapping: `root = deleted()`,
			exp:     nil,
			err:     "root was deleted",
		},
		"doesn't error out if a field is deleted": {
			mapping: `root.foo = deleted()`,
			exp:     map[string]any{},
			err:     "",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			blobl, err := bloblang.Parse(test.mapping)
			require.NoError(t, err)

			res, err := msg.BloblangQueryValue(blobl)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, test.exp, res)
		})
	}
}

func TestMessageMutate(t *testing.T) {
	p := message.NewPart([]byte(`not a json doc`))
	p.MetaSetMut("foo", "bar")
	p.MetaSetMut("bar", "baz")
	g1 := NewInternalMessage(p.ShallowCopy())

	_, err := g1.AsStructured()
	assert.Error(t, err)

	g1.SetStructured(map[string]any{
		"foo": "bar",
	})
	assert.Equal(t, "not a json doc", string(p.AsBytes()))

	s, err := g1.AsStructured()
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{
		"foo": "bar",
	}, s)

	g1.SetBytes([]byte("foo bar baz"))
	assert.Equal(t, "not a json doc", string(p.AsBytes()))

	_, err = g1.AsStructured()
	assert.Error(t, err)

	b, err := g1.AsBytes()
	assert.NoError(t, err)
	assert.Equal(t, "foo bar baz", string(b))

	g1.MetaDelete("foo")

	seen := map[string]any{}
	err = g1.MetaWalkMut(func(k string, v any) error {
		seen[k] = v
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"bar": "baz"}, seen)

	g1.MetaSetMut("foo", "new bar")

	seen = map[string]any{}
	err = g1.MetaWalkMut(func(k string, v any) error {
		seen[k] = v
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"foo": "new bar", "bar": "baz"}, seen)
}

func TestNewMessageMutate(t *testing.T) {
	g0 := NewMessage([]byte(`not a json doc`))
	g0.MetaSetMut("foo", "bar")
	g0.MetaSetMut("bar", "baz")

	g1 := g0.Copy()

	_, err := g1.AsStructured()
	assert.Error(t, err)

	g1.SetStructured(map[string]any{
		"foo": "bar",
	})
	g0Bytes, err := g0.AsBytes()
	require.NoError(t, err)
	assert.Equal(t, "not a json doc", string(g0Bytes))

	s, err := g1.AsStructuredMut()
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{
		"foo": "bar",
	}, s)

	g1.SetBytes([]byte("foo bar baz"))
	g0Bytes, err = g0.AsBytes()
	require.NoError(t, err)
	assert.Equal(t, "not a json doc", string(g0Bytes))

	_, err = g1.AsStructured()
	assert.Error(t, err)

	b, err := g1.AsBytes()
	assert.NoError(t, err)
	assert.Equal(t, "foo bar baz", string(b))

	g1.MetaDelete("foo")

	seen := map[string]any{}
	err = g1.MetaWalkMut(func(k string, v any) error {
		seen[k] = v
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"bar": "baz"}, seen)

	g1.MetaSetMut("foo", "new bar")

	seen = map[string]any{}
	err = g1.MetaWalkMut(func(k string, v any) error {
		seen[k] = v
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"foo": "new bar", "bar": "baz"}, seen)
}

func TestMessageMapping(t *testing.T) {
	part := NewMessage(nil)
	part.SetStructured(map[string]any{
		"content": "hello world",
	})

	blobl, err := bloblang.Parse("root.new_content = this.content.uppercase()")
	require.NoError(t, err)

	res, err := part.BloblangQuery(blobl)
	require.NoError(t, err)

	resI, err := res.AsStructured()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"new_content": "HELLO WORLD",
	}, resI)
}

func TestMessageBatchMapping(t *testing.T) {
	partOne := NewMessage(nil)
	partOne.SetStructured(map[string]any{
		"content": "hello world 1",
	})

	partTwo := NewMessage(nil)
	partTwo.SetStructured(map[string]any{
		"content": "hello world 2",
	})

	blobl, err := bloblang.Parse(`root.new_content = json("content").from_all().join(" - ")`)
	require.NoError(t, err)

	res, err := MessageBatch{partOne, partTwo}.BloblangQuery(0, blobl)
	require.NoError(t, err)

	resI, err := res.AsStructured()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"new_content": "hello world 1 - hello world 2",
	}, resI)
}

func TestMessageBatchQueryValue(t *testing.T) {
	partOne := NewMessage(nil)
	partOne.SetStructured(map[string]any{
		"content": "hello world 1",
	})

	partTwo := NewMessage(nil)
	partTwo.SetStructured(map[string]any{
		"content": "hello world 2",
	})

	tests := map[string]struct {
		mapping    string
		batchIndex int
		exp        any
		err        string
	}{
		"returns string": {
			mapping: `root = json("content")`,
			exp:     "hello world 1",
		},
		"returns integer": {
			mapping: `root = json("content").length()`,
			exp:     int64(13),
		},
		"returns float": {
			mapping: `root = json("content").length() / 2`,
			exp:     float64(6.5),
		},
		"returns bool": {
			mapping: `root = json("content").length() > 0`,
			exp:     true,
		},
		"returns bytes": {
			mapping: `root = content()`,
			exp:     []byte(`{"content":"hello world 1"}`),
		},
		"returns nil": {
			mapping: `root = null`,
			exp:     nil,
		},
		"returns null string": {
			mapping: `root = "null"`,
			exp:     "null",
		},
		"returns an array": {
			mapping: `root = [ json("content") ]`,
			exp:     []any{"hello world 1"},
		},
		"returns an object": {
			mapping: `root.new_content = json("content")`,
			exp:     map[string]any{"new_content": "hello world 1"},
		},
		"supports batch-wide queries": {
			mapping: `root.new_content = json("content").from_all().join(" - ")`,
			exp:     map[string]any{"new_content": "hello world 1 - hello world 2"},
		},
		"handles the specified message index correctly": {
			mapping:    `root = json("content")`,
			batchIndex: 1,
			exp:        "hello world 2",
		},
		"returns an error if the mapping throws": {
			mapping: `root = throw("kaboom")`,
			exp:     nil,
			err:     "failed assignment (line 1): kaboom",
		},
		"returns an error if the root is deleted": {
			mapping: `root = deleted()`,
			exp:     nil,
			err:     "root was deleted",
		},
		"doesn't error out if a field is deleted": {
			mapping: `root.foo = deleted()`,
			exp:     map[string]any{},
			err:     "",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			blobl, err := bloblang.Parse(test.mapping)
			require.NoError(t, err)

			res, err := MessageBatch{partOne, partTwo}.BloblangQueryValue(test.batchIndex, blobl)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, test.exp, res)
		})
	}
}

func BenchmarkMessageMappingNew(b *testing.B) {
	part := NewMessage(nil)
	part.SetStructured(map[string]any{
		"content": "hello world",
	})

	blobl, err := bloblang.Parse("root.new_content = this.content.uppercase()")
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		res, err := part.BloblangQuery(blobl)
		require.NoError(b, err)

		resI, err := res.AsStructured()
		require.NoError(b, err)
		assert.Equal(b, map[string]any{
			"new_content": "HELLO WORLD",
		}, resI)
	}
}

func BenchmarkMessageMappingOld(b *testing.B) {
	part := message.NewPart(nil)
	part.SetStructured(map[string]any{
		"content": "hello world",
	})

	msg := message.Batch{part}

	blobl, err := ibloblang.GlobalEnvironment().NewMapping("root.new_content = this.content.uppercase()")
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		res, err := blobl.MapPart(0, msg)
		require.NoError(b, err)

		resI, err := res.AsStructuredMut()
		require.NoError(b, err)
		assert.Equal(b, map[string]any{
			"new_content": "HELLO WORLD",
		}, resI)
	}
}

func TestSyncResponse(t *testing.T) {
	msgA := NewMessage([]byte("hello world a"))

	msgB, storeB := msgA.WithSyncResponseStore()
	msgB.SetBytes([]byte("hello world b"))

	require.Error(t, msgA.AddSyncResponse())
	require.NoError(t, msgB.AddSyncResponse())

	msgC := msgB.Copy()
	msgC.SetBytes([]byte("hello world c"))
	require.NoError(t, msgC.AddSyncResponse())

	resBatches := storeB.Read()
	require.Len(t, resBatches, 2)
	require.Len(t, resBatches[0], 1)
	require.Len(t, resBatches[1], 1)

	data, err := resBatches[0][0].AsBytes()
	require.NoError(t, err)
	assert.Equal(t, "hello world b", string(data))

	data, err = resBatches[1][0].AsBytes()
	require.NoError(t, err)
	assert.Equal(t, "hello world c", string(data))
}

func TestSyncResponseBatched(t *testing.T) {
	batchA := MessageBatch{
		NewMessage([]byte("hello world a 1")),
		NewMessage([]byte("hello world a 2")),
		NewMessage([]byte("hello world a 3")),
	}

	batchB, storeB := batchA.WithSyncResponseStore()
	batchB[0].SetBytes([]byte("hello world b 1"))
	batchB[1].SetBytes([]byte("hello world b 2"))
	batchB[2].SetBytes([]byte("hello world b 3"))

	require.Error(t, batchA.AddSyncResponse())
	require.NoError(t, batchB.AddSyncResponse())

	batchC := batchB.Copy()
	batchC[1].SetBytes([]byte("hello world c 2"))
	require.NoError(t, batchC.AddSyncResponse())

	batchD := batchA.Copy()
	batchD[1].SetBytes([]byte("hello world d 2"))
	require.Error(t, batchD.AddSyncResponse())

	resBatches := storeB.Read()
	require.Len(t, resBatches, 2)
	require.Len(t, resBatches[0], 3)
	require.Len(t, resBatches[1], 3)

	for i, c := range []string{
		"hello world b 1",
		"hello world b 2",
		"hello world b 3",
	} {
		data, err := resBatches[0][i].AsBytes()
		require.NoError(t, err)
		assert.Equal(t, c, string(data))
	}

	for i, c := range []string{
		"hello world b 1",
		"hello world c 2",
		"hello world b 3",
	} {
		data, err := resBatches[1][i].AsBytes()
		require.NoError(t, err)
		assert.Equal(t, c, string(data))
	}
}
