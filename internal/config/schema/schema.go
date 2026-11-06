// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang"
	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// Full represents the entirety of the Benthos instances configuration spec and
// all plugins.
type Full struct {
	Version string `json:"version"`
	Date    string `json:"date"`
	// DocsFormat says how the doc strings are written: "asciidoc" as the
	// specs hold them, or "markdown" after RenderMarkdown. Consumers that
	// render them check this rather than guessing.
	DocsFormat        string               `json:"docs_format"`
	Config            docs.FieldSpecs      `json:"config,omitempty"`
	Buffers           []docs.ComponentSpec `json:"buffers,omitempty"`
	Caches            []docs.ComponentSpec `json:"caches,omitempty"`
	Inputs            []docs.ComponentSpec `json:"inputs,omitempty"`
	Outputs           []docs.ComponentSpec `json:"outputs,omitempty"`
	Processors        []docs.ComponentSpec `json:"processors,omitempty"`
	RateLimits        []docs.ComponentSpec `json:"rate-limits,omitempty"`
	Metrics           []docs.ComponentSpec `json:"metrics,omitempty"`
	Tracers           []docs.ComponentSpec `json:"tracers,omitempty"`
	Scanners          []docs.ComponentSpec `json:"scanners,omitempty"`
	BloblangFunctions []query.FunctionSpec `json:"bloblang-functions,omitempty"`
	BloblangMethods   []query.MethodSpec   `json:"bloblang-methods,omitempty"`
}

// New walks all registered Benthos components and creates a full schema
// definition of it.
func New(version, date string, env *bundle.Environment, bEnv *bloblang.Environment) Full {
	s := Full{
		Version:    version,
		Date:       date,
		DocsFormat: "asciidoc",
		Config:     config.Spec(),
		Buffers:    env.BufferDocs(),
		Caches:     env.CacheDocs(),
		Inputs:     env.InputDocs(),
		Outputs:    env.OutputDocs(),
		Processors: env.ProcessorDocs(),
		RateLimits: env.RateLimitDocs(),
		Metrics:    env.MetricsDocs(),
		Tracers:    env.TracersDocs(),
		Scanners:   env.ScannerDocs(),
	}
	bEnv.WalkFunctions(func(name string, spec query.FunctionSpec) {
		s.BloblangFunctions = append(s.BloblangFunctions, spec)
	})
	bEnv.WalkMethods(func(name string, spec query.MethodSpec) {
		s.BloblangMethods = append(s.BloblangMethods, spec)
	})
	return s
}

func ofStatus(status string, components []docs.ComponentSpec) []docs.ComponentSpec {
	var newComps []docs.ComponentSpec
	for _, c := range components {
		if c.Status == docs.Status(status) {
			newComps = append(newComps, c)
		}
	}
	return newComps
}

// ReduceToStatus reduces the components in the schema to only those matching
// the given stability status.
func (f *Full) ReduceToStatus(status string) {
	f.Buffers = ofStatus(status, f.Buffers)
	f.Caches = ofStatus(status, f.Caches)
	f.Inputs = ofStatus(status, f.Inputs)
	f.Outputs = ofStatus(status, f.Outputs)
	f.Processors = ofStatus(status, f.Processors)
	f.RateLimits = ofStatus(status, f.RateLimits)
	f.Metrics = ofStatus(status, f.Metrics)
	f.Tracers = ofStatus(status, f.Tracers)
	f.Scanners = ofStatus(status, f.Scanners)

	var newFuncs []query.FunctionSpec
	for _, s := range f.BloblangFunctions {
		if s.Status == query.Status(status) {
			newFuncs = append(newFuncs, s)
		}
	}
	f.BloblangFunctions = newFuncs

	var newMethods []query.MethodSpec
	for _, s := range f.BloblangMethods {
		if s.Status == query.Status(status) {
			newMethods = append(newMethods, s)
		}
	}
	f.BloblangMethods = newMethods
}

func justNames(components []docs.ComponentSpec) []string {
	names := []string{}
	for _, c := range components {
		if c.Status != docs.StatusDeprecated {
			names = append(names, c.Name)
		}
	}
	return names
}

func justNamesBloblFuncs(fns []query.FunctionSpec) []string {
	names := []string{}
	for _, c := range fns {
		if c.Status != query.StatusDeprecated {
			names = append(names, c.Name)
		}
	}
	return names
}

