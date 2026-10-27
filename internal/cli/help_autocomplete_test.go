// Copyright 2025 Redpanda Data, Inc.

package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestTraverseHelpFlags(t *testing.T) {
	root := &cli.Command{
		Name:  "root",
		Usage: "root usage",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "help-autocomplete", Hidden: true},
			&cli.BoolFlag{Name: "verbose", Aliases: []string{"v"}, Usage: "be loud"},
		},
		Subcommands: []*cli.Command{
			{
				Name:  "sub",
				Usage: "sub usage",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "format", Aliases: []string{"fmt", "f"}, Value: "yaml", Usage: "output format"},
					&cli.StringSliceFlag{Name: "set", Usage: "set a field"},
					&cli.StringSliceFlag{Name: "resources", Value: cli.NewStringSlice("a.yaml", "b.yaml"), Usage: "resource files"},
					&cli.DurationFlag{Name: "timeout", Value: 5 * time.Second, Usage: "how long to wait"},
					&cli.StringFlag{Name: "name", Required: true, Usage: "a name"},
					&cli.IntFlag{Name: "count", Value: 3, Usage: "how many"},
					&cli.StringFlag{Name: "secret", Hidden: true, Usage: "hidden flag"},
					cli.HelpFlag,
				},
			},
			{
				Name:   "hidden",
				Hidden: true,
			},
		},
	}

	help := traverseHelp(root, nil)
	require.Len(t, help, 2)

	assert.Equal(t, "root", help[0].Path)
	assert.Equal(t, []pluginFlag{
		{Name: "verbose", Shorthand: "v", Type: "bool", Description: "be loud", Default: false},
	}, help[0].Flags)

	assert.Equal(t, "root_sub", help[1].Path)
	assert.Equal(t, []pluginFlag{
		{Name: "format", Shorthand: "f", Type: "string", Description: "output format", Default: "yaml"},
		{Name: "set", Type: "stringSlice", Description: "set a field", Default: []any{}},
		{Name: "resources", Type: "stringSlice", Description: "resource files", Default: "[a.yaml,b.yaml]"},
		{Name: "timeout", Type: "duration", Description: "how long to wait", Default: "5s"},
		{Name: "name", Type: "string", Description: "a name", Default: "", Required: true},
		{Name: "count", Type: "int", Description: "how many", Default: int64(3)},
	}, help[1].Flags)

	// Args are unchanged and still list hidden flags.
	assert.Contains(t, help[1].Args, "--secret")
}

func TestTraverseHelpFlagsAlwaysArray(t *testing.T) {
	b, err := json.Marshal(traverseHelp(&cli.Command{Name: "root"}, nil))
	require.NoError(t, err)
	assert.JSONEq(t, `[{"path":"root","flags":[]}]`, string(b))
}
