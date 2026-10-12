// Copyright 2026 Redpanda Data, Inc.

package schema

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// RenderMarkdown rewrites every doc string in the schema from AsciiDoc to
// Markdown, with links resolved to the docs site and env conditionals
// resolved for opts.Attributes. Config snippets and Bloblang mappings are left
// as they are.
func (f *Full) RenderMarkdown(opts docs.MarkdownOptions) {
	f.Config = renderFields(f.Config, "", opts)
	for _, c := range []struct {
		dir   string
		specs []docs.ComponentSpec
	}{
		{"buffers", f.Buffers},
		{"caches", f.Caches},
		{"inputs", f.Inputs},
		{"outputs", f.Outputs},
		{"processors", f.Processors},
		{"rate_limits", f.RateLimits},
		{"metrics", f.Metrics},
		{"tracers", f.Tracers},
		{"scanners", f.Scanners},
	} {
		for i := range c.specs {
			renderComponent(&c.specs[i], "components/"+c.dir+"/"+c.specs[i].Name, opts)
		}
	}
	for i := range f.BloblangFunctions {
		renderFunction(&f.BloblangFunctions[i], "guides/bloblang/functions", opts)
	}
	for i := range f.BloblangMethods {
		s := &f.BloblangMethods[i]
		s.Description = docs.AsciidocToMarkdown(s.Description, "guides/bloblang/methods", opts)
		s.Params = renderParams(s.Params, "guides/bloblang/methods", opts)
		s.Examples = renderExamples(s.Examples, "guides/bloblang/methods", opts)
		cats := make([]query.MethodCatSpec, len(s.Categories))
		for j, c := range s.Categories {
			c.Description = docs.AsciidocToMarkdown(c.Description, "guides/bloblang/methods", opts)
			c.Examples = renderExamples(c.Examples, "guides/bloblang/methods", opts)
			cats[j] = c
		}
		s.Categories = cats
	}
}

func renderComponent(c *docs.ComponentSpec, page string, opts docs.MarkdownOptions) {
	c.Summary = docs.AsciidocToMarkdown(c.Summary, page, opts)
	c.Description = docs.AsciidocToMarkdown(c.Description, page, opts)
	c.Footnotes = docs.AsciidocToMarkdown(c.Footnotes, page, opts)
	for i := range c.Examples {
		c.Examples[i].Summary = docs.AsciidocToMarkdown(c.Examples[i].Summary, page, opts)
	}
	c.Config = renderField(c.Config, page, opts)
}

func renderFunction(s *query.FunctionSpec, page string, opts docs.MarkdownOptions) {
	s.Description = docs.AsciidocToMarkdown(s.Description, page, opts)
	s.Params = renderParams(s.Params, page, opts)
	s.Examples = renderExamples(s.Examples, page, opts)
}

func renderParams(p query.Params, page string, opts docs.MarkdownOptions) query.Params {
	defs := make([]query.ParamDefinition, len(p.Definitions))
	for i, d := range p.Definitions {
		d.Description = docs.AsciidocToMarkdown(d.Description, page, opts)
		defs[i] = d
	}
	p.Definitions = defs
	return p
}

func renderExamples(ex []query.ExampleSpec, page string, opts docs.MarkdownOptions) []query.ExampleSpec {
	out := make([]query.ExampleSpec, len(ex))
	for i, e := range ex {
		e.Summary = docs.AsciidocToMarkdown(e.Summary, page, opts)
		out[i] = e
	}
	return out
}

func renderFields(fs docs.FieldSpecs, page string, opts docs.MarkdownOptions) docs.FieldSpecs {
	out := make(docs.FieldSpecs, len(fs))
	for i, f := range fs {
		out[i] = renderField(f, page, opts)
	}
	return out
}

func renderField(f docs.FieldSpec, page string, opts docs.MarkdownOptions) docs.FieldSpec {
	f.Description = docs.AsciidocToMarkdown(f.Description, page, opts)
	if len(f.AnnotatedOptions) > 0 {
		opts2 := make([][2]string, len(f.AnnotatedOptions))
		for i, o := range f.AnnotatedOptions {
			opts2[i] = [2]string{o[0], docs.AsciidocToMarkdown(o[1], page, opts)}
		}
		f.AnnotatedOptions = opts2
	}
	f.Children = renderFields(f.Children, page, opts)
	return f
}
