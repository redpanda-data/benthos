// Copyright 2025 Redpanda Data, Inc.

package blobl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Jeffail/gabs/v2"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"

	"github.com/redpanda-data/benthos/v4/internal/bloblang"
	"github.com/redpanda-data/benthos/v4/internal/bloblang/mapping"
	"github.com/redpanda-data/benthos/v4/internal/bloblang/parser"
	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/filepath/ifs"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/internal/value"
)

var red = color.New(color.FgRed).SprintFunc()

// CliCommand is a cli.Command definition for running a blobl mapping.
func CliCommand(opts *common.CLIOpts) *cli.Command {
	return &cli.Command{
		Name:  "blobl",
		Usage: opts.ExecTemplate("Execute a {{.ProductName}} mapping on documents consumed via stdin"),
		Description: opts.ExecTemplate(`
Provides a convenient tool for mapping JSON documents over the command line:

  cat documents.jsonl | {{.BinaryName}} blobl 'foo.bar.map_each(this.uppercase())'

  echo '{"foo":"bar"}' | {{.BinaryName}} blobl -f ./mapping.blobl

Find out more about Bloblang at: {{.DocumentationURL}}/guides/bloblang/about`)[1:],
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "threads",
				Aliases: []string{"t"},
				Value:   1,
				Usage:   "the number of processing threads to use, when >1 ordering is no longer guaranteed.",
			},
			&cli.BoolFlag{
				Name:    "raw",
				Aliases: []string{"r"},
				Usage:   "consume raw strings.",
			},
			&cli.BoolFlag{
				Name:    "pretty",
				Aliases: []string{"p"},
				Usage:   "pretty-print output.",
			},
			&cli.StringFlag{
				Name:    "file",
				Aliases: []string{"f"},
				Usage:   "execute a mapping from a file.",
			},
			&cli.IntFlag{
				Name:  "max-token-length",
				Usage: "Set the buffer size for document lines.",
				Value: bufio.MaxScanTokenSize,
			},
			&cli.StringSliceFlag{
				Name:    common.RootFlagEnvFile,
				Aliases: []string{"e"},
				Value:   cli.NewStringSlice(),
				Usage:   "import environment variables from a dotenv file",
			},
		},
		Before: func(c *cli.Context) error {
			return common.PreApplyEnvFilesAndTemplates(c, opts)
		},
		Action: func(ctx *cli.Context) error {
			return run(ctx, opts)
		},
		Subcommands: []*cli.Command{
			{
				Name:  "server",
				Usage: "EXPERIMENTAL: Run a web server that hosts a Bloblang app",
				Description: `
Run a web server that provides an interactive application for writing and
testing Bloblang mappings.

**WARNING** This server is intended for local debugging and experimentation
purposes only. Do NOT expose it to the internet.`[1:],
				Action: runServer,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "host",
						Value: "localhost",
						Usage: "the host to bind to.",
					},
					&cli.StringFlag{
						Name:    "port",
						Value:   "4195",
						Aliases: []string{"p"},
						Usage:   "the port to bind to.",
					},
					&cli.BoolFlag{
						Name:    "no-open",
						Value:   false,
						Aliases: []string{"n"},
						Usage:   "do not open the app in the browser automatically.",
					},
					&cli.StringFlag{
						Name:    "mapping-file",
						Value:   "",
						Aliases: []string{"m"},
						Usage:   "an optional path to a mapping file to load as the initial mapping within the app.",
					},
					&cli.StringFlag{
						Name:    "input-file",
						Value:   "",
						Aliases: []string{"i"},
						Usage:   "an optional path to an input file to load as the initial input to the mapping within the app.",
					},
					&cli.BoolFlag{
						Name:    "write",
						Value:   false,
						Aliases: []string{"w"},
						Usage:   "when editing a mapping and/or input file write changes made back to the respective source file, if the file does not exist it will be created.",
					},
				},
			},
		},
	}
}

type execCache struct {
	msg  message.Batch
	vars map[string]any
}

func newExecCache() *execCache {
	return &execCache{
		msg:  message.QuickBatch([][]byte{[]byte(nil)}),
		vars: map[string]any{},
	}
}

