package lsp

import (
	"fmt"

	_ "github.com/tliron/commonlog/simple"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TextDocumentCompletion(_ *glsp.Context, params *protocol.CompletionParams) (any, error) {
	var completionItems []protocol.CompletionItem

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
