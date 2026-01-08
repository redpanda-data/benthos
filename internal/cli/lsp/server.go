package lsp

import (
	"github.com/tliron/commonlog"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp/server"

	_ "github.com/tliron/commonlog/simple"
)

const lsName = "Redpanda Connect Language Server"

var (
	version string = "0.0.1"
	handler protocol.Handler
)

func Start() {
	commonlog.Configure(2, nil)
	handler = protocol.Handler{
		Initialize: initialize,
		Initialized: func(context *glsp.Context, params *protocol.InitializedParams) error {
			return nil
		},
		Shutdown:               shutdown,
		TextDocumentCompletion: TextDocumentCompletion,
		TextDocumentDidOpen:    TextDocumentDidOpen,
		TextDocumentDidClose:   TextDocumentDidClose,
	}
	server := server.NewServer(&handler, lsName, true)
	// server.RunStdio()
	server.RunTCP("127.0.0.1:8085")
}

func initialize(context *glsp.Context, params *protocol.InitializeParams) (any, error) {
	commonlog.NewInfoMessage(0, "Initializing Redpanda Connect LSP...")
	capabilities := handler.CreateServerCapabilities()
	capabilities.TextDocumentSync = 1 // full

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
