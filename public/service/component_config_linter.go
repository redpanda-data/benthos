// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// ComponentConfigLinter provides utilities for linting individual component
// configs.
type ComponentConfigLinter struct {
	env            *Environment
	lintConf       docs.LintConfig
	skipEnvVarLint bool
	envVarLookupFn func(string) (string, bool)
}

// NewComponentConfigLinter creates a component linter.
func (e *Environment) NewComponentConfigLinter() *ComponentConfigLinter {
	lintConf := docs.NewLintConfig(e.internal)
	lintConf.BloblangEnv = e.bloblangEnv.Deactivated()
	return &ComponentConfigLinter{
		env:            e,
		lintConf:       lintConf,
		envVarLookupFn: os.LookupEnv,
	}
}

// SetRejectDeprecated sets whether deprecated fields should trigger linting
// errors.
func (c *ComponentConfigLinter) SetRejectDeprecated(v bool) *ComponentConfigLinter {
	c.lintConf.RejectDeprecated = v
	return c
}

// SetRequireLabels sets whether labels must be present for all components that
// support them.
func (c *ComponentConfigLinter) SetRequireLabels(v bool) *ComponentConfigLinter {
	c.lintConf.RequireLabels = v
	return c
}

// SetSkipEnvVarCheck sets whether the linter should ignore cases where
// environment variables are referenced and do not exist.
func (c *ComponentConfigLinter) SetSkipEnvVarCheck(v bool) *ComponentConfigLinter {
	c.skipEnvVarLint = v
	return c
}

// SetEnvVarLookupFunc overrides the default environment variable lookup so that
// interpolations within a config are resolved by the provided closure function.
func (c *ComponentConfigLinter) SetEnvVarLookupFunc(fn func(context.Context, string) (string, bool)) *ComponentConfigLinter {
	c.envVarLookupFn = func(s string) (string, bool) {
		return fn(context.Background(), s)
	}
	return c
}

func (c *ComponentConfigLinter) readYAML(yamlBytes []byte) (cNode *yaml.Node, lints []Lint, err error) {
	if !utf8.Valid(yamlBytes) {
		lints = append(lints, Lint{
			Line: 1,
			Type: LintFailedRead,
			What: "detected invalid utf-8 encoding in config, this may result in interpolation functions not working as expected",
		})
	}

	if yamlBytes, err = config.NewReader("", nil, config.OptUseEnvLookupFunc(func(ctx context.Context, key string) (string, bool) {
		return c.envVarLookupFn(key)
	})).ReplaceEnvVariables(context.TODO(), yamlBytes); err != nil {
		var errEnvMissing *config.ErrMissingEnvVars
		if !errors.As(err, &errEnvMissing) {
			return
		}
		yamlBytes = errEnvMissing.BestAttempt
		if !c.skipEnvVarLint {
			lints = append(lints, Lint{Line: 1, Type: LintMissingEnvVar, What: err.Error()})
		}
	}

	if bytes.HasPrefix(yamlBytes, []byte("# BENTHOS LINT DISABLE")) {
		return
	}

	cNode, err = docs.UnmarshalYAML(yamlBytes)
	return
}

// LintYAML attempts to parse a component config in YAML format and, if
// successful, returns a slice of linting errors, or an error is the parsing
// failed.
func (c *ComponentConfigLinter) LintYAML(componentType string, yamlBytes []byte) (lints []Lint, err error) {
	var cNode *yaml.Node
	if cNode, lints, err = c.readYAML(yamlBytes); err != nil {
		return
	}

	for _, l := range docs.LintYAML(docs.NewLintContext(c.lintConf), docs.Type(componentType), cNode) {
		lints = append(lints, Lint{
			Column: l.Column,
			Line:   l.Line,
			Type:   convertDocsLintType(l.Type),
			What:   l.What,
		})
	}
	return
}
