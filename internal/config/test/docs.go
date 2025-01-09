// Copyright 2025 Redpanda Data, Inc.

package test

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/internal/docs"
)

const fieldTests = "tests"

// ConfigSpec returns a configuration spec for a template.
func ConfigSpec() docs.FieldSpec {
	return docs.FieldObject(fieldTests, "A list of one or more unit tests to execute.").Array().WithChildren(caseFields()...).Optional()
}

// FromAny returns a Case slice from a yaml node or parsed config.
func FromAny(v any) ([]Case, error) {
	if t, ok := v.(*yaml.Node); ok {
		var tmp struct {
			Tests []yaml.Node
		}
		if err := t.Decode(&tmp); err != nil {
			return nil, err
		}
		var cases []Case
		for i, v := range tmp.Tests {
			pConf, err := caseFields().ParsedConfigFromAny(&v)
			if err != nil {
				return nil, fmt.Errorf("%v: %w", i, err)
			}
			c, err := CaseFromParsed(pConf)
			if err != nil {
				return nil, fmt.Errorf("%v: %w", i, err)
			}
			cases = append(cases, c)
		}
		return cases, nil
	}

	pConf, err := ConfigSpec().ParsedConfigFromAny(v)
	if err != nil {
		return nil, err
	}
	return FromParsed(pConf)
}

// FromParsed extracts a Case slice from a parsed config.
func FromParsed(pConf *docs.ParsedConfig) ([]Case, error) {
	if !pConf.Contains(fieldTests) {
		return nil, nil
	}

	oList, err := pConf.FieldObjectList(fieldTests)
	if err != nil {
		return nil, err
	}

	var cases []Case
	for i, pc := range oList {
		c, err := CaseFromParsed(pc)
		if err != nil {
			return nil, fmt.Errorf("%v: %w", i, err)
		}
		cases = append(cases, c)
	}
	return cases, nil
}
