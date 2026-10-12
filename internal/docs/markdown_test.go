// Copyright 2026 Redpanda Data, Inc.

package docs_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/redpanda-data/benthos/v4/internal/docs"
)

func TestAsciidocToMarkdown(t *testing.T) {
	opts := docs.MarkdownOptions{
		BaseURL:       "https://docs.redpanda.com/connect",
		ComponentURLs: map[string]string{"streaming": "https://docs.redpanda.com/streaming"},
	}
	page := "components/inputs/foo"

	for _, test := range []struct {
		name, in, out string
		attrs         []string
	}{
		{
			name: "xrefs resolve to absolute URLs",
			in:   "See xref:components:inputs/kafka.adoc#tls[TLS] and xref:configuration:interpolation.adoc[interpolation].",
			out:  "See [TLS](https://docs.redpanda.com/connect/components/inputs/kafka/#tls) and [interpolation](https://docs.redpanda.com/connect/configuration/interpolation/).",
		},
		{
			name: "xref with code in its text",
			in:   "Use the xref:components:outputs/redpanda.adoc[`redpanda` output] instead.",
			out:  "Use the [`redpanda` output](https://docs.redpanda.com/connect/components/outputs/redpanda/) instead.",
		},
		{
			name: "component-qualified xrefs use the component URL, or text when unknown",
			in:   "xref:streaming:manage:a.adoc[A] and xref:other:b:c.adoc[C]",
			out:  "[A](https://docs.redpanda.com/streaming/manage/a/) and C",
		},
		{
			name: "anchors resolve to the page",
			in:   "Set <<checkpoint_limit,`checkpoint_limit`>> or <<tls-enabled>>.",
			out:  "Set [`checkpoint_limit`](https://docs.redpanda.com/connect/components/inputs/foo/#checkpoint_limit) or [`tls-enabled`](https://docs.redpanda.com/connect/components/inputs/foo/#tls-enabled).",
		},
		{
			name: "external links",
			in:   "Uses https://github.com/twmb/franz-go[Franz^] and link:https://example.com[example].",
			out:  "Uses [Franz](https://github.com/twmb/franz-go) and [example](https://example.com).",
		},
		{
			name:  "ifndef env-cloud keeps text outside Cloud",
			in:    "A.\n\nifndef::env-cloud[]\nSelf-managed.\nendif::[]\n\nifdef::env-cloud[]\nCloud.\nendif::[]",
			out:   "A.\n\nSelf-managed.\n",
			attrs: nil,
		},
		{
			name:  "ifndef env-cloud drops text in Cloud",
			in:    "A.\n\nifndef::env-cloud[]\nSelf-managed.\nendif::[]\n\nifdef::env-cloud[]\nCloud.\nendif::[]",
			out:   "A.\n\nCloud.",
			attrs: []string{"env-cloud"},
		},
		{
			name: "headings, admonitions, and bold",
			in:   "== Performance\n\nNOTE: This is *important*.",
			out:  "## Performance\n\n> **Note:** This is **important**.",
		},
		{
			name: "admonition blocks become quotes",
			in:   "[WARNING]\n====\nThis component is deprecated.\n====",
			out:  "> **Warning:**\n> This component is deprecated.",
		},
		{
			name: "source blocks become fences and are left alone",
			in:   "[source,yaml]\n----\ninput:\n  foo: xref:not.adoc[]\n----",
			out:  "```yaml\ninput:\n  foo: xref:not.adoc[]\n```",
		},
		{
			name: "tabs flatten into titled sections",
			in:   "[tabs]\n======\nCommon::\n+\n--\nText.\n--\n======",
			out:  "**Common**\nText.",
		},
		{
			name: "dsv tables become Markdown tables",
			in:   "[%header,format=dsv]\n|===\nType:Format\nCHAR:string\n|===",
			out:  "| Type | Format |\n| --- | --- |\n| CHAR | string |",
		},
		{
			name: "psv tables with one cell per line",
			in:   "|===\n|Name |Value\n\n|a\n|1\n\n|b\n|2\n|===",
			out:  "| Name | Value |\n| --- | --- |\n| a | 1 |\n| b | 2 |",
		},
		{
			name: "escaped interpolations and attribute references",
			in:   "Use `$\\{REDPANDA_BROKERS}`{nbsp}now.",
			out:  "Use `${REDPANDA_BROKERS}` now.",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			o := opts
			o.Attributes = map[string]bool{}
			for _, a := range test.attrs {
				o.Attributes[a] = true
			}
			assert.Equal(t, test.out, docs.AsciidocToMarkdown(test.in, page, o))
		})
	}
}

func TestAsciidocToMarkdownWithoutBaseURL(t *testing.T) {
	assert.Equal(t, "See TLS and `x`.", docs.AsciidocToMarkdown("See xref:components:inputs/kafka.adoc#tls[TLS] and <<x>>.", "components/inputs/foo", docs.MarkdownOptions{}))
}
