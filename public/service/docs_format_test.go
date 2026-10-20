// Copyright 2026 Redpanda Data, Inc.

package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"
)

// MarshalJSONV0Markdown must render a copy of the docs. The specs it reads
// are shared with the environment's registry, and with every environment
// cloned from it, such as one schema per distribution in the same process. A
// write into them makes the next render start from text that another
// distribution already rendered, and makes MarshalJSONV0 return Markdown
// labelled asciidoc.
func TestMarshalJSONV0MarkdownDoesNotMutateEnvironment(t *testing.T) {
	env := service.NewEmptyEnvironment()
	spec := service.NewConfigSpec().Example("Basic", "Uses *bold*.", "input: {}")
	require.NoError(t, env.RegisterInput("foo", spec,
		func(*service.ParsedConfig, *service.Resources) (service.Input, error) { return nil, nil }))

	_, err := env.FullConfigSchema("", "").MarshalJSONV0Markdown(service.DocsMarkdownOptions{})
	require.NoError(t, err)

	env.WalkInputs(func(_ string, c *service.ConfigView) {
		data, err := c.TemplateData()
		require.NoError(t, err)
		assert.Equal(t, "Uses *bold*.", data.Examples[0].Summary)
	})
}
