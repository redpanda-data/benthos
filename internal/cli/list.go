// Copyright 2025 Redpanda Data, Inc.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/config/schema"
	"github.com/redpanda-data/benthos/v4/internal/cuegen"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/internal/jsonschema"
)

func listCliCommand(opts *common.CLIOpts) *cli.Command {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:  "format",
			Value: "text",
			Usage: "Print the component list in a specific format. Options are text, json, jsonschema, or cue.",
		},
		&cli.StringFlag{
			Name:  "docs-format",
			Value: "asciidoc",
			Usage: "The format of the doc strings carried by the json-full and cue formats and by jsonschema for Bloblang functions and methods. Options are asciidoc, as the specs hold them and the docs site needs them, or markdown, with links resolved to the documentation URL and Cloud conditionals resolved for this distribution.",
		},
		&cli.StringFlag{
			Name:  "status",
			Value: "",
			Usage: "Filter the component list to only those matching the given status. Options are stable, beta or experimental.",
		},

		// Template imports
		&cli.StringSliceFlag{
			Name:    common.RootFlagTemplates,
			Aliases: []string{"t"},
			Usage:   opts.ExecTemplate("EXPERIMENTAL: import {{.ProductName}} templates, supports glob patterns (requires quotes)"),
		},
	}

	return &cli.Command{
		Name:  "list",
		Flags: flags,
		Usage: opts.ExecTemplate("List all {{.ProductName}} component types"),
		Description: opts.ExecTemplate(`
If any component types are explicitly listed then only types of those
components will be shown.

  {{.BinaryName}} list
  {{.BinaryName}} list --format json inputs output
  {{.BinaryName}} list rate-limits buffers

When using --format jsonschema with bloblang-functions or bloblang-methods,
you can optionally specify function or method names to retrieve metadata for
only those specific items:

  {{.BinaryName}} list --format jsonschema bloblang-functions
  {{.BinaryName}} list --format jsonschema bloblang-functions uuid_v4
  {{.BinaryName}} list --format jsonschema bloblang-functions uuid_v4 nanoid
  {{.BinaryName}} list --format jsonschema bloblang-methods uppercase catch`)[1:],
		Before: func(c *cli.Context) error {
			return common.PreApplyEnvFilesAndTemplates(c, opts)
		},
		Action: func(c *cli.Context) error {
			if err := listComponents(c, opts); err != nil {
				return err
			}
			return nil
		},
	}
}

func listComponents(c *cli.Context, opts *common.CLIOpts) error {
	ofTypes := map[string]struct{}{}
	for _, k := range c.Args().Slice() {
		ofTypes[k] = struct{}{}
	}

	schema := schema.New(opts.Version, opts.DateBuilt, opts.Environment, opts.BloblEnvironment)
	schema.Config = opts.MainConfigSpecCtor()
	switch c.String("docs-format") {
	case "asciidoc", "":
	case "markdown":
		attrs := map[string]bool{}
		if opts.DocsAttributes != nil {
			for _, a := range opts.DocsAttributes() {
				attrs[a] = true
			}
		}
		if problems := schema.RenderMarkdown(docs.MarkdownOptions{
			BaseURL:       opts.DocumentationURL,
			Attributes:    attrs,
			ComponentURLs: opts.DocsComponentURLs,
			HasComponent:  opts.DocsHasComponent,
		}); len(problems) > 0 {
			return errors.New("doc strings have malformed conditionals:\n  " + strings.Join(problems, "\n  "))
		}
	default:
		return fmt.Errorf("unknown docs format %q, options are asciidoc or markdown", c.String("docs-format"))
	}
	// Filter by status after rendering, so links to components of another
	// status that this binary still ships stay links.
	if status := c.String("status"); status != "" {
		schema.ReduceToStatus(status)
	}

	switch c.String("format") {
	case "text":
		flat := schema.Flattened()
		i := 0
		for _, k := range []string{
			"inputs",
			"processors",
			"outputs",
			"caches",
			"rate-limits",
			"buffers",
			"metrics",
			"tracers",
			"scanners",
			"bloblang-functions",
			"bloblang-methods",
		} {
			if _, exists := ofTypes[k]; len(ofTypes) > 0 && !exists {
				continue
			}
			if i > 0 {
				fmt.Fprintln(opts.Stdout, "")
			}
			i++
			title := cases.Title(language.English).String(strings.ReplaceAll(k, "-", " "))
			fmt.Fprintf(opts.Stdout, "%v:\n", title)
			for _, t := range flat[k] {
				fmt.Fprintf(opts.Stdout, "  - %v\n", t)
			}
		}
	case "json":
		flat := schema.Flattened()
		if len(ofTypes) > 0 {
			for k := range flat {
				if _, exists := ofTypes[k]; !exists {
					delete(flat, k)
				}
			}
		}
		jsonBytes, err := json.Marshal(flat)
		if err != nil {
			panic(err)
		}
		fmt.Fprintln(opts.Stdout, string(jsonBytes))
	case "json-full":
		jsonBytes, err := json.Marshal(schema)
		if err != nil {
			panic(err)
		}
		fmt.Fprintln(opts.Stdout, string(jsonBytes))
	case "json-full-scrubbed":
		schema.Scrub()
		jsonBytes, err := json.Marshal(schema)
		if err != nil {
			panic(err)
		}
		fmt.Fprintln(opts.Stdout, string(jsonBytes))
	case "jsonschema":
		// Special handling for bloblang functions and methods
		if _, isFunctions := ofTypes["bloblang-functions"]; isFunctions {
			functions := schema.BloblangFunctions

			// Filter by specific function names if provided
			if len(ofTypes) > 1 {
				filtered := []query.FunctionSpec{}
				for _, fn := range functions {
					if _, requested := ofTypes[fn.Name]; requested {
						filtered = append(filtered, fn)
					}
				}
				functions = filtered
			}

			outputBloblangJSON(opts, "bloblang-functions", functions)
			return nil
		}
		if _, isMethods := ofTypes["bloblang-methods"]; isMethods {
			methods := schema.BloblangMethods

			// Filter by specific method names if provided
			if len(ofTypes) > 1 {
				filtered := []query.MethodSpec{}
				for _, method := range methods {
					if _, requested := ofTypes[method.Name]; requested {
						filtered = append(filtered, method)
					}
				}
				methods = filtered
			}

			outputBloblangJSON(opts, "bloblang-methods", methods)
			return nil
		}

		// Default: Component jsonschema
		jsonSchemaBytes, err := jsonschema.Marshal(schema.Config, opts.Environment)
		if err != nil {
			panic(err)
		}
		fmt.Fprintln(opts.Stdout, string(jsonSchemaBytes))
	case "cue":
		source, err := cuegen.GenerateSchema(schema)
		if err != nil {
			panic(err)
		}
		fmt.Fprintln(opts.Stdout, string(source))
	}
	return nil
}

func outputBloblangJSON(opts *common.CLIOpts, key string, data any) {
	jsonBytes, err := json.Marshal(map[string]any{key: data})
	if err != nil {
		panic(err)
	}
	fmt.Fprintln(opts.Stdout, string(jsonBytes))
}
