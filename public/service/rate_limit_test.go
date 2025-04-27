// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/redpanda-data/benthos/v4/internal/component/metrics"
)

type closableRateLimit struct {
	next   time.Duration
	err    error
	closed bool
}

func (c *closableRateLimit) Access(ctx context.Context) (time.Duration, error) {
	return c.next, c.err
}

func (c *closableRateLimit) Close(ctx context.Context) error {
	c.closed = true
	return nil
}

func TestRateLimitAirGapShutdown(t *testing.T) {
	ctx := t.Context()
	rl := &closableRateLimit{
		next: time.Second,
	}
	agrl := newAirGapRateLimit(rl, metrics.Noop())

	tout, err := agrl.Access(ctx)
	assert.NoError(t, err)
	assert.Equal(t, time.Second, tout)

	rl.next = time.Millisecond
	rl.err = errors.New("test error")

	tout, err = agrl.Access(ctx)
	assert.EqualError(t, err, "test error")
	assert.Equal(t, time.Millisecond, tout)

	err = agrl.Close(ctx)
	assert.NoError(t, err)
	assert.True(t, rl.closed)
}

//------------------------------------------------------------------------------

type closableRateLimitType struct {
	next   time.Duration
	err    error
	closed bool
}

func (c *closableRateLimitType) Access(ctx context.Context) (time.Duration, error) {
	return c.next, c.err
}

func (c *closableRateLimitType) Close(ctx context.Context) error {
	c.closed = true
	return nil
}

func TestRateLimitReverseAirGapShutdown(t *testing.T) {
	rl := &closableRateLimitType{
		next: time.Second,
	}
	agrl := newReverseAirGapRateLimit(rl)

	tout, err := agrl.Access(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, time.Second, tout)

	rl.next = time.Millisecond
	rl.err = errors.New("test error")

	tout, err = agrl.Access(t.Context())
	assert.EqualError(t, err, "test error")
	assert.Equal(t, time.Millisecond, tout)

	assert.NoError(t, agrl.Close(t.Context()))
	assert.True(t, rl.closed)
}
