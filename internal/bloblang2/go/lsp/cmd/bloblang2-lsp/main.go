// bloblang2-lsp is a minimal LSP server for Bloblang V2 mappings.
package main

import (
	"os"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/lsp"
)

func main() {
	s := lsp.NewServer(os.Stdin, os.Stdout)
	if err := s.Run(); err != nil {
		os.Exit(1)
	}
}
