// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
	"github.com/redpanda-data/benthos/v4/public/schema"
)

func init() {
	bloblang.MustRegisterMethodV2("infer_schema",
		bloblang.NewPluginSpec().
			Category(query.MethodCategoryParsing).
			Description("Attempt to infer the schema of a given value. The resulting schema can then be used as an input to schema conversion and enforcement methods."),
		func(args *bloblang.ParsedParams) (bloblang.Method, error) {
			return func(v any) (any, error) {
				s, err := schema.InferFromAny(v)
				if err != nil {
					return nil, err
				}
				return s.ToAny(), nil
			}, nil
		})
}
