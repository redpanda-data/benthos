// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Component describes a matched plugin instance. It is passed to a
// Rule as a read-only view; mutation happens through the Result
// returned by the rule (Context.Replace).
type Component struct {
	// Type is the core component family, e.g. "processor".
	Type string
	// Name is the plugin name within Type, e.g. "bloblang".
	Name string
	// Path is the dotted location of this component within the config
	// (e.g. "pipeline.processors.0").
	Path string
	// Label is the YAML `label` value of the component, or "" if absent.
	Label string
	// LineStart and LineEnd are the 1-indexed line span of the
	// component container in the source YAML.
	LineStart, LineEnd int

	// container is the YAML mapping node holding the plugin's
	// `name: body` pair (and optional sibling fields like `label`).
	// Owned by the migrator; rules MUST NOT mutate it directly.
	container *yaml.Node
}

// BodyString returns the plugin's body as a string when the body is a
// YAML scalar (e.g. the `bloblang`, `mapping`, `mutation` and
// `bloblang_v2` processors all take a scalar string). Returns ok=false
// if the body is structured.
func (c *Component) BodyString() (string, bool) {
	v := c.bodyNode()
	if v == nil || v.Kind != yaml.ScalarNode {
		return "", false
	}
	return v.Value, true
}

// BodyAny decodes the plugin's body into an arbitrary Go value.
func (c *Component) BodyAny() (any, error) {
	v := c.bodyNode()
	if v == nil {
		return nil, fmt.Errorf("plugin %q has no body", c.Name)
	}
	var out any
	if err := v.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// bodyNode returns the YAML value node for the plugin's body, or nil
// if not present. The component container is a mapping with key/value
// pairs; we look for the entry whose key matches Name.
func (c *Component) bodyNode() *yaml.Node {
	if c.container == nil {
		return nil
	}
	for i := 0; i+1 < len(c.container.Content); i += 2 {
		if c.container.Content[i].Value == c.Name {
			return c.container.Content[i+1]
		}
	}
	return nil
}
