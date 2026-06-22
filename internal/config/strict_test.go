// Copyright 2026 Redpanda Data, Inc.

package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/config"
)

func TestConfigErrorHandlingStrict(t *testing.T) {
	spec := config.Spec()

	parse := func(t *testing.T, raw map[string]any) config.Type {
		t.Helper()
		pConf, err := spec.ParsedConfigFromAny(raw)
		require.NoError(t, err)
		c, err := config.FromParsed(bundle.GlobalEnvironment, pConf, nil)
		require.NoError(t, err)
		return c
	}

	t.Run("defaults off", func(t *testing.T) {
		c := parse(t, map[string]any{})
		assert.False(t, c.ErrorHandling.Strict)
	})

	t.Run("enabled", func(t *testing.T) {
		c := parse(t, map[string]any{
			"error_handling": map[string]any{"strict": true},
		})
		assert.True(t, c.ErrorHandling.Strict)
	})
}
