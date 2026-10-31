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
		HasComponent:  func(typeDir, name string) bool { return name != "grok" },
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
			name: "a bare <placeholder> in prose is escaped so Markdown renderers show it",
			in:   "GRANT SET CONTAINER TO <user> and `<user>` and <https://x.example/a> and a<b.",
			out:  "GRANT SET CONTAINER TO \\<user> and `<user>` and <https://x.example/a> and a\\<b.",
		},
		{
			name: "a +literal+ code span drops its plus signs",
			in:   "so `+logs_.*+` matches, and `a+b+c` stays.",
			out:  "so `logs_.*` matches, and `a+b+c` stays.",
		},
		{
			name: "a [[id]] block anchor is dropped",
			in:   "Text.\n\n[[merge-strategies]]\n=== Merge strategies\n\nMore.",
			out:  "Text.\n\n### Merge strategies\n\nMore.",
		},
		{
			name: "unconstrained __italic__ stays italic",
			in:   "will __allow out-of-order delivery__ since, but `__x__` is code.",
			out:  "will _allow out-of-order delivery_ since, but `__x__` is code.",
		},
		{
			name: "unconstrained __italic__ inside a word keeps asterisks",
			in:   "a foo__bar__baz word",
			out:  "a foo*bar*baz word",
		},
		{
			name: "only an escaped attribute reference loses its backslash",
			in:   "pattern `%\\{WORD:first}`, interface \\{} is nil, $\\{!count}, \\{empty} and {empty}x.",
			out:  "pattern `%\\{WORD:first}`, interface \\{} is nil, $\\{!count}, {empty} and x.",
		},
		{
			name: "xref with code in its text",
			in:   "Use the xref:components:outputs/redpanda.adoc[`redpanda` output] instead.",
			out:  "Use the [`redpanda` output](https://docs.redpanda.com/connect/components/outputs/redpanda/) instead.",
		},
		{
			name: "xref to a component this distribution lacks becomes text",
			in:   "Try xref:components:processors/grok.adoc[`grok`] or xref:components:processors/about.adoc[processors].",
			out:  "Try `grok` or [processors](https://docs.redpanda.com/connect/components/processors/about/).",
		},
		{
			name: "component-qualified xrefs use the component URL, or text when unknown",
			in:   "xref:streaming:manage:a.adoc[A] and xref:other:b:c.adoc[C]",
			out:  "[A](https://docs.redpanda.com/streaming/manage/a/) and C",
		},
		{
			name: "module-less xref is relative to the page's module",
			in:   "See xref:outputs/bigquery_cdc_migration.adoc[the guide].",
			out:  "See [the guide](https://docs.redpanda.com/connect/components/outputs/bigquery_cdc_migration/).",
		},
		{
			name: "empty xref text gets a readable label",
			in:   "See xref:configuration:error_handling.adoc[] and xref:guides:bloblang/about.adoc#methods[].",
			out:  "See [error handling](https://docs.redpanda.com/connect/configuration/error_handling/) and [methods](https://docs.redpanda.com/connect/guides/bloblang/about/#methods).",
		},
		{
			name: "link text keeps escaped brackets",
			in:   "DSN: https://example.com/dsn[`clickhouse://[user[:pass\\]@\\]host/db`^].",
			out:  "DSN: [`clickhouse://[user[:pass]@]host/db`](https://example.com/dsn).",
		},
		{
			name: "link attributes are dropped from the text",
			in:   "See https://example.com[Example,window=_blank] and link:https://example.com[example].",
			out:  "See [Example](https://example.com) and [example](https://example.com).",
		},
		{
			name: "anchors resolve to the page",
			in:   "Set <<checkpoint_limit,`checkpoint_limit`>> or <<tls-enabled>>.",
			out:  "Set [`checkpoint_limit`](https://docs.redpanda.com/connect/components/inputs/foo/#checkpoint_limit) or [`tls-enabled`](https://docs.redpanda.com/connect/components/inputs/foo/#tls-enabled).",
		},
		{
			name: "ifndef env-cloud keeps text outside Cloud",
			in:   "A.\n\nifndef::env-cloud[]\nSelf-managed.\nendif::[]\n\nifdef::env-cloud[]\nCloud.\nendif::[]",
			out:  "A.\n\nSelf-managed.\n",
		},
		{
			name:  "ifndef env-cloud drops text in Cloud",
			in:    "A.\n\nifndef::env-cloud[]\nSelf-managed.\nendif::[]\n\nifdef::env-cloud[]\nCloud.\nendif::[]",
			out:   "A.\n\nCloud.",
			attrs: []string{"env-cloud"},
		},
		{
			name:  "a named endif closes its block",
			in:    "A.\n\nifndef::env-cloud[]\nSelf-managed.\nendif::env-cloud[]\n\nB.",
			out:   "A.\n\nB.",
			attrs: []string{"env-cloud"},
		},
		{
			name:  "comma means any attribute, plus means all",
			in:    "ifdef::a,b[]\nany\nendif::[]\nifdef::a+b[]\nall\nendif::[]",
			out:   "any",
			attrs: []string{"a"},
		},
		{
			name:  "directives inside code blocks are still evaluated",
			in:    "```yaml\nx: 1\nifdef::env-cloud[]\ny: 2\nendif::[]\n```",
			out:   "```yaml\nx: 1\ny: 2\n```",
			attrs: []string{"env-cloud"},
		},
		{
			name: "an indented directive is text, as in Asciidoctor",
			in:   "- item\n  ifndef::env-cloud[]\n  more\n  endif::[]",
			out:  "- item\n  ifndef::env-cloud[]\n  more\n  endif::[]",
		},
		{
			name: "headings, admonitions, and bold",
			in:   "== Performance\n\nNOTE: This is *important*, but *.json and 2 * 3 are not bold.",
			out:  "## Performance\n\n> **Note:** This is **important**, but *.json and 2 * 3 are not bold.",
		},
		{
			name: "bold may span a code span, but asterisks in code never open one",
			in:   "*Merge-key (`identifier_fields`) types:* `int`. Pattern `a*b` and `*c*` stay.",
			out:  "**Merge-key (`identifier_fields`) types:** `int`. Pattern `a*b` and `*c*` stay.",
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
			name: "ordered and nested lists",
			in:   ". First\n. Second\n* top\n** nested",
			out:  "1. First\n1. Second\n- top\n  - nested",
		},
		{
			name: "passthroughs keep plus signs in prose",
			in:   "Use ++*literal*++ here, and parent + children + grandchildren, C++ too, and +mono+.",
			out:  "Use *literal* here, and parent + children + grandchildren, C++ too, and mono.",
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
			name: "psv tables with one cell per line and cell specifiers",
			in:   "|===\n|Name |Value\n\na|a\n2+|1\n\n|b\n|2\n|===",
			out:  "| Name | Value |\n| --- | --- |\n| a | 1 |\n| b | 2 |",
		},
		{
			name: "escaped braces and attribute references",
			in:   "Use `$\\{REDPANDA_BROKERS}`{nbsp}now, GET `/inputs/\\{id}`, a{plus}b, {startsb}x{endsb}.",
			out:  "Use `${REDPANDA_BROKERS}` now, GET `/inputs/{id}`, a+b, [x].",
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

func TestLintConditionals(t *testing.T) {
	for _, test := range []struct {
		name, in string
		problems int
		contains string
	}{
		{name: "the supported form is fine", in: "A.\n\nifndef::env-cloud[]\nB.\nendif::[]\n\nC."},
		{name: "a block at the start and end needs no blank lines", in: "ifdef::env-cloud[]\nB.\nendif::[]"},
		{name: "unclosed block", in: "ifndef::env-cloud[]\nB.", problems: 1, contains: "never closed"},
		{name: "stray endif", in: "B.\nendif::[]", problems: 1, contains: "without a matching"},
		{name: "unknown attribute", in: "ifndef::env_cloud[]\nB.\nendif::[]", problems: 1, contains: "unknown attribute \"env_cloud\"; known attributes: env-cloud"},
		{name: "single-line form", in: "ifdef::env-cloud[Cloud only.]", problems: 1, contains: "block form"},
		{name: "indented directive", in: "- a\n  ifndef::env-cloud[]\n  b\n  endif::[]", problems: 2, contains: "column 0"},
		{name: "glued to the paragraphs around it", in: "A.\nifndef::env-cloud[]\nB.\nendif::[]\nC.", problems: 2, contains: "blank line"},
		{name: "ifeval", in: "ifeval::[1 == 1]\nB.\nendif::[]", problems: 2, contains: "ifeval"},
		{name: "trailing whitespace after a directive", in: "A.\n\nifdef::env-cloud[]   \nB.\nendif::[]", problems: 2, contains: "remove the whitespace"},
		{name: "a directive inside a fenced code block", in: "A.\n\n```yaml\nifdef::env-cloud[]\nx: 1\nendif::[]\n```\n\nB.", problems: 2, contains: "inside a code block"},
		{name: "a directive inside a ---- listing", in: "A.\n\n----\nifdef::env-cloud[]\n----\n\nB.", problems: 1, contains: "inside a code block"},
		{name: "a directive after a closed fence is fine", in: "```\nx\n```\n\nifdef::env-cloud[]\nB.\nendif::[]"},
		{name: "opposite nesting renders for no reader", in: "ifdef::env-cloud[]\nA.\n\nifndef::env-cloud[]\nB.\nendif::[]\n\nendif::[]", problems: 1, contains: "renders for no reader"},
		{name: "non-ASCII whitespace after a directive", in: "A.\n\nifdef::env-cloud[]\u00a0\nB.\nendif::[]", problems: 2, contains: "remove the whitespace"},
		{name: "vertical tab before a directive", in: "A.\n\n\vifdef::env-cloud[]\nB.\nendif::[]", problems: 2, contains: "column 0"},
		{name: "a named endif that does not match", in: "ifdef::env-cloud[]\nB.\nendif::foo[]", problems: 1, contains: "does not match"},
		{name: "a matching named endif is fine", in: "ifdef::env-cloud[]\nB.\nendif::env-cloud[]"},
		{name: "mixed , and + in one list", in: "ifndef::env-cloud,env-cloud+env-cloud[]\nB.\nendif::[]", problems: 1, contains: "mixes , and +"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := docs.LintConditionals(test.in)
			assert.Len(t, got, test.problems, "%v", got)
			if test.contains != "" {
				assert.Contains(t, got[0], test.contains)
			}
		})
	}
}
