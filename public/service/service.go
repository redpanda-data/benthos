// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	ucli "github.com/urfave/cli/v2"

	"github.com/redpanda-data/benthos/v4/internal/bloblang"
	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/cli"
	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/internal/log"
)

// RunCLI executes Benthos as a CLI, allowing users to specify a configuration
// file path(s) and execute subcommands for linting configs, testing configs,
// etc. This is how a standard distribution of Benthos operates.
//
// This call blocks until either:
//
// 1. The service shuts down gracefully due to the inputs closing
// 2. A termination signal is received
// 3. The provided context has a deadline that is reached, triggering graceful termination
// 4. The provided context is cancelled (WARNING, this prevents graceful termination)
//
// In order to manage multiple Benthos stream lifecycles in a program use the
// StreamBuilder API instead.
func RunCLI(ctx context.Context, optFuncs ...CLIOptFunc) {
	s, err := RunCLIToCode(ctx, optFuncs...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	if s != 0 {
		os.Exit(s)
	}
}

// RunCLIToCode executes Benthos as a CLI, allowing users to specify a
// configuration file path(s) and execute subcommands for linting configs,
// testing configs, etc. This is how a standard distribution of Benthos
// operates. When the CLI exits the appropriate exit code is returned along with
// an error if operation was unsuccessful in an unexpected way.
//
// This call blocks until either:
//
// 1. The service shuts down gracefully due to the inputs closing
// 2. A termination signal is received
// 3. The provided context has a deadline that is reached, triggering graceful termination
// 4. The provided context is cancelled (WARNING, this prevents graceful termination)
//
// In order to manage multiple Benthos stream lifecycles in a program use the
// StreamBuilder API instead.
func RunCLIToCode(ctx context.Context, optFuncs ...CLIOptFunc) (exitCode int, err error) {
	cliOpts := &CLIOptBuilder{
		args: os.Args,
		opts: common.NewCLIOpts(cli.Version, cli.DateBuilt),
	}
	for _, o := range optFuncs {
		o(cliOpts)
	}
	cliOpts.opts.OnLoggerInit = func(l log.Modular) (log.Modular, error) {
		if cliOpts.outLoggerFn != nil {
			cliOpts.outLoggerFn(&Logger{m: l})
		}
		if cliOpts.teeLogger != nil {
			return log.TeeLogger(l, log.NewBenthosLogAdapter(cliOpts.teeLogger)), nil
		}
		return l, nil
	}

	if err := cli.App(cliOpts.opts).RunContext(ctx, cliOpts.args); err != nil {
		var cerr *common.ErrExitCode
		if errors.As(err, &cerr) {
			return cerr.Code, nil
		}
		return 1, err
	}
	return 0, nil
}

// CLIOptBuilder represents a CLI opts builder.
type CLIOptBuilder struct {
	args        []string
	opts        *common.CLIOpts
	teeLogger   *slog.Logger
	outLoggerFn func(*Logger)
}

// CLIOptFunc defines an option to pass through the standard Benthos CLI in order
// to customise it's behaviour.
type CLIOptFunc func(*CLIOptBuilder)

// CLIOptSetArgs overrides the default args provided to the CLI (os.Args) for
// the provided slice.
func CLIOptSetArgs(args ...string) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.args = args
	}
}

// CLIOptSetVersion overrides the default version and date built stamps.
func CLIOptSetVersion(version, dateBuilt string) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.opts.Version = version
		c.opts.DateBuilt = dateBuilt
	}
}

// CLIOptSetBinaryName overrides the default binary name in CLI help docs.
func CLIOptSetBinaryName(n string) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.opts.BinaryName = n
	}
}

// CLIOptSetProductName overrides the default product name in CLI help docs.
func CLIOptSetProductName(n string) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.opts.ProductName = n
	}
}

// CLIOptSetDocumentationURL overrides the default documentation URL in CLI help
// docs.
func CLIOptSetDocumentationURL(n string) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.opts.DocumentationURL = n
	}
}

// CLIOptSetShowRunCommand determines whether a `run` subcommand should appear
// in CLI help and autocomplete.
//
// Deprecated: The run command is always shown now.
func CLIOptSetShowRunCommand(show bool) CLIOptFunc {
	return func(c *CLIOptBuilder) {}
}

// CLIOptSetDefaultConfigPaths overrides the default paths used for detecting
// and loading config files when one was not provided explicitly with the
// --config flag.
func CLIOptSetDefaultConfigPaths(paths ...string) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.opts.ConfigSearchPaths = paths
	}
}

// CLIOptOnLoggerInit sets a closure to be called when the service-wide logger
// is initialised. A modified version can be returned, allowing you to mutate
// the fields and settings that it has.
func CLIOptOnLoggerInit(fn func(*Logger)) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.outLoggerFn = fn
	}
}

// CLIOptAddTeeLogger adds another logger to receive all log events from the
// service initialised via the CLI.
func CLIOptAddTeeLogger(l *slog.Logger) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.teeLogger = l
	}
}

// CLIOptSetMainSchemaFrom overrides the default Benthos configuration schema
// for another. A constructor is provided such that downstream components can
// still modify copies of the schema when needed.
//
// NOTE: This transfers the configuration schema but NOT the Environment plugins
// themselves, which is the global set by default.
func CLIOptSetMainSchemaFrom(fn func() *ConfigSchema) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.opts.MainConfigSpecCtor = func() docs.FieldSpecs {
			return fn().fields
		}
	}
}

// CLIOptSetEnvironment overrides the default Benthos plugin environment for
// another.
func CLIOptSetEnvironment(e *Environment) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.opts.Environment = e.internal
		c.opts.BloblEnvironment = e.bloblangEnv.XUnwrapper().(interface {
			Unwrap() *bloblang.Environment
		}).Unwrap()
	}
}

// CLIOptOnConfigParse sets a closure function to be called when a main
// configuration file load has occurred.
//
// If an error is returned this will be treated by the CLI the same as any other
// failure to parse the bootstrap config.
func CLIOptOnConfigParse(fn func(pConf *ParsedConfig) error) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.opts.OnManagerInitialised = func(mgr bundle.NewManagement, pConf *docs.ParsedConfig) error {
			return fn(&ParsedConfig{
				i:   pConf,
				mgr: mgr,
			})
		}
	}
}

// CLIOptSetEnvVarLookup overrides the default environment variable lookup
// function for config interpolation functions, this allows custom secret
// mechanisms to be referenced as an alternative, or in combination with,
// environment variables.
func CLIOptSetEnvVarLookup(fn func(context.Context, string) (string, bool)) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.opts.SecretAccessFn = fn
	}
}

// CLIOptOnStreamStart sets a function to be called when the CLI initialises
// either a single stream config execution (the `run` subcommand) or streams
// mode (the `streams` subcommand).
//
// The provided RunningStreamSummary grants access to information such as
// connectivity statuses of the stream(s) process.
func CLIOptOnStreamStart(fn func(s *RunningStreamSummary) error) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.opts.OnStreamInit = func(s common.RunningStream) error {
			return fn(&RunningStreamSummary{c: s})
		}
	}
}

// CLIOptCustomRunFlags sets a slice of custom cli flags and a closure to be
// called once those flags are parsed.
func CLIOptCustomRunFlags(flags []ucli.Flag, fn func(*ucli.Context) error) CLIOptFunc {
	return func(c *CLIOptBuilder) {
		c.opts.CustomRunFlags = flags
		c.opts.CustomRunExtractFn = fn
	}
}
