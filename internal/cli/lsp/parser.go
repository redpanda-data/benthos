package lsp

import (
	// "go/token"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"
)

func findTokenAtPosition(file *ast.File, line, column int) *token.Token {
	var result *token.Token

	// Walk each document in the file
	for _, doc := range file.Docs {
		ast.Walk(&visitor{
			targetLine:   line,
			targetColumn: column,
			result:       &result,
		}, doc.Body)

		if result != nil {
			break
		}
	}

	return result
}

type visitor struct {
	targetLine   int
	targetColumn int
	result       **token.Token
}

func (v *visitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	// Get the token from the node
	tok := node.GetToken()
	if tok != nil {
		pos := tok.Position

		// Check if this token contains our target position
		if pos.Line == v.targetLine {
			// Check if the column is within the token's range
			startCol := pos.Column
			endCol := startCol + len(tok.Value)

			if v.targetColumn >= startCol && v.targetColumn < endCol {
				*v.result = tok
			}
		}
	}

	return v
}
