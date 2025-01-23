// Copyright 2025 Redpanda Data, Inc.

package test

import (
	"errors"
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/filepath"
	"github.com/redpanda-data/benthos/v4/internal/filepath/ifs"
	"github.com/redpanda-data/benthos/v4/internal/log"
)

// CliCommand is a cli.Command definition for unit testing.
func CliCommand(cliOpts *common.CLIOpts) *cli.Command {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:  "log",
			Value: "",
			Usage: "allow components to write logs at a provided level to stdout.",
		},

		&cli.StringSliceFlag{
			Name:    common.RootFlagResources,
			Aliases: []string{"r"},
			Usage:   "pull in extra resources from a file, which can be referenced the same as resources defined in the main config, supports glob patterns (requires quotes)",
		},
	}
	flags = append(flags, common.EnvFileAndTemplateFlags(cliOpts, false)...)

	return &cli.Command{
		Name:  "test",
		Usage: cliOpts.ExecTemplate("Execute {{.ProductName}} unit tests"),
		Flags: flags,
		Description: cliOpts.ExecTemplate(`
Execute any number of {{.ProductName}} unit test definitions. If one or more tests
fail the process will report the errors and exit with a status code 1.

  {{.BinaryName}} test ./path/to/configs/...
  {{.BinaryName}} test ./foo_configs/*.yaml ./bar_configs/*.yaml
  {{.BinaryName}} test ./foo.yaml

For more information check out the docs at:
{{.DocumentationURL}}/configuration/unit_testing`)[1:],
		Before: func(c *cli.Context) error {
			return common.PreApplyEnvFilesAndTemplates(c, cliOpts)
		},
		Action: func(c *cli.Context) error {
			if len(cliOpts.RootFlags.GetSet(c)) > 0 {
				return errors.New("cannot override fields with --set (-s) during unit tests")
			}
			resourcesPaths := cliOpts.RootFlags.GetResources(c)
			var err error
			if resourcesPaths, err = filepath.Globs(ifs.OS(), resourcesPaths); err != nil {
				return fmt.Errorf("failed to resolve resource glob pattern: %w", err)
			}
			if logLevel := c.String("log"); logLevel != "" {
				logConf := log.NewConfig()
				logConf.LogLevel = logLevel
				logger, err := log.New(cliOpts.Stdout, ifs.OS(), logConf)
				if err != nil {
					return fmt.Errorf("failed to init logger: %w", err)
				}
				if RunAll(cliOpts, c.Args().Slice(), "_benthos_test", true, logger, resourcesPaths) {
					return nil
				}
			} else if RunAll(cliOpts, c.Args().Slice(), "_benthos_test", true, log.Noop(), resourcesPaths) {
				return nil
			}
			return &common.ErrExitCode{Err: errors.New("lint errors"), Code: 1}
		},
	}
}
