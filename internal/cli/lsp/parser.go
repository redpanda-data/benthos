package lsp

import (
	"fmt"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"
)

// TokenWithPath holds a token and its path in the YAML document
type TokenWithPath struct {
	Token *token.Token
	Path  string
}

func findTokenAtPosition(file *ast.File, line, column int) (*TokenWithPath, error) {
	var result *TokenWithPath

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

	if result != nil {
		return result, nil
	}

	return nil, fmt.Errorf("token not found at line %d, column %d", line, column)
}

type visitor struct {
	targetLine   int
	targetColumn int
	result       **TokenWithPath
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
				*v.result = &TokenWithPath{
					Token: tok,
					Path:  node.GetPath(),
				}
			}
		}
	}

	return v
}
