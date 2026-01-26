// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"errors"

	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// ConfigQuerier provides utilities for parsing and then querying fields in a
// config, allowing you to analyse its structure.
type ConfigQuerier struct {
	env   *Environment
	res   *Resources
	spec  docs.FieldSpecs
	pConf *docs.ParsedConfig
}

// NewYAMLConfigQuerier creates a component for parsing and then querying
// configs, allowing you to analyse the structure of a given config.
func (s *ConfigSchema) NewYAMLConfigQuerier(yamlStr string) (*ConfigQuerier, error) {
	rootNode, err := docs.UnmarshalYAML([]byte(yamlStr))
	if err != nil {
		return nil, err
	}

	pConf, err := s.fields.ParsedConfigFromAny(rootNode)
	if err != nil {
		return nil, err
	}

	return &ConfigQuerier{
		env:   s.env,
		res:   MockResources(),
		spec:  s.fields,
		pConf: pConf,
	}, nil
}

// WithResources returns a copy of the querier with the specified resources.
// This ensures that nested fields accessing resources are able to correctly
// reference them.
func (q *ConfigQuerier) WithResources(res *Resources) *ConfigQuerier {
	return &ConfigQuerier{
		env:   q.env,
		res:   res,
		spec:  q.spec,
		pConf: q.pConf,
	}
}

// FieldAtPath extracts are parsed field from the config at the given path.
func (q *ConfigQuerier) FieldAtPath(path ...string) (*ParsedConfig, error) {
	fieldDocs, err := q.spec.GetDocsForPath(q.env.internal, path...)
	if err != nil {
		return nil, err
	}

	a, exists := q.pConf.Field(path...)
	if !exists {
		return nil, errors.New("field not found in data")
	}

	pConf, err := fieldDocs.ParsedConfigFromAny(a)
	if err != nil {
		return nil, err
	}

	return &ParsedConfig{
		mgr: q.res.mgr,
		i:   pConf,
	}, nil
}
