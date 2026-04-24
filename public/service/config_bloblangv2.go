// Copyright 2026 Redpanda Data, Inc.

package service

import (
	"fmt"
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// NewBloblangV2Field defines a new config field that describes a Bloblang V2
// mapping string. A *bloblangv2.Executor can then be extracted from the parsed
// config via FieldBloblangV2.
//
// Bloblang V2 is a separate language from V1 with its own parser and plugin
// registry; see public/bloblangv2 for details.
func NewBloblangV2Field(name string) *ConfigField {
	tf := docs.FieldString(name, "")
	return &ConfigField{field: tf}
}

// FieldBloblangV2 accesses a field from a parsed config that was defined with
// NewBloblangV2Field and returns either a *bloblangv2.Executor or an error if
// the mapping was invalid.
func (p *ParsedConfig) FieldBloblangV2(path ...string) (*bloblangv2.Executor, error) {
	v, exists := p.i.Field(path...)
	if !exists {
		return nil, fmt.Errorf("field '%v' was not found in the config", strings.Join(path, "."))
	}

	str, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("expected field '%v' to be a string, got %T", strings.Join(path, "."), v)
	}

	exec, err := p.mgr.BloblV2Environment().Parse(str)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bloblang v2 mapping '%v': %v", strings.Join(path, "."), err)
	}
	return exec, nil
}
