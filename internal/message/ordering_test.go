// Copyright 2026 Redpanda Data, Inc.

package message

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderingSettledNoCallbacks(t *testing.T) {
	type fooKeyType int
	var fooKey fooKeyType

	for _, ctx := range []context.Context{
		t.Context(),
		context.WithValue(t.Context(), fooKey, "bar"),
	} {
		assert.NotPanics(t, func() {
			CtxOrderingSettled(ctx)
			CtxOrderingSettled(ctx)
		})
	}

	assert.NotPanics(t, func() {
		OrderingSettled(NewPart([]byte("foo")))
	})
}

func TestOrderingSettledCallbackOrder(t *testing.T) {
	var seen []string
	record := func(s string) func() {
		return func() { seen = append(seen, s) }
	}

	root := CtxOnOrderingSettled(t.Context(), record("root"))
	a := CtxOnOrderingSettled(root, record("a"))
	b := CtxOnOrderingSettled(root, record("b"))
	c := CtxOnOrderingSettled(a, record("c"))

	CtxOrderingSettled(a)
	CtxOrderingSettled(a)
	CtxOrderingSettled(b)
	CtxOrderingSettled(c)

	assert.Equal(t, []string{
		"root", "a",
		"root", "a",
		"root", "b",
		"root", "a", "c",
	}, seen)
}

func TestOrderingSettledSurvivesPartCopies(t *testing.T) {
	var fired int
	part := NewPart([]byte("foo"))
	part = part.WithContext(CtxOnOrderingSettled(part.GetContext(), func() { fired++ }))

	OrderingSettled(part.ShallowCopy())
	OrderingSettled(part.DeepCopy())

	type fooKeyType int
	var fooKey fooKeyType
	OrderingSettled(part.WithContext(context.WithValue(part.GetContext(), fooKey, "bar")))

	assert.Equal(t, 3, fired)
}

func BenchmarkOrderingSettled(b *testing.B) {
	type otherKeyType int

	nested := func(ctx context.Context, depth int) context.Context {
		for i := range depth {
			ctx = context.WithValue(ctx, otherKeyType(i), i)
		}
		return ctx
	}

	b.Run("no callback", func(b *testing.B) {
		part := NewPart([]byte("foo"))
		for b.Loop() {
			OrderingSettled(part)
		}
	})

	b.Run("no callback behind four values", func(b *testing.B) {
		part := NewPart([]byte("foo")).WithContext(nested(context.Background(), 4))
		for b.Loop() {
			OrderingSettled(part)
		}
	})

	b.Run("callback behind four values", func(b *testing.B) {
		var fired int
		ctx := CtxOnOrderingSettled(context.Background(), func() { fired++ })
		part := NewPart([]byte("foo")).WithContext(nested(ctx, 4))
		for b.Loop() {
			OrderingSettled(part)
		}
	})
}
