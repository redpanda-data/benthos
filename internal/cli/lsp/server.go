package lsp

import (
	"fmt"
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/config/schema"
	"github.com/redpanda-data/benthos/v4/internal/docs"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
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
	state := newState(opts)

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

func (s *state) didChange(context *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	// s.documents[params.TextDocument.URI] = params.TextDocument.TextDocumentIdentifier.
	return nil
}

func (s *state) completion(context *glsp.Context, params *protocol.CompletionParams) (any, error) {
	doc, ok := s.documents[params.TextDocument.URI]
	if !ok {
		return nil, nil
	}

	var (
		file *ast.File
		err  error
	)
	if file, err = parser.ParseBytes([]byte(doc), parser.ParseComments); err != nil {
		return nil, err
	}

	// $.input.generate.mapping
	var token *TokenWithPath
	if token, err = findTokenAtPosition(file, int(params.Position.Line+1), int(params.Position.Character+1)); err != nil {
		return nil, err
	}

	path := strings.Split(token.Path, ".")
	fmt.Printf("Token path: %s\n", token.Path)

	if path[0] != "$" {
		return &protocol.Hover{Contents: ""}, nil
	}

	var (
		components []docs.ComponentSpec
	)

	var (
		cs docs.ComponentSpec
		fs docs.FieldSpec
	)
	path = path[1:]
	cnt := 0
	for _, node := range path {
		cnt++

		switch node {
		case "input":
			components = s.schema.Inputs
		case "output":
			components = s.schema.Outputs
		case "processors", "processors[0]":
			components = s.schema.Processors
		case "cache":
			components = s.schema.Caches
		case "buffer":
			components = s.schema.Caches
		case "metrics":
			components = s.schema.Metrics
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
	}

	_ = fs
	_ = cs

	var completionItems []protocol.CompletionItem
	for _, opt := range cs.Config.Children {
		var insertText string
		if opt.Type == "string" {
			insertText = fmt.Sprintf("%s: %s", opt.Name, `""`)
		} else {
			insertText = fmt.Sprintf("%s: ", opt.Name)
		}
		kind := protocol.CompletionItemKind(5)
		completionItems = append(completionItems, protocol.CompletionItem{
			Label:         opt.Name,
			Kind:          &kind,
			Deprecated:    &opt.IsDeprecated,
			Documentation: opt.Description,
			InsertText:    &insertText,
		})
	}

	return completionItems, nil
}

func (s *state) openDocument(context *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	s.documents[params.TextDocument.URI] = params.TextDocument.Text
	return nil
}

func (s *state) closeDocument(context *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	doc := params.TextDocument.URI
	if _, ok := s.documents[doc]; ok {
		fmt.Printf("Closing document %s", doc)
		delete(s.documents, params.TextDocument.URI)
	}
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

	var (
		file *ast.File
		err  error
	)
	if file, err = parser.ParseBytes([]byte(doc), parser.ParseComments); err != nil {
		return nil, err
	}

	// $.input.generate.interval
	var token *TokenWithPath
	if token, err = findTokenAtPosition(file, int(params.Position.Line+1), int(params.Position.Character+1)); err != nil {
		return nil, err
	}

	path := strings.Split(token.Path, ".")
	fmt.Printf("Token path: %s\n", token.Path)

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

		switch node {
		case "input":
			components = s.schema.Inputs
		case "output":
			components = s.schema.Outputs
		case "processors", "processors[0]":
			components = s.schema.Processors
		case "cache":
			components = s.schema.Caches
		case "buffer":
			components = s.schema.Caches
		case "metrics":
			components = s.schema.Metrics
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
				content := fmt.Sprintf("# Field: %s (%s)\n-----------------------------\n%s\n", fs.Name, fs.Type, fs.Description)
				// if len(fs.Examples) > 0 {
				// 	content += fmt.Sprintf("-----------------------------\nExample:\n%s", fs.Examples[0])
				// }
				return &protocol.Hover{Contents: content}, nil
			case cs.Name:
				content := fmt.Sprintf("# Field: %s (%s)\n-----------------------------\n%s\n", cs.Name, cs.Type, cs.Description)
				// if len(cs.Examples) > 0 {
				// 	content += fmt.Sprintf("-----------------------------\nExample:\n%s", cs.Examples[0])
				// }
				return &protocol.Hover{Contents: content}, nil
			}
		}
	}

	return &protocol.Hover{Contents: ""}, nil
}
