// Copyright 2025 Redpanda Data, Inc.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strconv"
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
	Path  string       `json:"path,omitempty"`
	Short string       `json:"short,omitempty"`
	Long  string       `json:"long,omitempty"`
	Args  []string     `json:"args,omitempty"`
	Flags []pluginFlag `json:"flags"`
}

// pluginFlag matches the shape rpk uses for its own flags in --print-tree.
type pluginFlag struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     any    `json:"default"`
	Required    bool   `json:"required"`
	Deprecated  string `json:"deprecated,omitempty"`
}

// flagTypeAndDefault returns the pflag type name of a flag along with its
// default value formatted the way pflag would format it.
func flagTypeAndDefault(f cli.Flag) (typ, def string) {
	sliceDef := func(defaultText string, vals []string) string {
		if defaultText != "" {
			return defaultText
		}
		return "[" + strings.Join(vals, ",") + "]"
	}
	unquoted := func(defaultText, value string) string {
		if defaultText != "" {
			return defaultText
		}
		return value
	}
	switch t := f.(type) {
	case *cli.BoolFlag:
		return "bool", t.GetDefaultText()
	case *cli.StringFlag:
		return "string", unquoted(t.DefaultText, t.Value)
	case *cli.PathFlag:
		return "string", unquoted(t.DefaultText, t.Value)
	case *cli.IntFlag:
		return "int", t.GetDefaultText()
	case *cli.Int64Flag:
		return "int64", t.GetDefaultText()
	case *cli.UintFlag:
		return "uint", t.GetDefaultText()
	case *cli.Uint64Flag:
		return "uint64", t.GetDefaultText()
	case *cli.Float64Flag:
		return "float64", t.GetDefaultText()
	case *cli.DurationFlag:
		return "duration", t.GetDefaultText()
	case *cli.StringSliceFlag:
		var vals []string
		if t.Value != nil {
			vals = t.Value.Value()
		}
		return "stringSlice", sliceDef(t.DefaultText, vals)
	case *cli.IntSliceFlag:
		var vals []string
		if t.Value != nil {
			for _, v := range t.Value.Value() {
				vals = append(vals, strconv.Itoa(v))
			}
		}
		return "intSlice", sliceDef(t.DefaultText, vals)
	}
	if d, ok := f.(cli.DocGenerationFlag); ok {
		return "string", d.GetDefaultText()
	}
	return "string", ""
}

// parseDefault types a default value exactly as rpk does for its own flags.
func parseDefault(t, defValue string) any {
	if strings.HasSuffix(t, "Slice") || strings.HasSuffix(t, "Array") {
		if defValue == "" || defValue == "[]" {
			return []any{}
		}
		return defValue
	}
	if defValue == "" {
		if t == "string" {
			return ""
		}
		return nil
	}
	switch t {
	case "bool":
		if b, err := strconv.ParseBool(defValue); err == nil {
			return b
		}
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "count":
		if n, err := strconv.ParseInt(defValue, 10, 64); err == nil {
			return n
		}
	case "float32", "float64":
		if x, err := strconv.ParseFloat(defValue, 64); err == nil {
			return x
		}
	}
	return defValue
}

func commandFlags(cmd *cli.Command) []pluginFlag {
	flags := []pluginFlag{}
	for _, f := range cmd.Flags {
		names := f.Names()
		if len(names) == 0 {
			continue
		}
		if names[0] == "help" || names[0] == "help-autocomplete" {
			continue
		}
		if v, ok := f.(cli.VisibleFlag); ok && !v.IsVisible() {
			continue
		}
		pf := pluginFlag{Name: names[0]}
		for _, alias := range names[1:] {
			if len(alias) == 1 {
				pf.Shorthand = alias
				break
			}
		}
		if d, ok := f.(cli.DocGenerationFlag); ok {
			pf.Description = d.GetUsage()
		}
		if r, ok := f.(cli.RequiredFlag); ok {
			pf.Required = r.IsRequired()
		}
		typ, def := flagTypeAndDefault(f)
		pf.Type = typ
		pf.Default = parseDefault(typ, def)
		flags = append(flags, pf)
	}
	return flags
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
		Flags: commandFlags(cmd),
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

	// Custom commands allow tools to add their own features to the CLI.
	commands := opts.CustomCommands
	commandNames := map[string]struct{}{}
	for _, c := range commands {
		commandNames[c.Name] = struct{}{}
	}

	// Standard commands are part of the engine for now, but tools are able to
	// replace any of these with their own custom variants.
	for _, c := range []*cli.Command{
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
	} {
		if _, exists := commandNames[c.Name]; !exists {
			// Only add standard commands that haven't been replaced with a
			// custom variant are added.
			commands = append(commands, c)
		}
	}

	// Update usage and description strings with our templates.
	var execNestedTemplates func([]*cli.Command)
	execNestedTemplates = func(cs []*cli.Command) {
		for _, o := range cs {
			o.Usage = opts.ExecTemplate(o.Usage)
			o.Description = opts.ExecTemplate(o.Description)
			execNestedTemplates(o.Subcommands)
		}
	}
	execNestedTemplates(commands)

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
		Commands: commands,
	}

	app.OnUsageError = func(context *cli.Context, err error, isSubcommand bool) error {
		fmt.Fprintf(opts.Stdout, "Usage error: %v\n", err)
		_ = cli.ShowAppHelp(context)
		return err
	}
	return app
}
