package lsp

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TextDocumentHover(context *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	h := &protocol.Hover{Contents: "Booper"}
	return h, nil
}
