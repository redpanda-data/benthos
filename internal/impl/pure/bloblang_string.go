// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"fmt"
	"net/url"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

func init() {
	bloblang.MustRegisterMethodV2("parse_form_url_encoded",
		bloblang.NewPluginSpec().
			Category(query.MethodCategoryParsing).
			Description(`Attempts to parse a url-encoded query string (from an x-www-form-urlencoded request body) and returns a structured result.`).
			Example("", `root.values = this.body.parse_form_url_encoded()`,
				[2]string{
					`{"body":"noise=meow&animal=cat&fur=orange&fur=fluffy"}`,
					`{"values":{"animal":"cat","fur":["orange","fluffy"],"noise":"meow"}}`,
				},
			),
		func(args *bloblang.ParsedParams) (bloblang.Method, error) {
			return bloblang.StringMethod(func(data string) (any, error) {
				values, err := url.ParseQuery(data)
				if err != nil {
					return nil, fmt.Errorf("failed to parse value as url-encoded data: %w", err)
				}
				return urlValuesToMap(values), nil
			}), nil
		})
}

func urlValuesToMap(values url.Values) map[string]any {
	root := make(map[string]any, len(values))

	for k, v := range values {
		if len(v) == 1 {
			root[k] = v[0]
		} else {
			elements := make([]any, 0, len(v))
			for _, e := range v {
				elements = append(elements, e)
			}
			root[k] = elements
		}
	}

	return root
}
