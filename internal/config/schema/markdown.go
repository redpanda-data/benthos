// Copyright 2026 Redpanda Data, Inc.

package schema

import (
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// RenderMarkdown rewrites every doc string in the schema from AsciiDoc to
// Markdown, with links resolved to the docs site, links to components this
// schema lacks reduced to text, and env conditionals resolved for
// opts.Attributes. Config snippets and Bloblang mappings are left as they
// are. It returns the conditional problems docs.LintConditionals finds in any
// string, each prefixed with where it sits, because a malformed conditional
// would otherwise drop text silently.
func (f *Full) RenderMarkdown(opts docs.MarkdownOptions) []string {
	families := []struct {
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
	}
	if opts.HasComponent == nil {
		have := map[string]bool{}
		for _, fam := range families {
			for _, c := range fam.specs {
				have[fam.dir+"/"+c.Name] = true
			}
		}
		opts.HasComponent = func(typeDir, name string) bool { return have[typeDir+"/"+name] }
	}

	r := renderer{opts: opts}
	f.Config = r.fields(f.Config, "", "config")
	for _, fam := range families {
		for i := range fam.specs {
			r.component(&fam.specs[i], "components/"+fam.dir+"/"+fam.specs[i].Name)
		}
	}
	for i := range f.BloblangFunctions {
		s := &f.BloblangFunctions[i]
		where := "bloblang function " + s.Name
		s.Description = r.render(s.Description, "guides/bloblang/functions", where)
		s.Params = r.params(s.Params, "guides/bloblang/functions", where)
		s.Examples = r.examples(s.Examples, "guides/bloblang/functions", where)
	}
	for i := range f.BloblangMethods {
		s := &f.BloblangMethods[i]
		where := "bloblang method " + s.Name
		s.Description = r.render(s.Description, "guides/bloblang/methods", where)
		s.Params = r.params(s.Params, "guides/bloblang/methods", where)
		s.Examples = r.examples(s.Examples, "guides/bloblang/methods", where)
		cats := make([]query.MethodCatSpec, len(s.Categories))
		for j, c := range s.Categories {
			c.Description = r.render(c.Description, "guides/bloblang/methods", where+" ("+c.Category+")")
			c.Examples = r.examples(c.Examples, "guides/bloblang/methods", where)
			cats[j] = c
		}
		s.Categories = cats
	}
	f.DocsFormat = "markdown"
	return r.problems
}

type renderer struct {
	opts     docs.MarkdownOptions
	problems []string
}

func (r *renderer) render(s, page, where string) string {
	for _, p := range docs.LintConditionals(s) {
		r.problems = append(r.problems, where+": "+p)
	}
	return docs.AsciidocToMarkdown(s, page, r.opts)
}

func (r *renderer) component(c *docs.ComponentSpec, page string) {
	where := strings.TrimPrefix(page, "components/")
	if strings.Contains(c.Summary, "ifdef::") || strings.Contains(c.Summary, "ifndef::") {
		r.problems = append(r.problems, where+": the summary has a conditional; summaries render as one line in catalogs and page metadata, so put the conditional text in the description")
	}
	c.Summary = r.render(c.Summary, page, where+" summary")
	c.Description = r.render(c.Description, page, where+" description")
	c.Footnotes = r.render(c.Footnotes, page, where+" footnotes")
	for i := range c.Examples {
		c.Examples[i].Summary = r.render(c.Examples[i].Summary, page, where+" example "+c.Examples[i].Title)
	}
	c.Config = r.field(c.Config, page, where)
}

func (r *renderer) params(p query.Params, page, where string) query.Params {
	defs := make([]query.ParamDefinition, len(p.Definitions))
	for i, d := range p.Definitions {
		d.Description = r.render(d.Description, page, where+" param "+d.Name)
		defs[i] = d
	}
	p.Definitions = defs
	return p
}

func (r *renderer) examples(ex []query.ExampleSpec, page, where string) []query.ExampleSpec {
	out := make([]query.ExampleSpec, len(ex))
	for i, e := range ex {
		e.Summary = r.render(e.Summary, page, where+" example")
		out[i] = e
	}
	return out
}

func (r *renderer) fields(fs docs.FieldSpecs, page, where string) docs.FieldSpecs {
	out := make(docs.FieldSpecs, len(fs))
	for i, f := range fs {
		out[i] = r.field(f, page, where)
	}
	return out
}

func (r *renderer) field(f docs.FieldSpec, page, where string) docs.FieldSpec {
	if f.Name != "" {
		where += " field " + f.Name
	}
	f.Description = r.render(f.Description, page, where)
	if len(f.AnnotatedOptions) > 0 {
		opts := make([][2]string, len(f.AnnotatedOptions))
		for i, o := range f.AnnotatedOptions {
			opts[i] = [2]string{o[0], r.render(o[1], page, where+" option "+o[0])}
		}
		f.AnnotatedOptions = opts
	}
	f.Children = r.fields(f.Children, page, where)
	return f
}
