// Copyright 2026 Redpanda Data, Inc.

package service

import (
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigRetryBackOffFields(t *testing.T) {
	spec := NewConfigSpec().
		Fields(NewRetryBackOffFields(3, &backoff.ExponentialBackOff{
			InitialInterval: time.Second,
			MaxInterval:     time.Second * 5,
		})...)

	parsedConfig, err := spec.ParseYAML(`
backoff:
  max_interval: 300s
`, nil)
	require.NoError(t, err)

	maxRetries, err := parsedConfig.FieldInt("max_retries")
	require.NoError(t, err)
	assert.Equal(t, 3, maxRetries)

	bConf, err := parsedConfig.FieldBackOff("backoff")
	require.NoError(t, err)
	assert.Equal(t, time.Second, bConf.InitialInterval)
	assert.Equal(t, time.Second*300, bConf.MaxInterval)
	assert.Equal(t, time.Duration(0), bConf.MaxElapsedTime)
}
