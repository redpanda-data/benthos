// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"text/template"

	"github.com/Jeffail/gabs/v2"
	"gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// TemplateDataPlugin contains information ready to inject within a
// documentation page.
type TemplateDataPlugin struct {
	// The name of the plugin.
	Name string

	// The component type of the plugin.
	Type string

	// A summary of the components purpose.
	Summary string

	// A longer form description of the plugin.
	Description string

	// A general category of the plugin.
	Categories string

	// A list of examples of this plugin in action.
	Examples []TemplatDataPluginExample

	// A list of fields defined by the plugin.
	Fields []TemplateDataPluginField

	// Documentation that should be placed at the bottom of a page.
	Footnotes string

	// An example YAML config containing only common fields.
	CommonConfigYAML string

	// An example YAML config containing all fields.
	AdvancedConfigYAML string

	// A general stability status of the plugin.
	Status string

	// An abstract concept of support level.
	SupportLevel string

	// The version in which this plugin was added.
	Version string
}

// TemplatDataPluginExample contains a plugin example ready to inject into
// documentation.
type TemplatDataPluginExample struct {
	// A title for the example.
	Title string

	// Summary of the example.
	Summary string

	// A config snippet to show.
	Config string
}

// TemplateDataPluginField provides information about a field for injecting into
// documentation templates.
type TemplateDataPluginField struct {
	// The description of the field.
	Description string

	// Whether the field contains secrets.
	IsSecret bool

	// Whether the field is interpolated.
	IsInterpolated bool

	// The type information of the field.
	Type string

	// The version in which this field was added.
	Version string

	// An array of enum options accompanied by a description.
	AnnotatedOptions [][2]string

	// An array of enum options, without annotations.
	Options []string

	// An array of example values.
	Examples []any

	// FullName describes the full dot path name of the field relative to
	// the root of the documented component.
	FullName string

	// ExamplesMarshalled is a list of examples marshalled into YAML format.
	ExamplesMarshalled []string

	// DefaultMarshalled is a marshalled string of the default value in JSON
	// format, if there is one.
	DefaultMarshalled string
}

// TemplateData returns a struct containing useful documentation details, which
// can then be injected into a template in order to populate a
// documentation website automatically.
func (c *ConfigView) TemplateData() (TemplateDataPlugin, error) {
	_, rootOnly := map[string]struct{}{
		"cache":      {},
		"rate_limit": {},
		"processor":  {},
		"scanner":    {},
	}[string(c.component.Type)]

	conf := map[string]any{
		"type": c.component.Name,
	}
	for k, v := range docs.ReservedFieldsByType(c.component.Type) {
		if k == "plugin" {
			continue
		}
		if v.Default != nil {
			conf[k] = *v.Default
		}
	}

	return prepareComponentSpecForTemplate(c.prov, &c.component, !rootOnly, conf)
}

//------------------------------------------------------------------------------

func createOrderedConfig(prov docs.Provider, t docs.Type, rawExample any, filter docs.FieldFilter) (*yaml.Node, error) {
	var newNode yaml.Node
	if err := newNode.Encode(rawExample); err != nil {
		return nil, err
	}

	sanitConf := docs.NewSanitiseConfig(prov)
	sanitConf.RemoveTypeField = true
	sanitConf.Filter = filter
	sanitConf.ForExample = true
	if err := docs.SanitiseYAML(t, &newNode, sanitConf); err != nil {
		return nil, err
	}

	return &newNode, nil
}

func genExampleConfigs(prov docs.Provider, t docs.Type, nest bool, fullConfigExample any) (commonConfigStr, advConfigStr string, err error) {
	var advConfig, commonConfig any
	if advConfig, err = createOrderedConfig(prov, t, fullConfigExample, func(f docs.FieldSpec, _ any) bool {
		return !f.IsDeprecated
	}); err != nil {
		panic(err)
	}
	if commonConfig, err = createOrderedConfig(prov, t, fullConfigExample, func(f docs.FieldSpec, _ any) bool {
		return !f.IsAdvanced && !f.IsDeprecated
	}); err != nil {
		panic(err)
	}

	if nest {
		advConfig = map[string]any{string(t): advConfig}
		commonConfig = map[string]any{string(t): commonConfig}
	}

	advancedConfigBytes, err := marshalYAML(advConfig)
	if err != nil {
		panic(err)
	}
	commonConfigBytes, err := marshalYAML(commonConfig)
	if err != nil {
		panic(err)
	}

	return string(commonConfigBytes), string(advancedConfigBytes), nil
}

