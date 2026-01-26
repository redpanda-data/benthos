// Copyright 2025 Redpanda Data, Inc.

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	ifilepath "github.com/redpanda-data/benthos/v4/internal/filepath"
	"github.com/redpanda-data/benthos/v4/internal/filepath/ifs"
	"github.com/redpanda-data/benthos/v4/internal/log"
	"github.com/redpanda-data/benthos/v4/internal/manager"
	"github.com/redpanda-data/benthos/v4/internal/stream"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

func dryRunCliCommand(cliOpts *common.CLIOpts) *cli.Command {
	flags := []cli.Flag{
		&cli.BoolFlag{
			Name:  "verbose",
			Value: false,
			Usage: "Print the connectivity test result for each target file.",
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
		Name:  "dry-run",
		Usage: cliOpts.ExecTemplate("Test connectivity for components in {{.ProductName}} configs"),
		Flags: flags,
		Description: cliOpts.ExecTemplate(`
Exits with a status code 1 if any connectivity tests fail:

  {{.BinaryName}} connectivity-test ./configs/*.yaml
  {{.BinaryName}} connectivity-test ./foo.yaml ./bar.yaml`)[1:],
		Before: func(c *cli.Context) error {
			return common.PreApplyEnvFilesAndTemplates(c, cliOpts)
		},
		Action: func(c *cli.Context) error {
			return dryRunAction(c, cliOpts, cliOpts.Stderr)
		},
	}
}

type dryRunTest struct {
	source string
	result *component.ConnectionTestResult
}

func dryRunFile(c *cli.Context, opts *common.CLIOpts, filePath string) (pathTests []dryRunTest, err error) {
	// Create a lint config
	lConf := docs.NewLintConfig(opts.Environment)
	lConf.BloblangEnv = bloblang.XWrapEnvironment(opts.BloblEnvironment)

	confReaderOpts := []config.OptFunc{
		config.OptSetFullSpec(opts.MainConfigSpecCtor),
		config.OptAddOverrides(opts.RootFlags.GetSet(c)...),
		config.OptTestSuffix("_benthos_test"),
		config.OptSetLintConfig(lConf),
	}

	if opts.SecretAccessFn != nil {
		confReaderOpts = append(confReaderOpts, config.OptUseEnvLookupFunc(opts.SecretAccessFn))
	}

	// Read and lint the config file
	confReader := config.NewReader(filePath, nil, confReaderOpts...)
	conf, pConf, lints, readErr := confReader.Read()
	if readErr != nil {
		return nil, fmt.Errorf("failed to read config: %w", readErr)
	}
	defer func() {
		_ = confReader.Close(c.Context)
	}()

	if len(lints) > 0 {
		return nil, errors.New("config has lint errors, fix them first")
	}

	// Create a noop logger
	logger := log.Noop()
	if _, err = opts.OnLoggerInit(logger); err != nil {
		return
	}

	mgrOpts := []manager.OptFunc{
		manager.OptSetEngineVersion(opts.Version),
		manager.OptSetLogger(logger),
		manager.OptSetBloblangEnvironment(opts.BloblEnvironment),
		manager.OptSetEnvironment(opts.Environment),
	}

	// Create resource manager.
	var stoppableManager *manager.Type
	if stoppableManager, err = manager.New(conf.ResourceConfig, mgrOpts...); err != nil {
		err = fmt.Errorf("failed to initialise resources: %w", err)
		return
	}

	if err = opts.OnManagerInitialised(stoppableManager, pConf); err != nil {
		return
	}

	// Create the stream to test connectivity
	strm, err := stream.New(conf.Config, stoppableManager)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		_ = strm.Stop(ctx)
		_ = stoppableManager.CloseObservability(ctx)
	}()

	// Run connectivity tests
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	results := strm.ConnectionTest(ctx)
	for _, result := range results {
		pathTests = append(pathTests, dryRunTest{
			source: filePath,
			result: result,
		})
	}

	return pathTests, nil
}

func dryRunAction(c *cli.Context, opts *common.CLIOpts, stderr io.Writer) error {
	if err := opts.CustomRunExtractFn(c); err != nil {
		return err
	}

	targets, err := ifilepath.Globs(ifs.OS(), c.Args().Slice())
	if err != nil {
		return fmt.Errorf("connectivity-test paths error: %w", err)
	}
	if conf := opts.RootFlags.GetConfig(c); conf != "" {
		targets = append(targets, conf)
	}
	targets = append(targets, opts.RootFlags.GetResources(c)...)

	verbose := c.Bool("verbose")

	var pathTestMut sync.Mutex
	var pathTests []dryRunTest
	type result struct {
		target string
		ok     bool
		err    error
	}
	var testResults []result
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
				tests, testErr := dryRunFile(c, opts, target)
				pathTestMut.Lock()
				if testErr != nil {
					testResults = append(testResults, result{target, false, testErr})
				} else {
					hasFailure := false
					for _, test := range tests {
						pathTests = append(pathTests, test)
						if test.result.Err != nil && !errors.Is(test.result.Err, component.ErrConnectionTestNotSupported) {
							hasFailure = true
						}
					}
					testResults = append(testResults, result{target, !hasFailure, nil})
				}
				pathTestMut.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if verbose {
		for _, res := range testResults {
			if res.err != nil {
				fmt.Fprintf(opts.Stdout, "%v: %v (%v)\n", res.target, red("ERROR"), res.err)
			} else if res.ok {
				fmt.Fprintf(opts.Stdout, "%v: %v\n", res.target, green("OK"))
			} else {
				fmt.Fprintf(opts.Stdout, "%v: %v\n", res.target, yellow("FAILED"))
			}
		}
	}

	hasFailures := false
	for _, test := range pathTests {
		componentPath := test.result.Label
		if len(componentPath) == 0 {
			componentPath = "." + strings.Join(test.result.Path, ".")
		}

		if test.result.Err != nil {
			if errors.Is(test.result.Err, component.ErrConnectionTestNotSupported) {
				if verbose {
					fmt.Fprintf(opts.Stdout, "%v [%v]: %v\n", test.source, componentPath, yellow(test.result.Err.Error()))
				}
			} else {
				hasFailures = true
				fmt.Fprintf(stderr, "%v [%v]: %v\n", test.source, componentPath, red(test.result.Err.Error()))
			}
		} else if verbose {
			fmt.Fprintf(opts.Stdout, "%v [%v]: %v\n", test.source, componentPath, green("Connected"))
		}
	}

	if hasFailures {
		return &common.ErrExitCode{Err: errors.New("connectivity test failures"), Code: 1}
	}
	return nil
}
