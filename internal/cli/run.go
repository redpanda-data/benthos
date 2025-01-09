// Copyright 2025 Redpanda Data, Inc.

package cli

import (
	"errors"

	"github.com/urfave/cli/v2"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
)

func runCliCommand(opts *common.CLIOpts) *cli.Command {
	flags := common.RunFlags(opts, false)
	flags = append(flags, common.EnvFileAndTemplateFlags(opts, false)...)

	return &cli.Command{
		Name:  "run",
		Usage: opts.ExecTemplate("Run {{.ProductName}} in normal mode against a specified config file"),
		Flags: flags,
		Before: func(c *cli.Context) error {
			return common.PreApplyEnvFilesAndTemplates(c, opts)
		},
		Description: opts.ExecTemplate(`
Run a {{.ProductName}} config.

  {{.BinaryName}} run ./foo.yaml`)[1:],
		Action: func(c *cli.Context) error {
			if c.Args().Len() > 0 {
				if c.Args().Len() > 1 || opts.RootFlags.Config != "" {
					return errors.New("a maximum of one config must be specified with the run command")
				}
				opts.RootFlags.Config = c.Args().First()
			}
			return common.RunService(c, opts, false)
		},
	}
}
