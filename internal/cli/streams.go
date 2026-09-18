// Copyright 2025 Redpanda Data, Inc.

package cli

import (
	"github.com/urfave/cli/v2"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
)

func streamsCliCommand(opts *common.CLIOpts) *cli.Command {
	flags := []cli.Flag{
		&cli.BoolFlag{
			Name:   "no-api",
			Value:  false,
			Hidden: true,
			Usage:  "Deprecated: the streams-mode HTTP API is now disabled by default, enable it with --bind-http.",
		},
		&cli.BoolFlag{
			Name:  "bind-http",
			Value: false,
			Usage: "Register the streams-mode HTTP API endpoints for creating, updating and removing streams (POST /streams/{id} and so on). These are disabled by default because they accept pipeline configuration over HTTP. They are served on the service-wide HTTP server, so when enabling them secure that server by binding http.address to a trusted interface and/or configuring http.basic_auth. This flag does not affect the server's other endpoints such as /ping, /stats and /metrics.",
		},
		&cli.BoolFlag{
			Name:  "prefix-stream-endpoints",
			Value: true,
			Usage: "Whether HTTP endpoints registered by stream configs should be prefixed with the stream ID",
		},

		// Observability config only
		&cli.StringFlag{
			Name:    "observability",
			Aliases: []string{"o"},
			Value:   "",
			Usage:   "a path to a configuration file containing general service-wide fields such as http, logger, and so on",
		},
	}

	flags = append(flags, common.RunFlags(opts, false)...)
	flags = append(flags, common.EnvFileAndTemplateFlags(opts, false)...)

	return &cli.Command{
		Name:  "streams",
		Usage: opts.ExecTemplate("Run {{.ProductName}} in streams mode"),
		Flags: flags,
		Description: opts.ExecTemplate(`
Run {{.ProductName}} in streams mode, where multiple pipelines can be executed in a
single process. When the HTTP API is enabled with --bind-http the pipelines can
also be created, updated and removed via REST HTTP endpoints.

  {{.BinaryName}} streams
  {{.BinaryName}} streams -o ./root_config.yaml
  {{.BinaryName}} streams ./path/to/stream/configs ./and/some/more
  {{.BinaryName}} streams -o ./root_config.yaml ./streams/*.yaml

The config field specified with the --observability/-o flag is known as the root
config and should only contain observability and service-wide config fields such
as http, metrics, logger, resources, and so on.

For more information check out the docs at:
{{.DocumentationURL}}/guides/streams_mode/about`)[1:],
		Before: func(c *cli.Context) error {
			return common.PreApplyEnvFilesAndTemplates(c, opts)
		},
		Action: func(c *cli.Context) error {
			if oConf := c.String("observability"); oConf != "" {
				opts.RootFlags.Config = oConf
			}
			return common.RunService(c, opts, true)
		},
	}
}
