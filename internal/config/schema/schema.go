// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang"
	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// Full represents the entirety of the Benthos instances configuration spec and
// all plugins.
type Full struct {
	Version             string                  `json:"version"`
	Date                string                  `json:"date"`
	Config              docs.FieldSpecs         `json:"config,omitempty"`
	Buffers             []docs.ComponentSpec    `json:"buffers,omitempty"`
	Caches              []docs.ComponentSpec    `json:"caches,omitempty"`
	Inputs              []docs.ComponentSpec    `json:"inputs,omitempty"`
	Outputs             []docs.ComponentSpec    `json:"outputs,omitempty"`
	Processors          []docs.ComponentSpec    `json:"processors,omitempty"`
	RateLimits          []docs.ComponentSpec    `json:"rate-limits,omitempty"`
	Metrics             []docs.ComponentSpec    `json:"metrics,omitempty"`
	Tracers             []docs.ComponentSpec    `json:"tracers,omitempty"`
	Scanners            []docs.ComponentSpec    `json:"scanners,omitempty"`
	BloblangFunctions   []query.FunctionSpec    `json:"bloblang-functions,omitempty"`
	BloblangMethods     []query.MethodSpec      `json:"bloblang-methods,omitempty"`
	BloblangV2Functions []bloblangv2.PluginInfo `json:"bloblang-v2-functions,omitempty"`
	BloblangV2Methods   []bloblangv2.PluginInfo `json:"bloblang-v2-methods,omitempty"`
}

// New walks all registered Benthos components and creates a full schema
// definition of it.
func New(version, date string, env *bundle.Environment, bEnv *bloblang.Environment, bV2Env *bloblangv2.Environment) Full {
	s := Full{
		Version:    version,
		Date:       date,
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
	if bV2Env != nil {
		bV2Env.WalkFunctions(func(_ string, view *bloblangv2.FunctionView) {
			s.BloblangV2Functions = append(s.BloblangV2Functions, view.Info())
		})
		bV2Env.WalkMethods(func(_ string, view *bloblangv2.MethodView) {
			s.BloblangV2Methods = append(s.BloblangV2Methods, view.Info())
		})
	}
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

	// V2 plugin status reuses the same status string vocabulary; an empty
	// status is equivalent to "stable" for filtering purposes.
	v2Match := func(specStatus string) bool {
		if specStatus == "" {
			return status == "stable"
		}
		return specStatus == status
	}
	var newV2Funcs []bloblangv2.PluginInfo
	for _, s := range f.BloblangV2Functions {
		if v2Match(s.Status) {
			newV2Funcs = append(newV2Funcs, s)
		}
	}
	f.BloblangV2Functions = newV2Funcs

	var newV2Methods []bloblangv2.PluginInfo
	for _, s := range f.BloblangV2Methods {
		if v2Match(s.Status) {
			newV2Methods = append(newV2Methods, s)
		}
	}
	f.BloblangV2Methods = newV2Methods
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

func justNamesBloblV2(specs []bloblangv2.PluginInfo) []string {
	names := []string{}
	for _, s := range specs {
		if s.Status != "deprecated" {
			names = append(names, s.Name)
		}
	}
	return names
}

// Flattened returns a flattened representation of all registered plugin types
// and names.
func (f *Full) Flattened() map[string][]string {
	return map[string][]string{
		"buffers":               justNames(f.Buffers),
		"caches":                justNames(f.Caches),
		"inputs":                justNames(f.Inputs),
		"outputs":               justNames(f.Outputs),
		"processors":            justNames(f.Processors),
		"rate-limits":           justNames(f.RateLimits),
		"metrics":               justNames(f.Metrics),
		"tracers":               justNames(f.Tracers),
		"scanners":              justNames(f.Scanners),
		"bloblang-functions":    justNamesBloblFuncs(f.BloblangFunctions),
		"bloblang-methods":      justNamesBloblMethods(f.BloblangMethods),
		"bloblang-v2-functions": justNamesBloblV2(f.BloblangV2Functions),
		"bloblang-v2-methods":   justNamesBloblV2(f.BloblangV2Methods),
	}
}

// Scrub walks the schema and removes all descriptions and other long-form
// documentation, reducing the overall size.
func (f *Full) Scrub() {
	scrubFieldSpecs(f.Config)
	scrubComponentSpecs(f.Buffers)
	scrubComponentSpecs(f.Caches)
	scrubComponentSpecs(f.Inputs)
	scrubComponentSpecs(f.Outputs)
	scrubComponentSpecs(f.Processors)
	scrubComponentSpecs(f.RateLimits)
	scrubComponentSpecs(f.Metrics)
	scrubComponentSpecs(f.Tracers)
	scrubComponentSpecs(f.Scanners)

	for i := range f.BloblangFunctions {
		f.BloblangFunctions[i].Description = ""
		f.BloblangFunctions[i].Examples = nil
		scrubParams(f.BloblangFunctions[i].Params.Definitions)
	}
	for i := range f.BloblangMethods {
		f.BloblangMethods[i].Description = ""
		f.BloblangMethods[i].Examples = nil
		f.BloblangMethods[i].Categories = nil
		scrubParams(f.BloblangMethods[i].Params.Definitions)
	}
	for i := range f.BloblangV2Functions {
		f.BloblangV2Functions[i].Description = ""
		scrubV2Params(f.BloblangV2Functions[i].Params)
	}
	for i := range f.BloblangV2Methods {
		f.BloblangV2Methods[i].Description = ""
		scrubV2Params(f.BloblangV2Methods[i].Params)
	}
}

func scrubV2Params(p []bloblangv2.PluginParamInfo) {
	for i := range p {
		p[i].Description = ""
	}
}

func scrubParams(p []query.ParamDefinition) {
	for i := range p {
		p[i].Description = ""
	}
}

func scrubFieldSpecs(fs []docs.FieldSpec) {
	for i := range fs {
		fs[i].Description = ""
		fs[i].Examples = nil
		for j := range fs[i].AnnotatedOptions {
			fs[i].AnnotatedOptions[j][1] = ""
		}
		scrubFieldSpecs(fs[i].Children)
	}
}

func scrubFieldSpec(fs *docs.FieldSpec) {
	fs.Description = ""
	scrubFieldSpecs(fs.Children)
}

func scrubComponentSpecs(cs []docs.ComponentSpec) {
	for i := range cs {
		cs[i].Description = ""
		cs[i].Summary = ""
		cs[i].Footnotes = ""
		cs[i].Examples = nil
		scrubFieldSpec(&cs[i].Config)
	}
}
