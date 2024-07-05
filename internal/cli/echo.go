package cli

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/docs"
)

func echoCliCommand(opts *common.CLIOpts) *cli.Command {
	flags := []cli.Flag{
		&cli.StringSliceFlag{
			Name:    common.RootFlagSet,
			Aliases: []string{"s"},
			Usage:   "set a field (identified by a dot path) in the main configuration file, e.g. \"metrics.type=prometheus\"",
		},
		&cli.StringSliceFlag{
			Name:    common.RootFlagResources,
			Aliases: []string{"r"},
			Usage:   "pull in extra resources from a file, which can be referenced the same as resources defined in the main config, supports glob patterns (requires quotes)",
		},
	}
	flags = append(flags, common.EnvFileAndTemplateFlags(opts, false)...)

	return &cli.Command{
		Name:  "echo",
		Usage: "Parse a config file and echo back a normalised version",
		Flags: flags,
		Description: opts.ExecTemplate(`
This simple command is useful for sanity checking a config if it isn't
behaving as expected, as it shows you a normalised version after environment
variables have been resolved:

  {{.BinaryName}} echo ./config.yaml | less
  {{.BinaryName}} echo --set 'input.generate.mapping=root.id = uuid_v4()'
  
  `)[1:],
		Before: func(c *cli.Context) error {
			return common.PreApplyEnvFilesAndTemplates(c, opts)
		},
		Action: func(c *cli.Context) error {
			if c.Args().Len() > 0 {
				if c.Args().Len() > 1 {
					fmt.Fprintln(os.Stderr, "A maximum of one config must be specified with the echo command")
					os.Exit(1)
				}
				opts.RootFlags.Config = c.Args().First()
			}

			_, _, confReader := common.ReadConfig(c, opts, false)
			_, pConf, _, err := confReader.Read()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Configuration file read error: %v\n", err)
				os.Exit(1)
			}
			var node yaml.Node
			if err = node.Encode(pConf.Raw()); err == nil {
				sanitConf := docs.NewSanitiseConfig(opts.Environment)
				sanitConf.RemoveTypeField = true
				sanitConf.ScrubSecrets = true
				err = opts.MainConfigSpecCtor().SanitiseYAML(&node, sanitConf)
			}
			if err == nil {
				var configYAML []byte
				if configYAML, err = docs.MarshalYAML(node); err == nil {
					fmt.Println(string(configYAML))
				}
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Echo error: %v\n", err)
				os.Exit(1)
			}
			return nil
		},
	}
}
