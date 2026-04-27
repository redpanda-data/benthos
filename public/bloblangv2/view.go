// Copyright 2026 Redpanda Data, Inc.

package bloblangv2

import (
	"encoding/json"
)

// PluginParamInfo is a JSON-serialisable description of a single plugin
// parameter, suitable for embedding in generated schemas. The JSON tag names
// mirror those used by the V1 bloblang schema for tooling consistency.
type PluginParamInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Kind is one of "any", "string", "int64", "float64", "bool".
	Kind       string `json:"kind"`
	IsOptional bool   `json:"is_optional,omitempty"`
	HasDefault bool   `json:"has_default,omitempty"`
	// Default is the default value for the parameter when one is set.
	Default any `json:"default,omitempty"`
}

// PluginInfo is a JSON-serialisable description of a registered V2 method or
// function. Use Environment.WalkMethods / Environment.WalkFunctions to obtain
// these via the FunctionView / MethodView wrappers.
type PluginInfo struct {
	Name        string            `json:"name"`
	Status      string            `json:"status,omitempty"`
	Category    string            `json:"category,omitempty"`
	Description string            `json:"description,omitempty"`
	Version     string            `json:"version,omitempty"`
	Impure      bool              `json:"impure,omitempty"`
	Params      []PluginParamInfo `json:"params,omitempty"`
}

// FunctionView describes a V2 function plugin registered against an
// Environment. Obtain instances via Environment.WalkFunctions.
type FunctionView struct {
	info PluginInfo
}

// Name returns the function name as used in mappings.
func (v *FunctionView) Name() string { return v.info.Name }

// Status returns the lifecycle stage of the function (stable, experimental,
// beta, deprecated). An empty string is equivalent to stable.
func (v *FunctionView) Status() string { return v.info.Status }

// Description returns the human-readable description, if one was provided.
func (v *FunctionView) Description() string { return v.info.Description }

// Info returns the underlying serialisable description of the function.
func (v *FunctionView) Info() PluginInfo { return v.info }

// FormatJSON returns the function description as a JSON object. The schema of
// the document is the PluginInfo struct.
//
// Experimental: this method is intended for tooling and may change without
// notice.
func (v *FunctionView) FormatJSON() ([]byte, error) {
	return json.Marshal(v.info)
}

// MethodView describes a V2 method plugin registered against an Environment.
// Obtain instances via Environment.WalkMethods.
type MethodView struct {
	info PluginInfo
}

// Name returns the method name as used in mappings.
func (v *MethodView) Name() string { return v.info.Name }

// Status returns the lifecycle stage of the method (stable, experimental,
// beta, deprecated). An empty string is equivalent to stable.
func (v *MethodView) Status() string { return v.info.Status }

// Description returns the human-readable description, if one was provided.
func (v *MethodView) Description() string { return v.info.Description }

// Info returns the underlying serialisable description of the method.
func (v *MethodView) Info() PluginInfo { return v.info }

// FormatJSON returns the method description as a JSON object. The schema of
// the document is the PluginInfo struct.
//
// Experimental: this method is intended for tooling and may change without
// notice.
func (v *MethodView) FormatJSON() ([]byte, error) {
	return json.Marshal(v.info)
}

// pluginInfoFromSpec converts a registered plugin spec into the public-facing
// PluginInfo description used by views and schema generators.
func pluginInfoFromSpec(name string, spec *PluginSpec) PluginInfo {
	if spec == nil {
		return PluginInfo{Name: name}
	}
	info := PluginInfo{
		Name:        name,
		Status:      string(spec.status),
		Category:    spec.category,
		Description: spec.description,
		Version:     spec.version,
		Impure:      spec.impure,
	}
	for _, p := range spec.params {
		info.Params = append(info.Params, PluginParamInfo{
			Name:        p.name,
			Description: p.description,
			Kind:        paramKindToString(p.kind),
			IsOptional:  p.optional,
			HasDefault:  p.hasDefault,
			Default:     p.defaultVal,
		})
	}
	return info
}

func paramKindToString(k paramKind) string {
	switch k {
	case paramKindString:
		return "string"
	case paramKindInt64:
		return "int64"
	case paramKindFloat64:
		return "float64"
	case paramKindBool:
		return "bool"
	default:
		return "any"
	}
}
