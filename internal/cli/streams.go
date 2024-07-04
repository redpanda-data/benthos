package cli

import (
	"os"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/urfave/cli/v2"
)

func streamsCliCommand(opts *common.CLIOpts) *cli.Command {
	flags := []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-api",
			Value: false,
			Usage: "Disable the HTTP API for streams mode",
		},
		&cli.BoolFlag{
			Name:  "prefix-stream-endpoints",
			Value: true,
			Usage: "Whether HTTP endpoints registered by stream configs should be prefixed with the stream ID",
		},
	}

	return &cli.Command{
		Name:  "streams",
		Usage: opts.ExecTemplate("Run {{.ProductName}} in streams mode"),
		Flags: flags,
		Description: opts.ExecTemplate(`
Run {{.ProductName}} in streams mode, where multiple pipelines can be executed in a
single process and can be created, updated and removed via REST HTTP
endpoints.

  {{.BinaryName}} streams
  {{.BinaryName}} -c ./root_config.yaml streams
  {{.BinaryName}} streams ./path/to/stream/configs ./and/some/more
  {{.BinaryName}} -c ./root_config.yaml streams ./streams/*.yaml

In streams mode the stream fields of a root target config (input, buffer,
pipeline, output) will be ignored. Other fields will be shared across all
loaded streams (resources, metrics, etc).

For more information check out the docs at:
{{.DocumentationURL}}/guides/streams_mode/about`)[1:],
		Action: func(c *cli.Context) error {
			os.Exit(common.RunService(c, opts, true))
			return nil
		},
	}
}
