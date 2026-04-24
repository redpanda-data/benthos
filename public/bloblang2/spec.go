// Copyright 2026 Redpanda Data, Inc.

package bloblang2

// pluginStatus captures the lifecycle stage of a plugin (stable, experimental,
// etc.). Used for documentation only; it has no effect on parsing or execution.
type pluginStatus string

const (
	statusStable       pluginStatus = ""
	statusExperimental pluginStatus = "experimental"
	statusBeta         pluginStatus = "beta"
	statusDeprecated   pluginStatus = "deprecated"
)

// paramKind classifies the expected Go type of a plugin argument. Values
// flowing through ParsedParams are coerced / validated against this kind.
type paramKind int

const (
	paramKindAny paramKind = iota
	paramKindString
	paramKindInt64
	paramKindFloat64
	paramKindBool
)

// ParamDefinition describes a single parameter accepted by a plugin. Build
// instances with the NewStringParam / NewInt64Param / ... constructors and
// chain Optional, Default, or Description as needed.
type ParamDefinition struct {
	name        string
	description string
	kind        paramKind
	optional    bool
	hasDefault  bool
	defaultVal  any
}

// NewStringParam creates a new string typed parameter.
func NewStringParam(name string) ParamDefinition {
	return ParamDefinition{name: name, kind: paramKindString}
}

// NewInt64Param creates a new 64-bit integer typed parameter.
func NewInt64Param(name string) ParamDefinition {
	return ParamDefinition{name: name, kind: paramKindInt64}
}

// NewFloat64Param creates a new float64 typed parameter.
func NewFloat64Param(name string) ParamDefinition {
	return ParamDefinition{name: name, kind: paramKindFloat64}
}

// NewBoolParam creates a new bool typed parameter.
func NewBoolParam(name string) ParamDefinition {
	return ParamDefinition{name: name, kind: paramKindBool}
}

// NewAnyParam creates a new parameter that accepts any value type.
func NewAnyParam(name string) ParamDefinition {
	return ParamDefinition{name: name, kind: paramKindAny}
}

// Description attaches an optional human-readable description to the
// parameter, used by documentation generators.
func (d ParamDefinition) Description(str string) ParamDefinition {
	d.description = str
	return d
}

// Optional marks the parameter as optional; callers may omit it.
func (d ParamDefinition) Optional() ParamDefinition {
	d.optional = true
	return d
}

// Default assigns a default value to the parameter, implicitly marking it
// optional.
func (d ParamDefinition) Default(v any) ParamDefinition {
	d.optional = true
	d.hasDefault = true
	d.defaultVal = v
	return d
}

// PluginSpec describes the signature and documentation of a plugin method or
// function. Build with NewPluginSpec, then chain Param, Description, etc.
type PluginSpec struct {
	status      pluginStatus
	category    string
	description string
	version     string
	impure      bool
	params      []ParamDefinition
	variadic    bool
}

// NewPluginSpec creates an empty plugin spec.
func NewPluginSpec() *PluginSpec {
	return &PluginSpec{}
}

// Description attaches a human-readable description to the plugin.
func (p *PluginSpec) Description(s string) *PluginSpec {
	p.description = s
	return p
}

// Category attaches an optional category string used by documentation
// generators to group related plugins.
func (p *PluginSpec) Category(s string) *PluginSpec {
	p.category = s
	return p
}

// Version records the release in which the plugin was introduced.
func (p *PluginSpec) Version(v string) *PluginSpec {
	p.version = v
	return p
}

// Experimental flags the plugin as experimental.
func (p *PluginSpec) Experimental() *PluginSpec {
	p.status = statusExperimental
	return p
}

// Beta flags the plugin as beta-quality.
func (p *PluginSpec) Beta() *PluginSpec {
	p.status = statusBeta
	return p
}

// Deprecated flags the plugin as deprecated. It remains callable but is
// de-emphasised in documentation.
func (p *PluginSpec) Deprecated() *PluginSpec {
	p.status = statusDeprecated
	return p
}

// Impure marks the plugin as having side effects or observing state outside
// the mapping (e.g. reading env vars). Impure plugins are stripped from
// environments produced by Environment.OnlyPure.
func (p *PluginSpec) Impure() *PluginSpec {
	p.impure = true
	return p
}

// Variadic declares the plugin accepts an arbitrary number of positional
// arguments. It is invalid to combine Variadic with Param.
func (p *PluginSpec) Variadic() *PluginSpec {
	p.variadic = true
	return p
}

// Param appends a parameter to the plugin spec. Positional arguments must be
// supplied in the order Param is called.
func (p *PluginSpec) Param(d ParamDefinition) *PluginSpec {
	p.params = append(p.params, d)
	return p
}
