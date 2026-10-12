package service

import (
	"net/url"

	"github.com/redpanda-data/benthos/v4/internal/bloblang"
	"github.com/redpanda-data/benthos/v4/internal/bloblang/field"
)

// InterpolatedURL resolves a URL containing dynamic interpolation
// functions for a given message.
type InterpolatedURL struct {
	expr *field.Expression
}

// NewInterpolatedURL parses an interpolated URL expression.
func NewInterpolatedURL(expr string) (*InterpolatedURL, error) {
	e, err := bloblang.GlobalEnvironment().NewField(expr)
	if err != nil {
		return nil, err
	}
	return &InterpolatedURL{expr: e}, nil
}

// Static returns the underlying contents of the interpolated URL only if it
// contains zero dynamic expressions, and is therefore static, otherwise an
// empty string is returned. A second boolean parameter is also returned
// indicating whether the URL was static, helping to distinguish between a
// static empty URL versus a non-static URL.
func (i *InterpolatedURL) Static() (*url.URL, bool) {
	if i.expr.NumDynamicExpressions() > 0 {
		return nil, false
	}
	s, _ := i.expr.String(0, nil)

	u, err := url.Parse(s)
	if err != nil {
		return nil, false
	}

	return u, true
}

// TryURL resolves the interpolated field for a given message as a URL,
// returns an error if any interpolation functions fail.
func (i *InterpolatedURL) TryURL(m *Message) (*url.URL, error) {
	s, err := i.expr.String(0, fauxOldMessage{m.part})
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	return u, nil
}
