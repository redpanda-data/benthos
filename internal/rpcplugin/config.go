// Copyright 2025 Redpanda Data, Inc.

package rpcplugin

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/internal/filepath/ifs"
	"github.com/redpanda-data/benthos/v4/public/service"
)

const (
	pFName             = "name"
	pFPath             = "path"
	pFType             = "type"
	pFFields           = "fields"
	pFFieldName        = "name"
	pFFieldDescription = "description"
	pFFieldType        = "type"
	pFFieldKind        = "kind"
	pFFieldDefault     = "default"
	pFFieldAdvanced    = "advanced"
)

// Order of events:
//
// Start up:
// foo.rpcplugin.yaml detected
// foo.rpcplugin.yaml parsed as a rpc plugin config, validated, and linted
// component foo is registered as a plugin
//
// When instanciation of foo is made:
// directory foo is searched for plugin code
// plugin code is run based on the language (only binaries ran as-is initially)
// plugin hosts rpc API
// benthos calls init on the API with config fields, an instantiation ID is returned by plugin
// all plugin methods are called as API calls with params that match a regular plugin of the component type, plus the instantiation ID
// during shutdown a close API method is called, followed by the shutdown of the process

// The instantiation ID will potentially allow for future plugins where each instantiation exists within the same process, this will help with resource heavy languages (*cough*, java)

// FieldConfig describes a configuration field used in the template.
type FieldConfig struct {
	Name     string  `yaml:"name"`
	Type     *string `yaml:"type,omitempty"`
	Kind     *string `yaml:"kind,omitempty"`
	Default  *any    `yaml:"default,omitempty"`
	Advanced bool    `yaml:"advanced"`
}

// Config describes a Benthos component template.
type Config struct {
	Name   string        `yaml:"name"`
	Path   string        `yaml:"path"`
	Type   string        `yaml:"type"`
	Fields []FieldConfig `yaml:"fields"`
}

// FieldSpec creates a documentation field spec from a template field config.
func (c FieldConfig) FieldSpec() (docs.FieldSpec, error) {
	f := docs.FieldAnything(c.Name, "")
	f.IsAdvanced = c.Advanced
	if c.Default != nil {
		f = f.HasDefault(*c.Default)
	}
	if c.Type == nil {
		return f, errors.New("missing type field")
	}
	f = f.HasType(docs.FieldType(*c.Type))
	if c.Kind != nil {
		switch *c.Kind {
		case "map":
			f = f.Map()
		case "list":
			f = f.Array()
		case "scalar":
		default:
			return f, fmt.Errorf("unrecognised scalar type: %v", *c.Kind)
		}
	}
	if f.Type == "bloblang" {
		f = f.HasType(docs.FieldTypeString).IsBloblang()
	}
	return f, nil
}

// ComponentSpec creates a documentation component spec from a template config.
func (c Config) ComponentSpec() (docs.ComponentSpec, error) {
	fields := make([]docs.FieldSpec, len(c.Fields))
	for i, fieldConf := range c.Fields {
		var err error
		if fields[i], err = fieldConf.FieldSpec(); err != nil {
			return docs.ComponentSpec{}, fmt.Errorf("field %v: %w", i, err)
		}
	}
	config := docs.FieldComponent().WithChildren(fields...)

	return docs.ComponentSpec{
		Name:   c.Name,
		Type:   docs.Type(c.Type),
		Status: docs.StatusStable,
		Plugin: true,
		Config: config,
	}, nil
}

// ReadConfigYAML attempts to read a YAML byte slice as a template configuration
// file.
func ReadConfigYAML(env *bundle.Environment, pluginSpecBytes []byte) (conf Config, lints []docs.Lint, err error) {
	if err = yaml.Unmarshal(pluginSpecBytes, &conf); err != nil {
		return
	}

	var node yaml.Node
	if err = yaml.Unmarshal(pluginSpecBytes, &node); err != nil {
		return
	}

	lints = ConfigSpec().LintYAML(docs.NewLintContext(docs.NewLintConfig(env)), &node)
	return
}

// ReadConfigFile attempts to read a template configuration file.
func ReadConfigFile(env *bundle.Environment, path string) (conf Config, lints []docs.Lint, err error) {
	var templateBytes []byte
	if templateBytes, err = ifs.ReadFile(ifs.OS(), path); err != nil {
		return
	}
	return ReadConfigYAML(env, templateBytes)
}

//------------------------------------------------------------------------------

// ConfigFields returns a slice of configuration fields for a plugin manifest.
func ConfigFields() []*service.ConfigField {
	return []*service.ConfigField{
		service.NewStringField(pfName).
			Description("The name the plugin will be known as."),
		service.NewStringField(pfPath).
			Description("The path of the plugin implementation."),
		service.NewStringEnumField(pfType, "cache", "input", "output", "processor", "rate_limit").
			Description("The type of the component this template will create."),
		service.NewObjectListField(pfFields,
			service.NewStringField(pfFieldName).
				Description("The name of the field."),
			service.NewStringField(pfFieldDescription).
				Description("A description of the field.").
				HasDefault(""),
			service.NewStringAnnotatedEnumField(pfFieldType, map[string]string{
				"string":   "standard string type",
				"int":      "standard integer type",
				"float":    "standard float type",
				"bool":     "a boolean true/false",
				"bloblang": "a bloblang mapping",
				"unknown":  "allows for nesting arbitrary configuration inside of a field",
			}).Description("The scalar type of the field."),
			service.NewStringEnumField(pfFieldKind, "scalar", "map", "list").
				Description("The kind of the field.").
				HasDefault("scalar"),
			service.NewAnyField(pfFieldDefault).
				Description("An optional default value for the field. If a default value is not specified then a configuration without the field is considered incorrect.").
				Optional(),
			service.NewBoolField(pfFieldAdvanced).
				Description("Whether this field is considered advanced.").
				HasDefault(false),
		).
			Description("The configuration fields of the template, fields specified here will be parsed from a Redpanda Connect config and will be accessible from the template mapping."),
	}
}
