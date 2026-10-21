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

// A schema read back from MarshalJSONV0Markdown output keeps saying it holds
// Markdown, and is not rendered a second time.
func TestConfigSchemaFromJSONV0KeepsDocsFormat(t *testing.T) {
	env := service.NewEmptyEnvironment()
	spec := service.NewConfigSpec().Description("Uses *bold* and xref:components:inputs/foo.adoc[foo].")
	require.NoError(t, env.RegisterInput("foo", spec,
		func(*service.ParsedConfig, *service.Resources) (service.Input, error) { return nil, nil }))
	opts := service.DocsMarkdownOptions{DocumentationURL: "https://docs.example.com/connect"}

	md, err := env.FullConfigSchema("v", "d").MarshalJSONV0Markdown(opts)
	require.NoError(t, err)
	parsed, err := service.ConfigSchemaFromJSONV0(md)
	require.NoError(t, err)

	again, err := parsed.MarshalJSONV0()
	require.NoError(t, err)
	assert.Contains(t, string(again), `"docs_format":"markdown"`)
	assert.Contains(t, string(again), "Uses **bold** and [foo](https://docs.example.com/connect/components/inputs/foo/).")

	rerendered, err := parsed.MarshalJSONV0Markdown(opts)
	require.NoError(t, err)
	assert.JSONEq(t, string(again), string(rerendered))
}

// A distribution decides which components/<type>/<name> targets are
// components it lacks, so a docs page there that is not a component keeps
// its link.
func TestMarshalJSONV0MarkdownHasComponent(t *testing.T) {
	env := service.NewEmptyEnvironment()
	spec := service.NewConfigSpec().Description("See xref:components:outputs/bigquery_cdc_migration.adoc[the migration guide] and xref:components:outputs/gone.adoc[gone].")
	require.NoError(t, env.RegisterInput("foo", spec,
		func(*service.ParsedConfig, *service.Resources) (service.Input, error) { return nil, nil }))

	opts := service.DocsMarkdownOptions{
		DocumentationURL: "https://docs.example.com/connect",
		HasComponent:     func(_, name string) bool { return name != "gone" },
	}
	md, err := env.FullConfigSchema("v", "d").MarshalJSONV0Markdown(opts)
	require.NoError(t, err)
	assert.Contains(t, string(md), "[the migration guide](https://docs.example.com/connect/components/outputs/bigquery_cdc_migration/)")
	assert.Contains(t, string(md), "and gone.")
}
