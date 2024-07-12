package template

import (
	"errors"
	"fmt"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	ifilepath "github.com/redpanda-data/benthos/v4/internal/filepath"
	"github.com/redpanda-data/benthos/v4/internal/filepath/ifs"
	"github.com/redpanda-data/benthos/v4/internal/template"
)

func lintCliCommand(opts *common.CLIOpts) *cli.Command {
	return &cli.Command{
		Name:  "lint",
		Flags: common.EnvFileAndTemplateFlags(opts, false),
		Usage: opts.ExecTemplate("Parse {{.ProductName}} templates and report any linting errors"),
		Description: opts.ExecTemplate(`
Exits with a status code 1 if any linting errors are detected:

  {{.BinaryName}} template lint
  {{.BinaryName}} template lint ./templates/*.yaml
  {{.BinaryName}} template lint ./foo.yaml ./bar.yaml
  {{.BinaryName}} template lint ./templates/...

If a path ends with '...' then {{.ProductName}} will walk the target and lint any
files with the .yaml or .yml extension.`)[1:],
		Before: func(c *cli.Context) error {
			return common.PreApplyEnvFilesAndTemplates(c, opts)
		},
		Action: func(c *cli.Context) error {
			targets, err := ifilepath.GlobsAndSuperPaths(ifs.OS(), c.Args().Slice(), "yaml", "yml")
			if err != nil {
				return fmt.Errorf("lint paths error: %w", err)
			}
			var pathLints []pathLint
			for _, target := range targets {
				if target == "" {
					continue
				}
				lints := lintFile(target)
				if len(lints) > 0 {
					pathLints = append(pathLints, lints...)
				}
			}
			if len(pathLints) == 0 {
				return nil
			}
			for _, lint := range pathLints {
				lintText := fmt.Sprintf("%v%v\n", lint.source, lint.lint.Error())
				if lint.lint.Type == docs.LintFailedRead {
					fmt.Fprint(opts.Stderr, red(lintText))
				} else {
					fmt.Fprint(opts.Stderr, yellow(lintText))
				}
			}
			return &common.ErrExitCode{Err: errors.New("lint errors"), Code: 1}
		},
	}
}

var (
	red    = color.New(color.FgRed).SprintFunc()
	yellow = color.New(color.FgYellow).SprintFunc()
)

type pathLint struct {
	source string
	lint   docs.Lint
}

func lintFile(path string) (pathLints []pathLint) {
	conf, lints, err := template.ReadConfigFile(path)
	if err != nil {
		pathLints = append(pathLints, pathLint{
			source: path,
			lint:   docs.NewLintError(1, docs.LintFailedRead, err),
		})
		return
	}

	for _, l := range lints {
		pathLints = append(pathLints, pathLint{
			source: path,
			lint:   l,
		})
	}

	testErrors, err := conf.Test()
	if err != nil {
		pathLints = append(pathLints, pathLint{
			source: path,
			lint:   docs.NewLintError(1, docs.LintFailedRead, err),
		})
		return
	}

	for _, tErr := range testErrors {
		pathLints = append(pathLints, pathLint{
			source: path,
			lint:   docs.NewLintError(1, docs.LintFailedRead, errors.New(tErr)),
		})
	}
	return
}
