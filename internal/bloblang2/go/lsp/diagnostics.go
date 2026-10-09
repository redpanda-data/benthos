package lsp

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

// diagnose runs the parse+resolve pipeline and publishes diagnostics.
func (s *Server) diagnose(uri string) {
	text, _, ok := s.docs.get(uri)
	if !ok {
		return
	}

	var diagnostics []diagnostic

	prog, parseErrs := syntax.Parse(text, "", nil)
	if len(parseErrs) > 0 {
		for _, e := range parseErrs {
			diagnostics = append(diagnostics, posErrorToDiagnostic(e))
		}
		s.sendNotification("textDocument/publishDiagnostics", publishDiagnosticsParams{
			URI:         uri,
			Diagnostics: diagnostics,
		})
		return
	}

	syntax.Optimize(prog)

	resolveErrs := syntax.Resolve(prog, syntax.ResolveOptions{
		Methods:   s.stdlibMethods,
		Functions: s.stdlibFunctions,
	})
	for _, e := range resolveErrs {
		diagnostics = append(diagnostics, posErrorToDiagnostic(e))
	}

	// Store the program for completion use (even if there are resolve errors,
	// the parse was successful so we have a valid AST).
	s.docs.setProgram(uri, prog)

	if diagnostics == nil {
		diagnostics = []diagnostic{}
	}

	s.sendNotification("textDocument/publishDiagnostics", publishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diagnostics,
	})
}

func posErrorToDiagnostic(e syntax.PosError) diagnostic {
	// Pos is 1-based; LSP positions are 0-based.
	line := e.Pos.Line - 1
	col := e.Pos.Column - 1
	if line < 0 {
		line = 0
	}
	if col < 0 {
		col = 0
	}
	return diagnostic{
		Range: lspRange{
			Start: position{Line: line, Character: col},
			End:   position{Line: line, Character: col},
		},
		Severity: severityError,
		Source:   "bloblang2",
		Message:  e.Msg,
	}
}
