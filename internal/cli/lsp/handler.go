package lsp

import (
	"fmt"

	_ "github.com/tliron/commonlog/simple"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func textDocumentDidChange(context *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	fmt.Printf("did change: %s", params.TextDocument.URI)
	return nil
}

func textDocumentCompletion(ctx *glsp.Context, params *protocol.CompletionParams) (any, error) {
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