func prepareComponentSpecForTemplate(prov docs.Provider, c *docs.ComponentSpec, nest bool, fullConfigExample any) (ctx TemplateDataPlugin, err error) {
	ctx.Name = c.Name
	ctx.Type = string(c.Type)
	ctx.Summary = c.Summary
	ctx.Description = c.Description
	ctx.Footnotes = c.Footnotes
	ctx.Status = string(c.Status)
	ctx.SupportLevel = c.SupportLevel
	ctx.Version = c.Version
	ctx.Fields = flattenFieldSpecForTemplate(c.Config)

	if ctx.Status == "" {
		ctx.Status = string(docs.StatusStable)
	}

	for _, e := range c.Examples {
		ctx.Examples = append(ctx.Examples, TemplatDataPluginExample(e))
	}

	if len(c.Categories) > 0 {
		cats, _ := json.Marshal(c.Categories)
		ctx.Categories = string(cats)
	}

	if ctx.CommonConfigYAML, ctx.AdvancedConfigYAML, err = genExampleConfigs(prov, c.Type, nest, fullConfigExample); err != nil {
		return
	}

	if c.Description != "" && c.Description[0] == '\n' {
		ctx.Description = c.Description[1:]
	}
	if c.Footnotes != "" && c.Footnotes[0] == '\n' {
		ctx.Footnotes = c.Footnotes[1:]
	}
	return
}

//------------------------------------------------------------------------------

func marshalYAML(v any) ([]byte, error) {
	var cbytes bytes.Buffer
	enc := yaml.NewEncoder(&cbytes)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return cbytes.Bytes(), nil
}

func flattenFieldSpecForTemplate(f docs.FieldSpec) (flattenedFields []TemplateDataPluginField) {
	var walkFields func(path string, f docs.FieldSpecs)
	walkFields = func(path string, f docs.FieldSpecs) {
		for _, v := range f {
			if v.IsDeprecated {
				continue
			}
			newV := TemplateDataPluginField{
				Description:      strings.TrimSpace(v.Description),
				IsSecret:         v.IsSecret,
				IsInterpolated:   v.Interpolated,
				Type:             string(v.Type),
				Version:          v.Version,
				AnnotatedOptions: v.AnnotatedOptions,
				Options:          v.Options,
				Examples:         v.Examples,
			}
			newV.FullName = v.Name
			if path != "" {
				newV.FullName = path + v.Name
			}
			if len(v.Examples) > 0 {
				newV.ExamplesMarshalled = make([]string, len(v.Examples))
				for i, e := range v.Examples {
					exampleBytes, err := marshalYAML(map[string]any{
						v.Name: e,
					})
					if err == nil {
						newV.ExamplesMarshalled[i] = string(exampleBytes)
					}
				}
			}
			if v.Default != nil {
				newV.DefaultMarshalled = gabs.Wrap(*v.Default).String()
			}
			if newV.Description == "" {
				newV.Description = "Sorry! This field is missing documentation."
			}

			// TODO: Enable the better descriptions later
			switch v.Kind {
			case docs.KindMap:
				// newV.Type = "object of strings to " + newV.Type
				newV.Type = "object"
			case docs.KindArray:
				// newV.Type = "array of " + newV.Type
				newV.Type = "array"
			case docs.Kind2DArray:
				// newV.Type = "two-dimensional array of " + newV.Type
				newV.Type = "two-dimensional array"
			}

			flattenedFields = append(flattenedFields, newV)
			if len(v.Children) > 0 {
				newPath := path + v.Name
				switch v.Kind {
				case docs.KindArray:
					newPath += "[]"
				case docs.Kind2DArray:
					newPath += "[][]"
				case docs.KindMap:
					newPath += ".<name>"
				}
				walkFields(newPath+".", v.Children)
			}
		}
	}
	rootPath := ""
	switch f.Kind {
	case docs.KindArray:
		rootPath = "[]."
	case docs.KindMap:
		rootPath = "<name>."
	}
	walkFields(rootPath, f.Children)
	return flattenedFields
}

//------------------------------------------------------------------------------

// RenderDocs creates a markdown file that documents the configuration of the
// component config view. This markdown may include Docusaurus react elements as
// it matches the documentation generated for the official Benthos website.
//
// Experimental: This method is not intended for general use and could have its
// signature and/or behaviour changed outside of major version bumps.
func (c *ConfigView) RenderDocs() ([]byte, error) {
	_, rootOnly := map[string]struct{}{
		"cache":      {},
		"rate_limit": {},
		"processor":  {},
		"scanner":    {},
	}[string(c.component.Type)]

	conf := map[string]any{
		"type": c.component.Name,
	}
	for k, v := range docs.ReservedFieldsByType(c.component.Type) {
		if k == "plugin" {
			continue
		}
		if v.Default != nil {
			conf[k] = *v.Default
		}
	}

	data, err := prepareComponentSpecForTemplate(c.prov, &c.component, !rootOnly, conf)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	err = template.Must(template.New("component").Parse(docs.DeprecatedComponentTemplate)).Execute(&buf, data)
	return buf.Bytes(), err
}
