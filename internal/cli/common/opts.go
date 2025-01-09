// Copyright 2025 Redpanda Data, Inc.

package common

import (
	"bytes"
	"context"
	"io"
	"os"
	"path"
	"text/template"

	"github.com/urfave/cli/v2"

	"github.com/redpanda-data/benthos/v4/internal/bloblang"
	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/internal/log"
)

// StreamInitFunc is an optional func to be called when a stream (or streams
// mode) is initialised.
type StreamInitFunc func(s RunningStream) error

// CLIOpts contains the available CLI configuration options.
type CLIOpts struct {
	RootFlags *RootCommonFlags

	Stdout io.Writer
	Stderr io.Writer

	Version   string
	DateBuilt string

	BinaryName       string
	ProductName      string
	DocumentationURL string

	ConfigSearchPaths []string

	Environment      *bundle.Environment
	BloblEnvironment *bloblang.Environment
	SecretAccessFn   func(context.Context, string) (string, bool)

	MainConfigSpecCtor   func() docs.FieldSpecs // TODO: This becomes a service.Environment
	OnManagerInitialised func(mgr bundle.NewManagement, pConf *docs.ParsedConfig) error
	OnLoggerInit         func(l log.Modular) (log.Modular, error)

	OnStreamInit StreamInitFunc

	CustomRunFlags     []cli.Flag
	CustomRunExtractFn func(*cli.Context) error
}

// NewCLIOpts returns a new CLIOpts instance populated with default values.
func NewCLIOpts(version, dateBuilt string) *CLIOpts {
	binaryName := ""
	if len(os.Args) > 0 {
		binaryName = path.Base(os.Args[0])
	}
	return &CLIOpts{
		RootFlags:        &RootCommonFlags{},
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
		Version:          version,
		DateBuilt:        dateBuilt,
		BinaryName:       binaryName,
		ProductName:      "Benthos",
		DocumentationURL: "https://benthos.dev/docs",
		ConfigSearchPaths: []string{
			"/benthos.yaml",
			"/etc/benthos/config.yaml",
			"/etc/benthos.yaml",
		},
		Environment:      bundle.GlobalEnvironment,
		BloblEnvironment: bloblang.GlobalEnvironment(),
		SecretAccessFn: func(ctx context.Context, key string) (string, bool) {
			return os.LookupEnv(key)
		},
		MainConfigSpecCtor: config.Spec,
		OnManagerInitialised: func(mgr bundle.NewManagement, pConf *docs.ParsedConfig) error {
			return nil
		},
		OnLoggerInit: func(l log.Modular) (log.Modular, error) {
			return l, nil
		},
		OnStreamInit:       func(s RunningStream) error { return nil },
		CustomRunExtractFn: func(*cli.Context) error { return nil },
	}
}

// ExecTemplate parses a template and applies the CLI branding information to it.
func (c *CLIOpts) ExecTemplate(str string) string {
	t, err := template.New("cli").Parse(str)
	if err != nil {
		return str
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, struct {
		BinaryName       string
		ProductName      string
		DocumentationURL string
	}{
		BinaryName:       c.BinaryName,
		ProductName:      c.ProductName,
		DocumentationURL: c.DocumentationURL,
	}); err != nil {
		return str
	}

	return buf.String()
}
