package lsp

import (
	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

type textDocumentHover struct {
	opts *common.CLIOpts
	s    *state
}

func newTextDocumentHover(opts *common.CLIOpts, state *state) *textDocumentHover {
	return &textDocumentHover{
		opts: opts,
		s:    state,
	}
}

func (t *textDocumentHover) onHover(context *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	h := &protocol.Hover{Contents: "Booper"}
	return h, nil
}
