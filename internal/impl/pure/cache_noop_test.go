// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"
)

func TestNoopCacheStandard(t *testing.T) {
	t.Parallel()

	resources := service.MockResources()

	c := noopMemCache("TestNoopCacheStandard", resources.Logger())

	err := c.Set(t.Context(), "foo", []byte("bar"), nil)
	require.NoError(t, err)

	value, err := c.Get(t.Context(), "foo")
	require.EqualError(t, err, "key does not exist")

	assert.Nil(t, value)
}