func (e *execCache) executeMapping(exec *mapping.Executor, rawInput, prettyOutput bool, input []byte) (string, error) {
	e.msg.Get(0).SetBytes(input)

	var valuePtr *any
	var parseErr error

	lazyValue := func() *any {
		if valuePtr == nil && parseErr == nil {
			if rawInput {
				var value any = input
				valuePtr = &value
			} else {
				if jObj, err := e.msg.Get(0).AsStructured(); err == nil {
					valuePtr = &jObj
				} else {
					if errors.Is(err, message.ErrMessagePartNotExist) {
						parseErr = errors.New("message is empty")
					} else {
						parseErr = fmt.Errorf("parse as json: %w", err)
					}
				}
			}
		}
		return valuePtr
	}

	for k := range e.vars {
		delete(e.vars, k)
	}

	var result any = value.Nothing(nil)
	err := exec.ExecOnto(query.FunctionContext{
		Maps:     exec.Maps(),
		Vars:     e.vars,
		MsgBatch: e.msg,
		NewMeta:  e.msg.Get(0),
		NewValue: &result,
	}.WithValueFunc(lazyValue), mapping.AssignmentContext{
		Vars:  e.vars,
		Meta:  e.msg.Get(0),
		Value: &result,
	})
	if err != nil {
		var ctxErr query.ErrNoContext
		if parseErr != nil && errors.As(err, &ctxErr) {
			if ctxErr.FieldName != "" {
				err = fmt.Errorf("unable to reference message as structured (with 'this.%v'): %w", ctxErr.FieldName, parseErr)
			} else {
				err = fmt.Errorf("unable to reference message as structured (with 'this'): %w", parseErr)
			}
		}
		return "", err
	}

	var resultStr string
	switch t := result.(type) {
	case string:
		resultStr = t
	case []byte:
		resultStr = string(t)
	case value.Delete:
		return "", nil
	case value.Nothing:
		// Do not change the original contents
		if v := lazyValue(); v != nil {
			gObj := gabs.Wrap(v)
			if prettyOutput {
				resultStr = gObj.StringIndent("", "  ")
			} else {
				resultStr = gObj.String()
			}
		} else {
			resultStr = string(input)
		}
	default:
		gObj := gabs.Wrap(result)
		if prettyOutput {
			resultStr = gObj.StringIndent("", "  ")
		} else {
			resultStr = gObj.String()
		}
	}

	// TODO: Return metadata as well?
	return resultStr, nil
}

func run(c *cli.Context, opts *common.CLIOpts) error {
	if err := opts.CustomRunExtractFn(c); err != nil {
		return err
	}

	t := c.Int("threads")
	if t < 1 {
		t = 1
	}
	raw := c.Bool("raw")
	pretty := c.Bool("pretty")
	file := c.String("file")
	mb := []byte(c.Args().First())

	if file != "" {
		if len(mb) > 0 {
			return errors.New("invalid flags, unable to execute both a file mapping and an inline mapping")
		}
		mappingBytes, err := ifs.ReadFile(ifs.OS(), file)
		if err != nil {
			return fmt.Errorf("failed to read mapping file: %w", err)
		}
		mb = mappingBytes
	}

	mb, err := config.NewReader("", nil, config.OptUseEnvLookupFunc(opts.SecretAccessFn)).
		ReplaceEnvVariables(context.TODO(), mb)
	if err != nil {
		return fmt.Errorf("failed to replace env vars: %s", err)
	}

	mapping := string(mb)

	bEnv := bloblang.NewEnvironment().WithImporterRelativeToFile(file)
	exec, err := bEnv.NewMapping(mapping)
	if err != nil {
		if perr, ok := err.(*parser.Error); ok {
			return fmt.Errorf("failed to parse mapping: %v", perr.ErrorAtPositionStructured("", []rune(mapping)))
		}
		return errors.New(err.Error())
	}

	eGroup, _ := errgroup.WithContext(c.Context)

	inputsChan := make(chan []byte)
	eGroup.Go(func() error {
		defer close(inputsChan)

		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(nil, c.Int("max-token-length"))
		for scanner.Scan() {
			input := make([]byte, len(scanner.Bytes()))
			copy(input, scanner.Bytes())
			inputsChan <- input
		}
		return scanner.Err()
	})

	resultsChan := make(chan string)
	go func() {
		for res := range resultsChan {
			fmt.Fprintln(opts.Stdout, res)
		}
	}()

	for i := 0; i < t; i++ {
		eGroup.Go(func() error {
			execCache := newExecCache()
			for {
				input, open := <-inputsChan
				if !open {
					return nil
				}

				resultStr, err := execCache.executeMapping(exec, raw, pretty, input)
				if err != nil {
					fmt.Fprintln(opts.Stderr, red(fmt.Sprintf("failed to execute map: %v", err)))
					continue
				}
				resultsChan <- resultStr
			}
		})
	}

	err = eGroup.Wait()
	close(resultsChan)
	return err
}
