package common

import (
	"fmt"
	"os"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/parser"
	"github.com/redpanda-data/benthos/v4/internal/filepath"
	"github.com/redpanda-data/benthos/v4/internal/filepath/ifs"
	"github.com/redpanda-data/benthos/v4/internal/template"
	"github.com/urfave/cli/v2"
)

func RunFlags(opts *CLIOpts, hidden bool) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:   "log.level",
			Hidden: hidden,
			Value:  "",
			Usage:  "override the configured log level, options are: off, error, warn, info, debug, trace",
		},
		&cli.StringSliceFlag{
			Name:    "set",
			Hidden:  hidden,
			Aliases: []string{"s"},
			Usage:   "set a field (identified by a dot path) in the main configuration file, e.g. `\"metrics.type=prometheus\"`",
		},
		&cli.StringSliceFlag{
			Name:    "resources",
			Hidden:  hidden,
			Aliases: []string{"r"},
			Usage:   "pull in extra resources from a file, which can be referenced the same as resources defined in the main config, supports glob patterns (requires quotes)",
		},
		&cli.BoolFlag{
			Name:   "chilled",
			Hidden: hidden,
			Value:  false,
			Usage:  "continue to execute a config containing linter errors",
		},
		&cli.BoolFlag{
			Name:    "watcher",
			Hidden:  hidden,
			Aliases: []string{"w"},
			Value:   false,
			Usage:   "EXPERIMENTAL: watch config files for changes and automatically apply them",
		},
	}
}

func EnvFileAndTemplateFlags(opts *CLIOpts, hidden bool) []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "env-file",
			Hidden:  hidden,
			Aliases: []string{"e"},
			Value:   cli.NewStringSlice(),
			Usage:   "import environment variables from a dotenv file",
		},
		&cli.StringSliceFlag{
			Name:    "templates",
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
	dotEnvPaths, err := filepath.Globs(ifs.OS(), c.StringSlice("env-file"))
	if err != nil {
		fmt.Printf("Failed to resolve env file glob pattern: %v\n", err)
		os.Exit(1)
	}
	for _, dotEnvFile := range dotEnvPaths {
		dotEnvBytes, err := ifs.ReadFile(ifs.OS(), dotEnvFile)
		if err != nil {
			fmt.Printf("Failed to read dotenv file: %v\n", err)
			os.Exit(1)
		}
		vars, err := parser.ParseDotEnvFile(dotEnvBytes)
		if err != nil {
			fmt.Printf("Failed to parse dotenv file: %v\n", err)
			os.Exit(1)
		}
		for k, v := range vars {
			if err = os.Setenv(k, v); err != nil {
				fmt.Printf("Failed to set env var '%v': %v\n", k, err)
				os.Exit(1)
			}
		}
	}

	templatesPaths, err := filepath.Globs(ifs.OS(), c.StringSlice("templates"))
	if err != nil {
		fmt.Printf("Failed to resolve template glob pattern: %v\n", err)
		os.Exit(1)
	}
	lints, err := template.InitTemplates(templatesPaths...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Template file read error: %v\n", err)
		os.Exit(1)
	}
	if !c.Bool("chilled") && len(lints) > 0 {
		for _, lint := range lints {
			fmt.Fprintln(os.Stderr, lint)
		}
		fmt.Println(opts.ExecTemplate("Shutting down due to linter errors, to prevent shutdown run {{.ProductName}} with --chilled"))
		os.Exit(1)
	}
	return nil
}
