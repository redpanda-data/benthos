package lsp

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/goccy/go-json"
	"github.com/goccy/go-yaml"

	_ "github.com/tliron/commonlog/simple"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func Decode(v string) ([]byte, error) {
	var ret []string
	dec := yaml.NewDecoder(strings.NewReader(v))
	for {
		var v any
		if err := dec.Decode(&v); err != nil {
			if err == io.EOF {
				break
			}
			return nil, errors.New(yaml.FormatError(err, true, true))
		}
		got, err := json.MarshalIndentWithOption(v, "", "  ", json.Colorize(json.DefaultColorScheme))
		if err != nil {
			return nil, err
		}
		ret = append(ret, string(got))
	}
	return []byte(strings.Join(ret, "\n")), nil
}

func TextDocumentDidOpen(context *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	fmt.Printf("opened: %s", params.TextDocument.URI)
	return nil
}

func TextDocumentDidClose(context *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	fmt.Printf("closed %s", params.TextDocument.URI)
	return nil
}

func TextDocumentDidChange(context *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	fmt.Printf("did change: %s", params.TextDocument.URI)
	return nil
}

func TextDocumentCompletion(ctx *glsp.Context, params *protocol.CompletionParams) (any, error) {
	var completionItems []protocol.CompletionItem

	// filePath := params.TextDocumentPositionParams.TextDocument.URI
	// position := params.TextDocumentPositionParams.Position

	// FOR AUTOCOMPLETE RP Connect
	for word, entry := range rpConnectMapper {
		term := entry.Term
		description := entry.Description
		detail := fmt.Sprintf("%s\n%s", term, description)
		completionItems = append(completionItems, protocol.CompletionItem{
			Label:      word,
			Detail:     &detail,
			InsertText: &term,
		})
	}
	for label, snippet := range CodeSnippets {
		insertText := snippet.Body
		detail := snippet.Description
		kind := protocol.CompletionItemKindSnippet
		textFormat := protocol.InsertTextFormatSnippet

		completionItems = append(completionItems, protocol.CompletionItem{
			Label:            label,
			Detail:           &detail,
			InsertText:       &insertText,
			Kind:             &kind,
			InsertTextFormat: &textFormat,
		})
	}

	return completionItems, nil
}
