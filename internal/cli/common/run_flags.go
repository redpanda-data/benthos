// Copyright 2025 Redpanda Data, Inc.

package common

import (
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/parser"
	"github.com/redpanda-data/benthos/v4/internal/filepath"
	"github.com/redpanda-data/benthos/v4/internal/filepath/ifs"
	"github.com/redpanda-data/benthos/v4/internal/template"
)

// RootCommonFlags represents a collection of flags that could be set at the
// root of the cli, even in the case of a subcommand being specified afterwards.
//
// This is odd behaviour but we need to support it for backwards compatibility.
// And so, in cases where a subcommand needs to override these flags we use this
// type to extract flag arguments in order to ensure that root level flags are
// still honoured.
type RootCommonFlags struct {
	Config    string
	LogLevel  string
	Set       []string
	Resources []string
	Chilled   bool
	Watcher   bool
}

// RootCommonFlagsExtract attempts to read all common root flags from a cli
// context (presumed to be the root context).
func (c *CLIOpts) RootCommonFlagsExtract(ctx *cli.Context) {
	c.RootFlags.Config = ctx.String(RootFlagConfig)
	c.RootFlags.LogLevel = ctx.String(RootFlagLogLevel)
	c.RootFlags.Set = ctx.StringSlice(RootFlagSet)
	c.RootFlags.Resources = ctx.StringSlice(RootFlagResources)
	c.RootFlags.Chilled = ctx.Bool(RootFlagChilled)
	c.RootFlags.Watcher = ctx.Bool(RootFlagWatcher)
}

// GetConfig attempts to read a config flag either from the current context, or
// falls back to whatever the root context set it to.
func (r *RootCommonFlags) GetConfig(c *cli.Context) string {
	if v := c.String(RootFlagConfig); v != "" {
		return v
	}
	return r.Config
}

// GetLogLevel attempts to read a config flag either from the current context,
// or falls back to whatever the root context set it to.
func (r *RootCommonFlags) GetLogLevel(c *cli.Context) string {
	if v := c.String(RootFlagLogLevel); v != "" {
		return v
	}
	return r.LogLevel
}

// GetSet attempts to read a config flag either from the current context, or
// falls back to whatever the root context set it to.
func (r *RootCommonFlags) GetSet(c *cli.Context) []string {
	if v := c.StringSlice(RootFlagSet); len(v) > 0 {
		return v
	}
	return r.Set
}

// GetResources attempts to read a config flag either from the current context,
// or falls back to whatever the root context set it to.
func (r *RootCommonFlags) GetResources(c *cli.Context) []string {
	if v := c.StringSlice(RootFlagResources); len(v) > 0 {
		return v
	}
	return r.Resources
}

// GetChilled attempts to read a config flag either from the current context,
// or falls back to whatever the root context set it to.
func (r *RootCommonFlags) GetChilled(c *cli.Context) bool {
	return c.Bool(RootFlagChilled) || r.Chilled
}

// GetWatcher attempts to read a config flag either from the current context,
// or falls back to whatever the root context set it to.
func (r *RootCommonFlags) GetWatcher(c *cli.Context) bool {
	return c.Bool(RootFlagWatcher) || r.Watcher
}

//------------------------------------------------------------------------------

// Common names for old root level flags
const (
	RootFlagConfig    = "config"
	RootFlagLogLevel  = "log.level"
	RootFlagSet       = "set"
	RootFlagResources = "resources"
	RootFlagChilled   = "chilled"
	RootFlagWatcher   = "watcher"
	RootFlagEnvFile   = "env-file"
	RootFlagTemplates = "templates"
)

// RunFlags is the full set of root level flags that have been deprecated and
// are now documented at each subcommand that requires them.
func RunFlags(opts *CLIOpts, hidden bool) []cli.Flag {
	f := []cli.Flag{
		&cli.StringFlag{
			Name:   RootFlagLogLevel,
			Hidden: hidden,
			Value:  "",
			Usage:  "override the configured log level, options are: off, error, warn, info, debug, trace",
		},
		&cli.StringSliceFlag{
			Name:    RootFlagSet,
			Hidden:  hidden,
			Aliases: []string{"s"},
			Usage:   "set a field (identified by a dot path) in the main configuration file, e.g. \"metrics.type=prometheus\"",
		},
		&cli.StringSliceFlag{
			Name:    RootFlagResources,
			Hidden:  hidden,
			Aliases: []string{"r"},
			Usage:   "pull in extra resources from a file, which can be referenced the same as resources defined in the main config, supports glob patterns (requires quotes)",
		},
		&cli.BoolFlag{
			Name:   RootFlagChilled,
			Hidden: hidden,
			Value:  false,
			Usage:  "continue to execute a config containing linter errors",
		},
		&cli.BoolFlag{
			Name:    RootFlagWatcher,
			Hidden:  hidden,
			Aliases: []string{"w"},
			Value:   false,
			Usage:   "EXPERIMENTAL: watch config files for changes and automatically apply them",
		},
	}
	return append(f, opts.CustomRunFlags...)
}

// EnvFileAndTemplateFlags represents env file and template flags that are used
// by some subcommands.
func EnvFileAndTemplateFlags(opts *CLIOpts, hidden bool) []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{
			Name:    RootFlagEnvFile,
			Hidden:  hidden,
			Aliases: []string{"e"},
			Value:   cli.NewStringSlice(),
			Usage:   "import environment variables from a dotenv file",
		},
		&cli.StringSliceFlag{
			Name:    RootFlagTemplates,
			Hidden:  hidden,
			Aliases: []string{"t"},
			Usage:   opts.ExecTemplate("EXPERIMENTAL: import {{.ProductName}} templates, supports glob patterns (requires quotes)"),
		},
	}
}

// PreApplyEnvFilesAndTemplates takes a cli context and checks for flags
// `env-file` and `templates` in order to parse and execute them before the CLI
// proceeds onto the next behaviour.
func PreApplyEnvFilesAndTemplates(c *cli.Context, opts *CLIOpts) error {
	dotEnvPaths, err := filepath.Globs(ifs.OS(), c.StringSlice(RootFlagEnvFile))
	if err != nil {
		return fmt.Errorf("failed to resolve env file glob pattern: %w", err)
	}
	for _, dotEnvFile := range dotEnvPaths {
		dotEnvBytes, err := ifs.ReadFile(ifs.OS(), dotEnvFile)
		if err != nil {
			return fmt.Errorf("failed to read dotenv file: %w", err)
		}
		vars, err := parser.ParseDotEnvFile(dotEnvBytes)
		if err != nil {
			return fmt.Errorf("failed to parse dotenv file: %w", err)
		}
		for k, v := range vars {
			if err = os.Setenv(k, v); err != nil {
				return fmt.Errorf("failed to set env var '%v': %w", k, err)
			}
		}
	}

	templatesPaths, err := filepath.Globs(ifs.OS(), c.StringSlice(RootFlagTemplates))
	if err != nil {
		return fmt.Errorf("failed to resolve template glob pattern: %w", err)
	}
	lints, err := template.InitTemplates(opts.Environment, opts.BloblEnvironment, templatesPaths...)
	if err != nil {
		return fmt.Errorf("template file read error: %w", err)
	}
	if !opts.RootFlags.GetChilled(c) && len(lints) > 0 {
		for _, lint := range lints {
			fmt.Fprintln(opts.Stderr, lint)
		}
		return errors.New(opts.ExecTemplate("Shutting down due to linter errors, to prevent shutdown run {{.ProductName}} with --chilled"))
	}
	return nil
}
