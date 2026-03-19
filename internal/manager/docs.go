// Copyright 2025 Redpanda Data, Inc.

package manager

import (
	"errors"
	"fmt"

	"github.com/Jeffail/gabs/v2"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/docs"
)

func lintResource(ctx docs.LintContext, line, col int, v any) []docs.Lint {
	if _, ok := v.(map[string]any); !ok {
		return nil
	}
	gObj := gabs.Wrap(v)
	label, _ := gObj.S("label").Data().(string)
	if label == "" {
		return []docs.Lint{
			docs.NewLintError(line, docs.LintBadLabel, errors.New("the label field for resources must be unique and not empty")),
		}
	}
	return nil
}

// Spec returns a field spec for the manager configuration.
func Spec() docs.FieldSpecs {
	return docs.FieldSpecs{
		docs.FieldInput(
			"input_resources", "A list of input resources, each must have a unique label.",
		).Array().LinterFunc(lintResource).HasDefault([]any{}).Advanced(),

		docs.FieldProcessor(
			"processor_resources", "A list of processor resources, each must have a unique label.",
		).Array().LinterFunc(lintResource).HasDefault([]any{}).Advanced(),

		docs.FieldOutput(
			"output_resources", "A list of output resources, each must have a unique label.",
		).Array().LinterFunc(lintResource).HasDefault([]any{}).Advanced(),

		docs.FieldCache(
			"cache_resources", "A list of cache resources, each must have a unique label.",
		).Array().LinterFunc(lintResource).HasDefault([]any{}).Advanced(),

		docs.FieldRateLimit(
			"rate_limit_resources", "A list of rate limit resources, each must have a unique label.",
		).Array().LinterFunc(lintResource).HasDefault([]any{}).Advanced(),
	}
}

// CustomResourceSpecs returns field specs for all registered custom resource
// types in the given environment. These should be appended to the schema.
func CustomResourceSpecs(env *bundle.Environment) docs.FieldSpecs {
	var specs docs.FieldSpecs
	for _, crt := range env.CustomResourceTypes() {
		children := docs.FieldSpecs{
			docs.FieldString("label", "A unique label for this resource."),
		}
		children = append(children, crt.Fields...)

		specs = append(specs, docs.FieldObject(
			crt.Name,
			fmt.Sprintf("A list of %s resources, each must have a unique label.", crt.Name),
		).WithChildren(children...).Array().LinterFunc(lintResource).HasDefault([]any{}).Advanced())
	}
	return specs
}
