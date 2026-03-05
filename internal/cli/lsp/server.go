package lsp

import (
	"github.com/redpanda-data/benthos/v4/internal/cli/common"

	"github.com/tliron/commonlog"
	_ "github.com/tliron/commonlog/simple"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp/server"
	"github.com/urfave/cli/v2"
)

const lsName = "Redpanda Connect Language Server"

var (
	version string = "0.0.1"
	handler protocol.Handler
)

// CliCommand returns the CLI command for running the LSP server.
func CliCommand(opts *common.CLIOpts) *cli.Command {
	return &cli.Command{
		Name:  "lsp",
		Usage: "Run the Language Server Protocol (LSP) server",
		Description: `
Starts the Redpanda Connect Language Server Protocol (LSP) server.
This provides IDE features like autocompletion and hover documentation.`,
		Action: func(c *cli.Context) error {
			Start(opts)
			return nil
		},
	}
}

func Start(opts *common.CLIOpts) {
	commonlog.Configure(2, nil)
	state := newStateManager(opts)

	handler = protocol.Handler{
		Initialize: initialize,
		Initialized: func(context *glsp.Context, params *protocol.InitializedParams) error {
			return nil
		},
		Shutdown:               shutdown,
		TextDocumentCompletion: state.completion,
		TextDocumentDidChange:  state.didChange,
		TextDocumentDidOpen:    state.openDocument,
		TextDocumentDidClose:   state.closeDocument,
		TextDocumentHover:      state.onHover,
		DocumentLinkResolve:    state.documentLink,
	}
	server := server.NewServer(&handler, lsName, true)
	// server.RunStdio()
	server.RunTCP("127.0.0.1:8085")
}

func initialize(context *glsp.Context, params *protocol.InitializeParams) (any, error) {
	commonlog.NewInfoMessage(0, "Initializing Redpanda Connect LSP...")
	capabilities := handler.CreateServerCapabilities()
	capabilities.TextDocumentSync = 1 // full
	capabilities.HoverProvider = true

	trueVar := true
	capabilities.CompletionProvider = &protocol.CompletionOptions{
		ResolveProvider: &trueVar,
	}

	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    lsName,
			Version: &version,
		},
	}, nil
}

func shutdown(context *glsp.Context) error {
	return nil
}
