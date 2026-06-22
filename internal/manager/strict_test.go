// Copyright 2026 Redpanda Data, Inc.

package manager_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/manager"
)

func TestManagerStrictDefaultsOff(t *testing.T) {
	mgr, err := manager.New(manager.NewResourceConfig())
	require.NoError(t, err)
	assert.False(t, mgr.Strict())
}

// The strict flag must survive every manager derivation, since components
// (pipelines, processors, outputs) are constructed via derived managers.
func TestManagerStrictPreservedThroughDerivation(t *testing.T) {
	mgr, err := manager.New(manager.NewResourceConfig(), manager.OptSetStrict(true))
	require.NoError(t, err)
	require.True(t, mgr.Strict())

	assert.True(t, mgr.ForStream("s").Strict(), "ForStream must preserve strict")
	assert.True(t, mgr.IntoPath("a", "b").Strict(), "IntoPath must preserve strict")
	assert.True(t, mgr.ForStream("s").IntoPath("pipeline", "processors", "0").Strict(),
		"nested stream + path derivation must preserve strict")
}
