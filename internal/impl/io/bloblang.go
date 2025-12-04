// Copyright 2025 Redpanda Data, Inc.

package io

import (
	"os"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

func init() {
	bloblang.MustRegisterFunctionV2("hostname",
		bloblang.NewPluginSpec().
			Impure().
			Category(query.FunctionCategoryEnvironment).
			Description(`Returns the hostname of the machine running Benthos. Useful for identifying which instance processed a message in distributed deployments.`).
			ExampleNotTested("", `root.processed_by = hostname()`),
		func(_ *bloblang.ParsedParams) (bloblang.Function, error) {
			return func() (any, error) {
				hn, err := os.Hostname()
				if err != nil {
					return nil, err
				}
				return hn, err
			}, nil
		},
	)

	bloblang.MustRegisterFunctionV2("env",
		bloblang.NewPluginSpec().
			Impure().
			StaticWithFunc(func(args *bloblang.ParsedParams) bool {
				noCache, _ := args.GetBool("no_cache")
				return !noCache
			}).
			Category(query.FunctionCategoryEnvironment).
			Description("Reads an environment variable and returns its value as a string. Returns `null` if the variable is not set. By default, values are cached for performance.").
			Param(bloblang.NewStringParam("name").
				Description("The name of the environment variable to read.")).
			Param(bloblang.NewBoolParam("no_cache").
				Description("Disable caching to read the latest value on each invocation.").
				Default(false)).
			ExampleNotTested("", `root.api_key = env("API_KEY")`).
			ExampleNotTested("", `root.database_url = env("DB_URL").or("localhost:5432")`).
			ExampleNotTested(
				"Use `no_cache` to read updated environment variables during runtime, useful for dynamic configuration changes.",
				`root.config = env(name: "DYNAMIC_CONFIG", no_cache: true)`,
			),
		func(args *bloblang.ParsedParams) (bloblang.Function, error) {
			name, err := args.GetString("name")
			if err != nil {
				return nil, err
			}

			noCache, err := args.GetBool("no_cache")
			if err != nil {
				return nil, err
			}

			var cachedValue any
			if !noCache {
				if valueStr, exists := os.LookupEnv(name); exists {
					cachedValue = valueStr
				}
			}

			return func() (any, error) {
				if noCache {
					if valueStr, exists := os.LookupEnv(name); exists {
						return valueStr, nil
					}
					return nil, nil
				}
				return cachedValue, nil
			}, nil
		},
	)

	bloblang.MustRegisterFunctionV2("file",
		bloblang.NewPluginSpec().
			Impure().
			StaticWithFunc(func(args *bloblang.ParsedParams) bool {
				noCache, _ := args.GetBool("no_cache")
				return !noCache
			}).
			Category(query.FunctionCategoryEnvironment).
			Description("Reads a file and returns its contents as bytes. Paths are resolved from the process working directory. For paths relative to the mapping file, use `file_rel`. By default, files are cached after first read.").
			Param(bloblang.NewStringParam("path").
				Description("The absolute or relative path to the file.")).
			Param(bloblang.NewBoolParam("no_cache").
				Description("Disable caching to read the latest file contents on each invocation.").
				Default(false)).
			ExampleNotTested("", `root.config = file("/etc/config.json").parse_json()`).
			ExampleNotTested("", `root.template = file("./templates/email.html").string()`).
			ExampleNotTested(
				"Use `no_cache` to read updated file contents during runtime, useful for hot-reloading configuration.",
				`root.rules = file(path: "/etc/rules.yaml", no_cache: true).parse_yaml()`,
			),
		func(args *bloblang.ParsedParams) (bloblang.Function, error) {
			path, err := args.GetString("path")
			if err != nil {
				return nil, err
			}

			noCache, err := args.GetBool("no_cache")
			if err != nil {
				return nil, err
			}

			var cachedPathBytes []byte
			if !noCache {
				// TODO: Obtain FS from bloblang environment.
				if cachedPathBytes, err = os.ReadFile(path); err != nil {
					return nil, err
				}
			}

			return func() (any, error) {
				if noCache {
					return os.ReadFile(path)
				}
				return cachedPathBytes, nil
			}, nil
		},
	)

	bloblang.MustRegisterFunctionV2("file_rel",
		bloblang.NewPluginSpec().
			Impure().
			StaticWithFunc(func(args *bloblang.ParsedParams) bool {
				noCache, _ := args.GetBool("no_cache")
				return !noCache
			}).
			Category(query.FunctionCategoryEnvironment).
			Description("Reads a file and returns its contents as bytes. Paths are resolved relative to the mapping file's directory, making it portable across different environments. By default, files are cached after first read.").
			Param(bloblang.NewStringParam("path").
				Description("The path to the file, relative to the mapping file's directory.")).
			Param(bloblang.NewBoolParam("no_cache").
				Description("Disable caching to read the latest file contents on each invocation.").
				Default(false)).
			ExampleNotTested("", `root.schema = file_rel("./schemas/user.json").parse_json()`).
			ExampleNotTested("", `root.lookup = file_rel("../data/lookup.csv").parse_csv()`).
			ExampleNotTested(
				"Use `no_cache` to read updated file contents during runtime, useful for reloading data files without restarting.",
				`root.translations = file_rel(path: "./i18n/en.yaml", no_cache: true).parse_yaml()`,
			),
		func(args *bloblang.ParsedParams) (bloblang.Function, error) {
			path, err := args.GetString("path")
			if err != nil {
				return nil, err
			}

			noCache, err := args.GetBool("no_cache")
			if err != nil {
				return nil, err
			}

			var cachedPathBytes []byte
			if !noCache {
				if cachedPathBytes, err = args.ImportFile(path); err != nil {
					return nil, err
				}
			}

			return func() (any, error) {
				if noCache {
					return args.ImportFile(path)
				}
				return cachedPathBytes, nil
			}, nil
		},
	)
}
