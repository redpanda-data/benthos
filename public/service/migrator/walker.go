// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// walk parses the input YAML, traverses every plugin instance in the
// stream config, applies any matching rule, and returns the rewritten
// document plus the per-component changes. The input bytes are not
// mutated; the returned tree is a fresh allocation.
func walk(yamlBytes []byte, rules map[Target]Rule, ctx *Context, verbose bool) (string, []Change, error) {
	root, err := docs.UnmarshalYAML(yamlBytes)
	if err != nil {
		return "", nil, fmt.Errorf("parse config: %w", err)
	}

	spec := config.Spec()
	provider := bundle.GlobalEnvironment

	var changes []Change

	walkConf := docs.WalkComponentConfig{
		Provider: provider,
		Func: func(wc docs.WalkedComponent) error {
			coreType, ok := wc.Field.Type.IsCoreComponent()
			if !ok {
				return nil
			}
			target := Target{
				ComponentType: string(coreType),
				Name:          wc.Name,
			}
			rule, found := rules[target]
			if !found {
				return nil
			}

			container, ok := wc.Value.(*yaml.Node)
			if !ok {
				return nil
			}

			comp := &Component{
				Type:      target.ComponentType,
				Name:      target.Name,
				Path:      wc.Path,
				Label:     wc.Label,
				LineStart: wc.LineStart,
				LineEnd:   wc.LineEnd,
				container: container,
			}
			res := rule(ctx, comp)
			ch := buildChange(target, comp, res, verbose)
			if ch != nil {
				changes = append(changes, *ch)
			}
			if res.kind == resultReplace {
				if err := applyReplacement(container, target.Name, res.replacement); err != nil {
					return fmt.Errorf("%s/%s: %w", target.ComponentType, target.Name, err)
				}
			}
			return nil
		},
	}

	if err := spec.WalkComponentsYAML(walkConf, root); err != nil {
		return "", nil, err
	}

	out, err := docs.MarshalYAML(*root)
	if err != nil {
		return "", nil, fmt.Errorf("marshal config: %w", err)
	}
	return string(out), changes, nil
}

// applyReplacement mutates the container mapping node in place,
// renaming the plugin's key and replacing its value with the supplied
// body. The body may be a string (assigned to the existing scalar
// node, preserving its style) or an arbitrary Go value (encoded into
// a fresh yaml.Node).
func applyReplacement(container *yaml.Node, oldName string, r replacement) error {
	if container == nil || container.Kind != yaml.MappingNode {
		return errors.New("container is not a mapping node")
	}
	for i := 0; i+1 < len(container.Content); i += 2 {
		if container.Content[i].Value != oldName {
			continue
		}
		container.Content[i].Value = r.name
		valueNode := container.Content[i+1]
		switch body := r.body.(type) {
		case string:
			if valueNode.Kind != yaml.ScalarNode {
				newScalar := &yaml.Node{Kind: yaml.ScalarNode, Value: body}
				preserveScalarStyle(newScalar, body)
				container.Content[i+1] = newScalar
				return nil
			}
			valueNode.Value = body
			valueNode.Tag = ""
			preserveScalarStyle(valueNode, body)
			return nil
		default:
			var encoded yaml.Node
			if err := encoded.Encode(body); err != nil {
				return fmt.Errorf("encode replacement body: %w", err)
			}
			container.Content[i+1] = &encoded
			return nil
		}
	}
	return fmt.Errorf("plugin %q not found in container", oldName)
}

// preserveScalarStyle picks a sensible scalar style for a string body.
// Multi-line bodies render best as literal-block scalars (`|`), single
// lines fall back to whatever yaml.v3 chooses (usually plain or
// double-quoted depending on content).
func preserveScalarStyle(n *yaml.Node, body string) {
	for _, r := range body {
		if r == '\n' {
			n.Style = yaml.LiteralStyle
			return
		}
	}
	n.Style = 0
}

// buildChange materialises the Change record for a rule outcome.
// Returns nil for Skip results when verbose is false, since silent
// skips are noise in non-verbose reports.
func buildChange(target Target, c *Component, res Result, verbose bool) *Change {
	ch := &Change{
		Target:    target,
		Path:      c.Path,
		Label:     c.Label,
		LineStart: c.LineStart,
		LineEnd:   c.LineEnd,
	}
	switch res.kind {
	case resultReplace:
		ch.Outcome = OutcomeRewritten
		ch.Severity = SeverityInfo
		ch.NewName = res.replacement.name
		ch.Reason = fmt.Sprintf("rewrote %s/%s -> %s", target.ComponentType, target.Name, res.replacement.name)
		ch.BloblangReport = res.replacement.bloblangReport
		return ch
	case resultUnsupported:
		ch.Outcome = OutcomeUnsupported
		ch.Severity = SeverityError
		ch.Reason = res.reason
		return ch
	case resultSkip:
		if !verbose && res.reason == "" {
			return nil
		}
		ch.Outcome = OutcomeSkipped
		ch.Severity = SeverityInfo
		ch.Reason = res.reason
		if !verbose {
			return nil
		}
		return ch
	}
	return nil
}