func justNamesBloblMethods(fns []query.MethodSpec) []string {
	names := []string{}
	for _, c := range fns {
		if c.Status != query.StatusDeprecated {
			names = append(names, c.Name)
		}
	}
	return names
}

// Flattened returns a flattened representation of all registered plugin types
// and names.
func (f *Full) Flattened() map[string][]string {
	return map[string][]string{
		"buffers":            justNames(f.Buffers),
		"caches":             justNames(f.Caches),
		"inputs":             justNames(f.Inputs),
		"outputs":            justNames(f.Outputs),
		"processors":         justNames(f.Processors),
		"rate-limits":        justNames(f.RateLimits),
		"metrics":            justNames(f.Metrics),
		"tracers":            justNames(f.Tracers),
		"scanners":           justNames(f.Scanners),
		"bloblang-functions": justNamesBloblFuncs(f.BloblangFunctions),
		"bloblang-methods":   justNamesBloblMethods(f.BloblangMethods),
	}
}

// Scrub walks the schema and removes all descriptions and other long-form
// documentation, reducing the overall size.
func (f *Full) Scrub() {
	// Every slice here can share its backing array with the environment's
	// registry, or with the distribution's config spec, so scrub into copies.
	// Writing through them would blank the docs of every later schema built
	// from the same environment.
	f.Config = scrubFieldSpecs(f.Config)
	f.Buffers = scrubComponentSpecs(f.Buffers)
	f.Caches = scrubComponentSpecs(f.Caches)
	f.Inputs = scrubComponentSpecs(f.Inputs)
	f.Outputs = scrubComponentSpecs(f.Outputs)
	f.Processors = scrubComponentSpecs(f.Processors)
	f.RateLimits = scrubComponentSpecs(f.RateLimits)
	f.Metrics = scrubComponentSpecs(f.Metrics)
	f.Tracers = scrubComponentSpecs(f.Tracers)
	f.Scanners = scrubComponentSpecs(f.Scanners)

	if f.BloblangFunctions != nil {
		fns := make([]query.FunctionSpec, len(f.BloblangFunctions))
		for i, fn := range f.BloblangFunctions {
			fn.Description = ""
			fn.Examples = nil
			fn.Params.Definitions = scrubParams(fn.Params.Definitions)
			fns[i] = fn
		}
		f.BloblangFunctions = fns
	}
	if f.BloblangMethods != nil {
		methods := make([]query.MethodSpec, len(f.BloblangMethods))
		for i, m := range f.BloblangMethods {
			m.Description = ""
			m.Examples = nil
			m.Categories = nil
			m.Params.Definitions = scrubParams(m.Params.Definitions)
			methods[i] = m
		}
		f.BloblangMethods = methods
	}
}

func scrubParams(p []query.ParamDefinition) []query.ParamDefinition {
	if p == nil {
		return nil
	}
	out := make([]query.ParamDefinition, len(p))
	for i, d := range p {
		d.Description = ""
		out[i] = d
	}
	return out
}

func scrubFieldSpecs(fs []docs.FieldSpec) []docs.FieldSpec {
	if fs == nil {
		return nil
	}
	out := make([]docs.FieldSpec, len(fs))
	for i, f := range fs {
		out[i] = scrubFieldSpec(f)
	}
	return out
}

func scrubFieldSpec(f docs.FieldSpec) docs.FieldSpec {
	f.Description = ""
	f.Examples = nil
	if f.AnnotatedOptions != nil {
		opts := make([][2]string, len(f.AnnotatedOptions))
		for j, o := range f.AnnotatedOptions {
			opts[j] = [2]string{o[0], ""}
		}
		f.AnnotatedOptions = opts
	}
	f.Children = scrubFieldSpecs(f.Children)
	return f
}

func scrubComponentSpecs(cs []docs.ComponentSpec) []docs.ComponentSpec {
	if cs == nil {
		return nil
	}
	out := make([]docs.ComponentSpec, len(cs))
	for i, c := range cs {
		c.Description = ""
		c.Summary = ""
		c.Footnotes = ""
		c.Examples = nil
		c.Config = scrubFieldSpec(c.Config)
		out[i] = c
	}
	return out
}
