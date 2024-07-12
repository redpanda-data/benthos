package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"runtime"
	"sync"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	ifilepath "github.com/redpanda-data/benthos/v4/internal/filepath"
	"github.com/redpanda-data/benthos/v4/internal/filepath/ifs"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

func lintCliCommand(cliOpts *common.CLIOpts) *cli.Command {
	flags := []cli.Flag{
		&cli.BoolFlag{
			Name:  "deprecated",
			Value: false,
			Usage: "Print linting errors for the presence of deprecated fields.",
		},
		&cli.BoolFlag{
			Name:  "labels",
			Value: false,
			Usage: "Print linting errors when components do not have labels.",
		},
		&cli.BoolFlag{
			Name:  "skip-env-var-check",
			Value: false,
			Usage: "Do not produce lint errors when environment interpolations exist without defaults within configs but aren't defined.",
		},

		// General config flags
		&cli.StringSliceFlag{
			Name:    common.RootFlagResources,
			Aliases: []string{"r"},
			Usage:   "pull in extra resources from a file, which can be referenced the same as resources defined in the main config, supports glob patterns (requires quotes)",
		},
	}
	flags = append(flags, common.EnvFileAndTemplateFlags(cliOpts, false)...)

	return &cli.Command{
		Name:  "lint",
		Usage: cliOpts.ExecTemplate("Parse {{.ProductName}} configs and report any linting errors"),
		Flags: flags,
		Description: cliOpts.ExecTemplate(`
Exits with a status code 1 if any linting errors are detected:

  {{.BinaryName}} lint ./configs/*.yaml
  {{.BinaryName}} lint ./foo.yaml ./bar.yaml
  {{.BinaryName}} lint ./configs/...

If a path ends with '...' then {{.ProductName}} will walk the target and lint any
files with the .yaml or .yml extension.`)[1:],
		Before: func(c *cli.Context) error {
			return common.PreApplyEnvFilesAndTemplates(c, cliOpts)
		},
		Action: func(c *cli.Context) error {
			return LintAction(c, cliOpts, cliOpts.Stderr)
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

func lintFile(opts *common.CLIOpts, path string, skipEnvVarCheck bool, spec docs.FieldSpecs, lConf docs.LintConfig) (pathLints []pathLint) {
	_, lints, err := config.NewReader("", nil, config.OptUseEnvLookupFunc(opts.SecretAccessFn)).
		ReadYAMLFileLinted(context.TODO(), spec, path, skipEnvVarCheck, lConf)
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
	return
}

func lintMDSnippets(path string, spec docs.FieldSpecs, lConf docs.LintConfig) (pathLints []pathLint) {
	rawBytes, err := ifs.ReadFile(ifs.OS(), path)
	if err != nil {
		pathLints = append(pathLints, pathLint{
			source: path,
			lint:   docs.NewLintError(1, docs.LintFailedRead, err),
		})
		return
	}

	startTag, endTag := []byte("```yaml"), []byte("```")

	nextSnippet := bytes.Index(rawBytes, startTag)
	for nextSnippet != -1 {
		nextSnippet += len(startTag)

		snippetLine := bytes.Count(rawBytes[:nextSnippet], []byte("\n")) + 1

		endOfSnippet := bytes.Index(rawBytes[nextSnippet:], endTag)
		if endOfSnippet == -1 {
			pathLints = append(pathLints, pathLint{
				source: path,
				lint:   docs.NewLintError(snippetLine, docs.LintFailedRead, errors.New("markdown snippet not terminated")),
			})
			return
		}
		endOfSnippet = nextSnippet + endOfSnippet + len(endTag)

		configBytes := rawBytes[nextSnippet : endOfSnippet-len(endTag)]
		if nextSnippet = bytes.Index(rawBytes[endOfSnippet:], []byte("```yaml")); nextSnippet != -1 {
			nextSnippet += endOfSnippet
		}

		cNode, err := docs.UnmarshalYAML(configBytes)
		if err != nil {
			pathLints = append(pathLints, pathLint{
				source: path,
				lint:   docs.NewLintError(snippetLine, docs.LintFailedRead, err),
			})
			continue
		}

		pConf, err := spec.ParsedConfigFromAny(cNode)
		if err != nil {
			var l docs.Lint
			if errors.As(err, &l) {
				l.Line += snippetLine - 1
				pathLints = append(pathLints, pathLint{
					source: path,
					lint:   l,
				})
			} else {
				pathLints = append(pathLints, pathLint{
					source: path,
					lint:   docs.NewLintError(snippetLine, docs.LintFailedRead, err),
				})
			}
		}

		if _, err := config.FromParsed(lConf.DocsProvider, pConf, nil); err != nil {
			var l docs.Lint
			if errors.As(err, &l) {
				l.Line += snippetLine - 1
				pathLints = append(pathLints, pathLint{
					source: path,
					lint:   l,
				})
			} else {
				pathLints = append(pathLints, pathLint{
					source: path,
					lint:   docs.NewLintError(snippetLine, docs.LintFailedRead, err),
				})
			}
		} else {
			for _, l := range spec.LintYAML(docs.NewLintContext(lConf), cNode) {
				l.Line += snippetLine - 1
				pathLints = append(pathLints, pathLint{
					source: path,
					lint:   l,
				})
			}
		}
	}
	return
}

// LintAction performs the benthos lint subcommand and returns the appropriate
// exit code. This function is exported for testing purposes only.
func LintAction(c *cli.Context, opts *common.CLIOpts, stderr io.Writer) error {
	targets, err := ifilepath.GlobsAndSuperPaths(ifs.OS(), c.Args().Slice(), "yaml", "yml")
	if err != nil {
		return fmt.Errorf("lint paths error: %w", err)
	}
	if conf := opts.RootFlags.GetConfig(c); conf != "" {
		targets = append(targets, conf)
	}
	targets = append(targets, opts.RootFlags.GetResources(c)...)

	lConf := docs.NewLintConfig(opts.Environment)
	lConf.BloblangEnv = bloblang.XWrapEnvironment(opts.BloblEnvironment)
	lConf.RejectDeprecated = c.Bool("deprecated")
	lConf.RequireLabels = c.Bool("labels")
	skipEnvVarCheck := c.Bool("skip-env-var-check")

	spec := opts.MainConfigSpecCtor()

	var pathLintMut sync.Mutex
	var pathLints []pathLint
	threads := runtime.NumCPU()
	var wg sync.WaitGroup
	wg.Add(threads)
	for i := 0; i < threads; i++ {
		go func(threadID int) {
			defer wg.Done()
			for j, target := range targets {
				if j%threads != threadID {
					continue
				}
				if target == "" {
					continue
				}
				var lints []pathLint
				if path.Ext(target) == ".md" {
					lints = lintMDSnippets(target, spec, lConf)
				} else {
					lints = lintFile(opts, target, skipEnvVarCheck, spec, lConf)
				}
				if len(lints) > 0 {
					pathLintMut.Lock()
					pathLints = append(pathLints, lints...)
					pathLintMut.Unlock()
				}
			}
		}(i)
	}
	wg.Wait()

	if len(pathLints) == 0 {
		return nil
	}

	for _, lint := range pathLints {
		lintText := fmt.Sprintf("%v%v\n", lint.source, lint.lint.Error())
		if lint.lint.Type == docs.LintFailedRead || lint.lint.Type == docs.LintComponentMissing {
			fmt.Fprint(stderr, red(lintText))
		} else {
			fmt.Fprint(stderr, yellow(lintText))
		}
	}
	return &common.ErrExitCode{Err: errors.New("lint errors"), Code: 1}
}
