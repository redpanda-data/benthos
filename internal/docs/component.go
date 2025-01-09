// Copyright 2025 Redpanda Data, Inc.

package docs

// AnnotatedExample is an isolated example for a component.
type AnnotatedExample struct {
	// A title for the example.
	Title string `json:"title"`

	// Summary of the example.
	Summary string `json:"summary"`

	// A config snippet to show.
	Config string `json:"config"`
}

// Status of a component.
type Status string

// Component statuses.
var (
	StatusStable       Status = "stable"
	StatusBeta         Status = "beta"
	StatusExperimental Status = "experimental"
	StatusDeprecated   Status = "deprecated"
)

// Type of a component.
type Type string

// Component types.
var (
	TypeBuffer    Type = "buffer"
	TypeCache     Type = "cache"
	TypeInput     Type = "input"
	TypeMetrics   Type = "metrics"
	TypeOutput    Type = "output"
	TypeProcessor Type = "processor"
	TypeRateLimit Type = "rate_limit"
	TypeTracer    Type = "tracer"
	TypeScanner   Type = "scanner"
)

// Types returns a slice containing all component types.
func Types() []Type {
	return []Type{
		TypeBuffer,
		TypeCache,
		TypeInput,
		TypeMetrics,
		TypeOutput,
		TypeProcessor,
		TypeRateLimit,
		TypeTracer,
		TypeScanner,
	}
}

// ComponentSpec describes a Benthos component.
type ComponentSpec struct {
	// Name of the component
	Name string `json:"name"`

	// Type of the component (input, output, etc)
	Type Type `json:"type"`

	// The status of the component.
	Status Status `json:"status"`

	// The support level of the component. This is an abstract concept.
	SupportLevel string `json:"support_level,omitempty"`

	// Plugin is true for all plugin components.
	Plugin bool `json:"plugin"`

	// Summary of the component (in Asciidoc, must be short).
	Summary string `json:"summary,omitempty"`

	// Description of the component (in Asciidoc).
	Description string `json:"description,omitempty"`

	// Categories that describe the purpose of the component.
	Categories []string `json:"categories"`

	// Footnotes of the component (in Asciidoc).
	Footnotes string `json:"footnotes,omitempty"`

	// Examples demonstrating use cases for the component.
	Examples []AnnotatedExample `json:"examples,omitempty"`

	// A summary of each field in the component configuration.
	Config FieldSpec `json:"config"`

	// Version is the Benthos version this component was introduced.
	Version string `json:"version,omitempty"`
}
