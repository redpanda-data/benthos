package service

import (
	"fmt"
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// NewInterpolatedURLField defines a new config field that describes a
// dynamic URL that supports Bloblang interpolation functions. It is then
// possible to extract an *FieldInterpolatedURL from the resulting parsed config
// with the method FieldInterpolatedURL.
func NewInterpolatedURLField(name string) *ConfigField {
	tf := docs.FieldURL(name, "").IsInterpolated()
	return &ConfigField{field: tf}
}

// FieldInterpolatedURL accesses a field from a parsed config that was
// defined with NewInterpolatedURLField and returns either an
// *InterpolatedURL or an error if the string was invalid.
func (p *ParsedConfig) FieldInterpolatedURL(path ...string) (*InterpolatedURL, error) {
	v, exists := p.i.Field(path...)
	if !exists {
		return nil, fmt.Errorf("field '%v' was not found in the config", strings.Join(path, "."))
	}

	str, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("expected field '%v' to be a string, got %T", strings.Join(path, "."), v)
	}

	e, err := p.mgr.BloblEnvironment().NewField(str)
	if err != nil {
		return nil, fmt.Errorf("failed to parse interpolated field '%v': %v", strings.Join(path, "."), err)
	}

	return &InterpolatedURL{expr: e}, nil
}
