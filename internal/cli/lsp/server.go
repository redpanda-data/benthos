package lsp

import (
	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/config/schema"
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

func Start(opts *common.CLIOpts) {
	commonlog.Configure(2, nil)
	state := newState(opts)

	handler = protocol.Handler{
		Initialize: initialize,
		Initialized: func(context *glsp.Context, params *protocol.InitializedParams) error {
			return nil
		},
		Shutdown:               shutdown,
		TextDocumentCompletion: textDocumentCompletion,
		TextDocumentDidChange:  textDocumentDidChange,
		TextDocumentDidOpen:    state.openDocument,
		TextDocumentDidClose:   state.closeDocument,
		TextDocumentHover:      state.onHover,
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

type state struct {
	documents map[string]string
	opts      *common.CLIOpts
	schema    schema.Full
}

func newState(opts *common.CLIOpts) state {
	schema := schema.New(opts.Version, opts.DateBuilt, opts.Environment, opts.BloblEnvironment)
	schema.Config = opts.MainConfigSpecCtor()

	return state{
		documents: map[string]string{},
		opts:      opts,
		schema:    schema,
	}
}

func (s *state) openDocument(context *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	s.documents[params.TextDocument.URI] = params.TextDocument.Text
	return nil
}

func (s *state) closeDocument(context *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	//TODO: Remove document
	return nil
}

func (s *state) updateDocument(uri, text string) {
	s.documents[uri] = text
}

// func (s *state) onHover(id int, uri string, position protocol.Position) {
func (s *state) onHover(context *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	h := &protocol.Hover{Contents: s.schema.Config[0].Description}
	return h, nil
}
