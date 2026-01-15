package lsp

import (
	"fmt"
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/config/schema"
	"github.com/redpanda-data/benthos/v4/internal/docs"

	"github.com/goccy/go-yaml/parser"
	"github.com/tliron/commonlog"
	_ "github.com/tliron/commonlog/simple"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp/server"
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
	doc, ok := s.documents[params.TextDocument.URI]
	if !ok {
		return nil, nil
	}

	file, err := parser.ParseBytes([]byte(doc), parser.ParseComments)
	if err != nil {
		return nil, nil
	}
	token := findTokenAtPosition(file, int(params.Position.Line+1), int(params.Position.Character))
	path := strings.Split(token.Path, ".")

	if path[0] != "$" {
		return &protocol.Hover{Contents: ""}, nil
	}

	var (
		components []docs.ComponentSpec
	)

	// $.input.generate.interval
	// $.input.generate.mapping
	var (
		cs docs.ComponentSpec
		fs docs.FieldSpec
	)
	path = path[1:]
	cnt := 0
	for _, node := range path {
		cnt++
		fmt.Println(node)

		switch node {
		case "input":
			components = s.schema.Inputs
		case "output":
			components = s.schema.Outputs
		case "processors":
			components = s.schema.Processors
		case "cache":
			components = s.schema.Caches
		case "buffer":
			components = s.schema.Caches
		}

		// components
		for _, spec := range components {
			if node == spec.Name {
				cs = spec
				break
			}
		}

		// children
		if len(cs.Config.Children) > 0 {
			for _, c := range cs.Config.Children {
				if node == c.Name {
					fs = c
					break
				}
			}
		}

		if cnt == len(path) {
			switch node {
			case fs.Name:
				content := fmt.Sprintf("Field: %s (%s)\n-----------------------------\n%s\n", fs.Name, fs.Type, fs.Description)
				if len(fs.Examples) > 0 {
					content += fmt.Sprintf("-----------------------------\nExample:\n%s", fs.Examples[0])
				}
				return &protocol.Hover{Contents: content}, nil
			case cs.Name:
				content := fmt.Sprintf("Field: %s (%s)\n-----------------------------\n%s\n", cs.Name, cs.Type, cs.Description)
				if len(cs.Examples) > 0 {
					content += fmt.Sprintf("-----------------------------\nExample:\n%s", cs.Examples[0])
				}
				return &protocol.Hover{Contents: content}, nil
			}
		}
	}

	return &protocol.Hover{Contents: ""}, nil
}
