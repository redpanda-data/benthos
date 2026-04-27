// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"fmt"

	"github.com/Jeffail/gabs/v2"

	"github.com/redpanda-data/benthos/v4/internal/value"
	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// V2 ports of V1 object methods that don't require lambda arguments.

func init() {
	bloblangv2.MustRegisterMethod("array",
		bloblangv2.NewPluginSpec().
			Category("Coercion").
			Description("Returns the receiver wrapped in a single-element array, unless it is already an array, in which case it is returned unchanged."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return func(v any) (any, error) {
				if _, ok := v.([]any); ok {
					return v, nil
				}
				return []any{v}, nil
			}, nil
		},
	)

	bloblangv2.MustRegisterMethod("exists",
		bloblangv2.NewPluginSpec().
			Category("Object & Array").
			Description("Returns true when the dot-path argument resolves to a field that is present on the receiver object — even when its value is null. Returns false otherwise.").
			Param(bloblangv2.NewStringParam("path").Description("A dot-separated path to the field.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			pathStr, err := args.GetString("path")
			if err != nil {
				return nil, err
			}
			path := gabs.DotPathToSlice(pathStr)
			return func(v any) (any, error) {
				return gabs.Wrap(v).Exists(path...), nil
			}, nil
		},
	)

	bloblangv2.MustRegisterMethod("get",
		bloblangv2.NewPluginSpec().
			Category("Object & Array").
			Description("Extracts a field value from the receiver object identified by a dot-path. Returns null when the path does not resolve.").
			Param(bloblangv2.NewStringParam("path").Description("A dot-separated path to the field.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			pathStr, err := args.GetString("path")
			if err != nil {
				return nil, err
			}
			path := gabs.DotPathToSlice(pathStr)
			return func(v any) (any, error) {
				return gabs.Wrap(v).S(path...).Data(), nil
			}, nil
		},
	)

	bloblangv2.MustRegisterMethod("explode",
		bloblangv2.NewPluginSpec().
			Category("Object & Array").
			Description(`Expands a nested array or object at the given path into multiple documents while preserving the surrounding structure. With an array target the result is an array of documents; with an object target the result is an object keyed by the nested keys.`).
			Param(bloblangv2.NewStringParam("path").Description("A dot-separated path to the field to explode.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			pathRaw, err := args.GetString("path")
			if err != nil {
				return nil, err
			}
			path := gabs.DotPathToSlice(pathRaw)
			return func(v any) (any, error) {
				rootMap, ok := v.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("expected object receiver, got %T", v)
				}

				target := gabs.Wrap(v).Search(path...)
				copyFrom := mapCloneWithoutPath(rootMap, path)

				switch t := target.Data().(type) {
				case []any:
					result := make([]any, len(t))
					for i, ele := range t {
						g := gabs.Wrap(value.IClone(copyFrom))
						_, _ = g.Set(ele, path...)
						result[i] = g.Data()
					}
					return result, nil
				case map[string]any:
					result := make(map[string]any, len(t))
					for k, ele := range t {
						g := gabs.Wrap(value.IClone(copyFrom))
						_, _ = g.Set(ele, path...)
						result[k] = g.Data()
					}
					return result, nil
				}
				return nil, fmt.Errorf("expected array or object value at path %q, found: %T", pathRaw, target.Data())
			}, nil
		},
	)

	bloblangv2.MustRegisterMethod("assign",
		bloblangv2.NewPluginSpec().
			Category("Object & Array").
			Description("Combines the receiver with the with argument. For objects, source keys overwrite destination keys on conflict. For arrays the with value is concatenated. Use merge() instead for non-overwriting behaviour.").
			Param(bloblangv2.NewAnyParam("with").Description("Object or array to assign onto the receiver.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			source, err := args.Get("with")
			if err != nil {
				return nil, err
			}
			return func(v any) (any, error) {
				source := value.IClone(source)
				if root, isArray := v.([]any); isArray {
					if rhs, isAlsoArray := source.([]any); isAlsoArray {
						return append(root, rhs...), nil
					}
					return append(root, source), nil
				}
				if _, isObject := v.(map[string]any); !isObject {
					return nil, fmt.Errorf("expected object or array receiver, got %T", v)
				}
				root := gabs.New()
				if err := root.MergeFn(gabs.Wrap(v), assignerOverwrite); err != nil {
					return nil, err
				}
				if err := root.MergeFn(gabs.Wrap(source), assignerOverwrite); err != nil {
					return nil, err
				}
				return root.Data(), nil
			}, nil
		},
	)
}

func assignerOverwrite(_, source any) any { return source }

// mapCloneWithoutPath returns a deep clone of m with the value at path
// removed. Used by explode to derive the carrier document for each exploded
// child.
func mapCloneWithoutPath(m map[string]any, path []string) any {
	if len(path) == 0 {
		return value.IClone(m)
	}
	cloned, ok := value.IClone(m).(map[string]any)
	if !ok {
		return value.IClone(m)
	}
	cur := cloned
	for i := 0; i < len(path)-1; i++ {
		next, ok := cur[path[i]].(map[string]any)
		if !ok {
			return cloned
		}
		cur = next
	}
	delete(cur, path[len(path)-1])
	return cloned
}
