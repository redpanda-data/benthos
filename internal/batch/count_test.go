// Copyright 2025 Redpanda Data, Inc.

package batch

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/redpanda-data/benthos/v4/internal/message"
)

func TestCount(t *testing.T) {
	p1 := message.GetContext(message.NewPart([]byte("foo bar")))

	p2 := CtxWithCollapsedCount(p1, 2)
	p3 := CtxWithCollapsedCount(p2, 3)
	p4 := CtxWithCollapsedCount(p1, 4)

	assert.Equal(t, 1, CtxCollapsedCount(p1))
	assert.Equal(t, 2, CtxCollapsedCount(p2))
	assert.Equal(t, 4, CtxCollapsedCount(p3))
	assert.Equal(t, 4, CtxCollapsedCount(p4))
}

func TestMessageCount(t *testing.T) {
	m := message.QuickBatch([][]byte{
		[]byte("FOO"),
		[]byte("BAR"),
		[]byte("BAZ"),
	})

	assert.Equal(t, 3, MessageCollapsedCount(m))
}
