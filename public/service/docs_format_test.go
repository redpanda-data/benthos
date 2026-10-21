// Copyright 2026 Redpanda Data, Inc.

package service_test

import (
	"encoding/json"
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

// The attributes a distribution declares with SetDocsAttributes decide which
// env-cloud blocks its Markdown output keeps, so a Cloud binary shows the same
// text as the Cloud docs site. No CLI flag or option takes part.
func TestMarshalJSONV0MarkdownResolvesDocsAttributes(t *testing.T) {
	env := service.NewEmptyEnvironment()
	spec := service.NewConfigSpec().Description(`Sends data.

ifndef::env-cloud[]
Self-managed only.
endif::[]

ifdef::env-cloud[]
Cloud only.
endif::[]`)
	require.NoError(t, env.RegisterInput("foo", spec,
		func(*service.ParsedConfig, *service.Resources) (service.Input, error) { return nil, nil }))

	description := func(attrs ...string) string {
		out, err := env.FullConfigSchema("", "").SetDocsAttributes(attrs...).
			MarshalJSONV0Markdown(service.DocsMarkdownOptions{})
		require.NoError(t, err)
		var doc struct {
			Inputs []struct {
				Description string `json:"description"`
			} `json:"inputs"`
		}
		require.NoError(t, json.Unmarshal(out, &doc))
		return doc.Inputs[0].Description
	}

	selfManaged := description()
	assert.Contains(t, selfManaged, "Self-managed only.")
	assert.NotContains(t, selfManaged, "Cloud only.")

	cloud := description(service.DocsAttributeCloud)
	assert.Contains(t, cloud, "Cloud only.")
	assert.NotContains(t, cloud, "Self-managed only.")
}

// A malformed conditional does not fail the render: the schema can hold
// strings registered at run time that no build linted. The other components
// still render, and LintDocsConditionals is how a distribution checks the
// strings it compiles in.
func TestMarshalJSONV0MarkdownRendersPastMalformedConditional(t *testing.T) {
	env := service.NewEmptyEnvironment()
	bad := "Reads data.\n\n ifdef::env-cloud[]\nCloud only.\nendif::[]"
	require.NotEmpty(t, service.LintDocsConditionals(bad))
	for name, desc := range map[string]string{"bad": bad, "good": "Uses *bold*."} {
		require.NoError(t, env.RegisterInput(name, service.NewConfigSpec().Description(desc),
			func(*service.ParsedConfig, *service.Resources) (service.Input, error) { return nil, nil }))
	}

	out, err := env.FullConfigSchema("", "").MarshalJSONV0Markdown(service.DocsMarkdownOptions{})
	require.NoError(t, err)
	var doc struct {
		Inputs []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"inputs"`
	}
	require.NoError(t, json.Unmarshal(out, &doc))
	got := map[string]string{}
	for _, in := range doc.Inputs {
		got[in.Name] = in.Description
	}
	assert.Equal(t, "Uses **bold**.", got["good"])
	assert.Contains(t, got["bad"], "Reads data.")
}
