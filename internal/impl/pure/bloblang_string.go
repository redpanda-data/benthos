package pure

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

func init() {
	if err := bloblang.RegisterMethodV2("parse_form_url_encoded",
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
		}); err != nil {
		panic(err)
	}

	if err := bloblang.RegisterMethodV2("lint_yaml_config",
		bloblang.NewPluginSpec().
			Category(query.MethodCategoryParsing).
			Version("4.30.1").
			Beta().
			Description(`Lints a yaml configuration and returns an array of linting errors if any.`).
			Param(bloblang.NewBoolParam("deprecated").Description("Emit linting errors for the presence of deprecated components and fields.").Default(false)).
			Param(bloblang.NewBoolParam("require_labels").Description("Emit linting errors when components do not have labels.").Default(false)).
			Param(bloblang.NewBoolParam("skip_env_var_check").Description("Suppress lint errors when environment interpolations exist without defaults within configs but aren't defined.").Default(false)).
			Example("", `root = content().lint_yaml_config()`,
				[2]string{
					`input:
  generate:
    count: 1
`,
					`["(3,1) field mapping is required"]`,
				},
			),
		func(args *bloblang.ParsedParams) (bloblang.Method, error) {
			linterConf := docs.NewLintConfig(bundle.GlobalEnvironment)

			if deprecated, err := args.GetOptionalBool("deprecated"); err != nil {
				return nil, err
			} else {
				linterConf.RejectDeprecated = *deprecated
			}
			if requireLabels, err := args.GetOptionalBool("require_labels"); err != nil {
				return nil, err
			} else {
				linterConf.RequireLabels = *requireLabels
			}

			skipEnvVarCheck, err := args.GetOptionalBool("skip_env_var_check")
			if err != nil {
				return nil, err
			}
			var envConfRdr *config.Reader
			if !*skipEnvVarCheck {
				envConfRdr = config.NewReader("", nil)
			}

			return bloblang.BytesMethod(func(data []byte) (any, error) {
				var outputLints []any

				if !*skipEnvVarCheck {
					var err error
					if data, err = envConfRdr.ReplaceEnvVariables(context.Background(), data); err != nil {
						var errEnvMissing *config.ErrMissingEnvVars
						if errors.As(err, &errEnvMissing) {
							outputLints = append(outputLints, docs.NewLintError(1, docs.LintMissingEnvVar, err).Error())
						} else {
							return nil, fmt.Errorf("failed to replace env vars: %w", err)
						}
					}
				}

				if bytes.HasPrefix(data, []byte("# BENTHOS LINT DISABLE")) {
					return outputLints, nil
				}

				configLints, err := config.LintYAMLBytes(linterConf, data)
				if err != nil {
					return nil, fmt.Errorf("failed to parse yaml: %w", err)
				}

				for _, lint := range configLints {
					outputLints = append(outputLints, lint.Error())
				}

				return outputLints, nil
			}), nil
		}); err != nil {
		panic(err)
	}
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
