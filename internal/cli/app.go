// Copyright 2025 Redpanda Data, Inc.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/redpanda-data/benthos/v4/internal/cli/blobl"
	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/cli/studio"
	clitemplate "github.com/redpanda-data/benthos/v4/internal/cli/template"
	"github.com/redpanda-data/benthos/v4/internal/cli/test"
)

// Build stamps.
var (
	Version   = "unknown"
	DateBuilt = "unknown"
)

func init() {
	if Version != "unknown" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, mod := range info.Deps {
			if mod.Path == "github.com/redpanda-data/benthos/v4" {
				if mod.Version != "(devel)" {
					Version = mod.Version
				}
				if mod.Replace != nil {
					v := mod.Replace.Version
					if v != "" && v != "(devel)" {
						Version = v
					}
				}
			}
		}
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && Version == "unknown" {
				Version = s.Value
			}
			if s.Key == "vcs.time" && DateBuilt == "unknown" {
				DateBuilt = s.Value
			}
		}
	}
}

//------------------------------------------------------------------------------

type pluginHelp struct {
	Path  string   `json:"path,omitempty"`
	Short string   `json:"short,omitempty"`
	Long  string   `json:"long,omitempty"`
	Args  []string `json:"args,omitempty"`
}

// In support of --help-autocomplete.
func traverseHelp(cmd *cli.Command, pieces []string) []pluginHelp {
	pieces = append(pieces, cmd.Name)
	var args []string
	for _, a := range cmd.Flags {
		for _, n := range a.Names() {
			if len(n) > 1 {
				args = append(args, "--"+n)
			} else {
				args = append(args, "-"+n)
			}
		}
	}
	help := []pluginHelp{{
		Path:  strings.Join(pieces, "_"),
		Short: cmd.Usage,
		Long:  cmd.Description,
		Args:  args,
	}}
	for _, cmd := range cmd.Subcommands {
		if cmd.Hidden {
			continue
		}
		help = append(help, traverseHelp(cmd, pieces)...)
	}
	return help
}

// App returns the full CLI app definition, this is useful for writing unit
// tests around the CLI.
func App(opts *common.CLIOpts) *cli.App {
	flags := []cli.Flag{
		&cli.BoolFlag{
			Name:    "version",
			Aliases: []string{"v"},
			Value:   false,
			Usage:   "display version info, then exit",
		},
		&cli.BoolFlag{
			Name:   "help-autocomplete",
			Value:  false,
			Usage:  "print json serialised cli argument definitions to assist with autocomplete",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:    common.RootFlagConfig,
			Aliases: []string{"c"},
			Hidden:  true,
			Value:   "",
			Usage:   "a path to a configuration file",
		},
	}
	flags = append(flags, common.RunFlags(opts, true)...)
	flags = append(flags, common.EnvFileAndTemplateFlags(opts, true)...)

	app := &cli.App{
		Name:  opts.BinaryName,
		Usage: opts.ExecTemplate("A stream processor for mundane tasks - {{.DocumentationURL}}"),
		Description: opts.ExecTemplate(`
Either run {{.ProductName}} as a stream processor or choose a command:

  {{.BinaryName}} list inputs
  {{.BinaryName}} create kafka//file > ./config.yaml
  {{.BinaryName}} run ./config.yaml
  {{.BinaryName}} run -r "./production/*.yaml" ./config.yaml`)[1:],
		Flags: flags,
		Before: func(c *cli.Context) error {
			opts.RootCommonFlagsExtract(c)
			return common.PreApplyEnvFilesAndTemplates(c, opts)
		},
		Action: func(c *cli.Context) error {
			if c.Bool("version") {
				fmt.Fprintf(opts.Stdout, "Version: %v\nDate: %v\n", opts.Version, opts.DateBuilt)
				return nil
			}
			if c.Bool("help-autocomplete") {
				_ = json.NewEncoder(opts.Stdout).Encode(traverseHelp(c.Command, nil))
				return nil
			}
			if c.Args().Len() > 0 {
				fmt.Fprintf(opts.Stderr, "Unrecognised command: %v\n", c.Args().First())
				_ = cli.ShowAppHelp(c)
				return &common.ErrExitCode{Err: errors.New("unrecognised command"), Code: 1}
			}

			return common.RunService(c, opts, false)
		},
		Commands: []*cli.Command{
			echoCliCommand(opts),
			lintCliCommand(opts),
			runCliCommand(opts),
			streamsCliCommand(opts),
			listCliCommand(opts),
			createCliCommand(opts),
			test.CliCommand(opts),
			clitemplate.CliCommand(opts),
			blobl.CliCommand(opts),
			studio.CliCommand(opts),
		},
	}

	app.OnUsageError = func(context *cli.Context, err error, isSubcommand bool) error {
		fmt.Fprintf(opts.Stdout, "Usage error: %v\n", err)
		_ = cli.ShowAppHelp(context)
		return err
	}
	return app
}
