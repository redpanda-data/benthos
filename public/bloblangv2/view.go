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

// UnmarshalJSON normalises Default against Kind so that round-trip through
// JSON preserves Go-typed semantics. encoding/json decodes every numeric
// literal as float64; without this hook a parameter declared with
// NewInt64Param("x").Default(int64(5)) would round-trip with Default of type
// float64, leaving the typed Kind ("int64") inconsistent with the stored
// value.
func (p *PluginParamInfo) UnmarshalJSON(b []byte) error {
	type alias PluginParamInfo
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	a.Default = coerceDefaultByKind(a.Default, a.Kind)
	*p = PluginParamInfo(a)
	return nil
}

// coerceDefaultByKind brings a JSON-decoded default value back into the Go
// type implied by the parameter's Kind. Only numeric kinds need coercion;
// bool and string already round-trip with their declared types intact.
func coerceDefaultByKind(v any, kind string) any {
	switch kind {
	case "int64":
		if f, ok := v.(float64); ok {
			return int64(f)
		}
	case "float64":
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return v
}

// PluginInfo is a JSON-serialisable description of a registered V2 method or
// function. Use Environment.WalkMethods / Environment.WalkFunctions to obtain
// these via the FunctionView / MethodView wrappers.
type PluginInfo struct {
	Name        string `json:"name"`
	Status      string `json:"status,omitempty"`
	Category    string `json:"category,omitempty"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	Impure      bool   `json:"impure,omitempty"`
	// RequiresMessageContext signals that the function reads from a
	// pipeline message (batch position, content bytes, error, tracing).
	// Such functions only resolve when the executor is run via
	// Executor.QueryMessage; calls from a plain Query / QueryMetadata
	// path produce a runtime error. Tooling (linters, docs generators)
	// can use the flag to gate suggestions.
	RequiresMessageContext bool              `json:"requires_message_context,omitempty"`
	Params                 []PluginParamInfo `json:"params,omitempty"`
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
		Name:                   name,
		Status:                 string(spec.status),
		Category:               spec.category,
		Description:            spec.description,
		Version:                spec.version,
		Impure:                 spec.impure,
		RequiresMessageContext: spec.requiresMessageContext,
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
	case paramKindLambda:
		return "lambda"
	default:
		return "any"
	}
}

// NewPluginSpecFromInfo reconstructs a PluginSpec from a serialised PluginInfo
// description. This is the reverse of the per-plugin information emitted by
// Environment.WalkFunctions / WalkMethods and is intended for tooling that
// loads schemas serialised from a separate process (e.g. a remote linter
// rebuilding stub registrations from a JSON schema dump).
//
// The reconstruction is performed exclusively through the public builder
// chain (NewPluginSpec, NewStringParam / NewInt64Param / ..., Description,
// Optional, Default, etc.) so that any validation enforced by those
// constructors also applies to round-tripped specs. Unknown status strings
// fall back to the default (stable) status.
//
// Experimental: this function is intended for tooling and may change without
// notice.
func NewPluginSpecFromInfo(info PluginInfo) *PluginSpec {
	spec := NewPluginSpec().
		Description(info.Description).
		Category(info.Category).
		Version(info.Version)
	if info.Impure {
		spec = spec.Impure()
	}
	if info.RequiresMessageContext {
		spec.requiresMessageContext = true
	}
	switch info.Status {
	case string(statusExperimental):
		spec = spec.Experimental()
	case string(statusBeta):
		spec = spec.Beta()
	case string(statusDeprecated):
		spec = spec.Deprecated()
	}
	for _, p := range info.Params {
		spec = spec.Param(paramFromInfo(p))
	}
	return spec
}

func paramFromInfo(p PluginParamInfo) ParamDefinition {
	var def ParamDefinition
	switch p.Kind {
	case "string":
		def = NewStringParam(p.Name)
	case "int64":
		def = NewInt64Param(p.Name)
	case "float64":
		def = NewFloat64Param(p.Name)
	case "bool":
		def = NewBoolParam(p.Name)
	case "lambda":
		def = NewLambdaParam(p.Name)
	default:
		def = NewAnyParam(p.Name)
	}
	if p.Description != "" {
		def = def.Description(p.Description)
	}
	switch {
	case p.HasDefault:
		def = def.Default(p.Default)
	case p.IsOptional:
		def = def.Optional()
	}
	return def
}
