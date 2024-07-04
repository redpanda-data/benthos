package cli

import (
	"fmt"
	"os"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

func echoCliCommand(opts *common.CLIOpts) *cli.Command {
	return &cli.Command{
		Name:  "echo",
		Usage: "Parse a config file and echo back a normalised version",
		Description: opts.ExecTemplate(`
This simple command is useful for sanity checking a config if it isn't
behaving as expected, as it shows you a normalised version after environment
variables have been resolved:

  {{.BinaryName}} -c ./config.yaml echo | less`)[1:],
		Action: func(c *cli.Context) error {
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
