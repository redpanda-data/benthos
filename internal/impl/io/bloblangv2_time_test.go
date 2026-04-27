// Copyright 2026 Redpanda Data, Inc.

package io_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"

	_ "github.com/redpanda-data/benthos/v4/public/components/io"
)

func runBloblangV2(t *testing.T, mapping string, input any) any {
	t.Helper()
	exec, err := bloblangv2.GlobalEnvironment().Parse(mapping)
	require.NoError(t, err)
	out, err := exec.Query(input)
	require.NoError(t, err)
	return out
}

func TestBloblangV2TimestampUnix(t *testing.T) {
	now := time.Now().Unix()
	got := runBloblangV2(t, `output = timestamp_unix()`, nil).(int64)
	// Allow 5s tolerance for test flakiness.
	assert.InDelta(t, now, got, 5)
}

func TestBloblangV2TimestampUnixMilli(t *testing.T) {
	now := time.Now().UnixMilli()
	got := runBloblangV2(t, `output = timestamp_unix_milli()`, nil).(int64)
	assert.InDelta(t, now, got, 5000)
}

func TestBloblangV2TimestampUnixMicro(t *testing.T) {
	now := time.Now().UnixMicro()
	got := runBloblangV2(t, `output = timestamp_unix_micro()`, nil).(int64)
	assert.InDelta(t, now, got, 5_000_000)
}

func TestBloblangV2TimestampUnixNano(t *testing.T) {
	now := time.Now().UnixNano()
	got := runBloblangV2(t, `output = timestamp_unix_nano()`, nil).(int64)
	assert.InDelta(t, now, got, 5_000_000_000)
}
