// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"context"
	"errors"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// ConfigQuerier provides utilities for parsing and then querying fields in a
// config, allowing you to analyse its structure.
type ConfigQuerier struct {
	env  *Environment
	res  *Resources
	spec docs.FieldSpecs

	envVarLookupFn func(context.Context, string) (string, bool)
}

// NewConfigQuerier creates a utility for parsing and then querying configs,
// allowing you to analyse the structure of a given config.
func (s *ConfigSchema) NewConfigQuerier() *ConfigQuerier {
	return &ConfigQuerier{
		env:  s.env,
		res:  MockResources(),
		spec: s.fields,
		envVarLookupFn: func(_ context.Context, k string) (string, bool) {
			return os.LookupEnv(k)
		},
	}
}

// SetResources sets the resources to be referenced by parsed configs, this
// ensures that nested fields accessing resources are able to correctly
// reference them.
func (q *ConfigQuerier) SetResources(res *Resources) {
	q.res = res
}

// SetEnvVarLookupFunc changes the behaviour of the config querier so that
// the value of environment variable interpolations (of the form `${FOO}`) are
// obtained via a provided function rather than the default of os.LookupEnv.
func (q *ConfigQuerier) SetEnvVarLookupFunc(fn func(context.Context, string) (string, bool)) {
	q.envVarLookupFn = fn
}

// ParseYAML parses a YAML config string and returns a ConfigQueryFile that can
// be used to query fields within the parsed config.
func (q *ConfigQuerier) ParseYAML(confStr string) (*ConfigQueryFile, error) {
	rootNode, err := q.getYAMLNode([]byte(confStr))
	if err != nil {
		return nil, err
	}

	pConf, err := q.spec.ParsedConfigFromAny(rootNode)
	if err != nil {
		return nil, err
	}

	return &ConfigQueryFile{
		res:        q.res,
		spec:       q.spec,
		parsedConf: pConf,
	}, nil
}

func (q *ConfigQuerier) getYAMLNode(b []byte) (*yaml.Node, error) {
	var err error
	if b, err = config.NewReader("", nil, config.OptUseEnvLookupFunc(q.envVarLookupFn)).ReplaceEnvVariables(context.TODO(), b); err != nil {
		// TODO: Allow users to specify whether they care about env variables
		// missing, in which case we error or not based on that.
		var errEnvMissing *config.ErrMissingEnvVars
		if errors.As(err, &errEnvMissing) {
			b = errEnvMissing.BestAttempt
		} else {
			return nil, err
		}
	}
	return docs.UnmarshalYAML(b)
}

//------------------------------------------------------------------------------

// ConfigQueryFile represents a parsed config file that can be queried for
// specific fields at given paths.
type ConfigQueryFile struct {
	res        *Resources
	spec       docs.FieldSpecs
	parsedConf *docs.ParsedConfig
}

// FieldAtPath extracts a parsed field from the config at the given path.
func (f *ConfigQueryFile) FieldAtPath(path ...string) (*ParsedConfig, error) {
	fieldDocs, err := f.spec.GetDocsForPath(f.res.mgr.Environment(), path...)
	if err != nil {
		return nil, err
	}

	a, exists := f.parsedConf.Field(path...)
	if !exists {
		return nil, errors.New("field not found in data")
	}

	pConf, err := fieldDocs.ParsedConfigFromAny(a)
	if err != nil {
		return nil, err
	}

	return &ParsedConfig{
		mgr: f.res.mgr,
		i:   pConf,
	}, nil
}
