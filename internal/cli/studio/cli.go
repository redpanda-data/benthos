// Copyright 2025 Redpanda Data, Inc.

package studio

import (
	"github.com/urfave/cli/v2"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
)

// CliCommand is a cli.Command definition for interacting with Benthos studio.
func CliCommand(cliOpts *common.CLIOpts) *cli.Command {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:    "endpoint",
			Aliases: []string{"e"},
			Value:   "https://studio.benthos.dev",
			Usage:   "Specify the URL of the Benthos studio server to connect to.",
		},
	}

	return &cli.Command{
		Name:   "studio",
		Usage:  "Interact with Benthos studio (https://studio.benthos.dev)",
		Flags:  flags,
		Hidden: true,
		Description: `
EXPERIMENTAL: This subcommand is experimental and therefore are subject to
change outside of major version releases.`[1:],
		Subcommands: []*cli.Command{
			syncSchemaCommand(cliOpts),
			pullCommand(cliOpts),
		},
	}
}
