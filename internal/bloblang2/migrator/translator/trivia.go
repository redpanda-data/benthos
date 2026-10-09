package translator

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

// Every V1 statement carries a TriviaSet (comments + blank lines collected
// by the V1 parser). The translator copies that set onto the V2 node it
// produces so V2 output preserves the author's prose and pacing.
//
// Rules for edge cases:
//   - When a V1 statement is dropped (translateStmt returns nil), its
//     trivia is lost. A future pass can hoist it onto the next surviving
//     statement if this becomes a pain point.
//   - When a V1 statement expands into multiple V2 statements, leading
//     should attach to the first emitted V2 node and trailing to the last.
//     No current rule expands 1:N, but `attachLeadingTo` / `attachTrailingTo`
//     helpers exist for when that arrives.
//   - Synthesised V2 statements (e.g. ModeMapping's `output = input`
//     prelude) get no V1 trivia.

// triviaKindFromV1 maps V1 trivia kinds to V2.
func triviaKindFromV1(k v1ast.TriviaKind) syntax.TriviaKind {
	if k == v1ast.TriviaBlankLine {
		return syntax.TriviaBlankLine
	}
	return syntax.TriviaComment
}

// convertTriviaList clones a V1 trivia slice into V2 form.
func convertTriviaList(in []v1ast.Trivia) []syntax.Trivia {
	if len(in) == 0 {
		return nil
	}
	out := make([]syntax.Trivia, len(in))
	for i, t := range in {
		out[i] = syntax.Trivia{
			Kind: triviaKindFromV1(t.Kind),
			Text: t.Text,
			Pos:  syntax.Pos{Line: t.Pos.Line, Column: t.Pos.Column},
		}
	}
	return out
}

// copyTrivia copies the V1 statement's leading+trailing trivia onto the
// V2 statement. Safe to call with a nil V2 — no-op in that case.
func copyTrivia(src v1ast.Stmt, dst syntax.Stmt) {
	if dst == nil {
		return
	}
	srcTri := src.Trivia()
	dstTri := dst.Trivia()
	dstTri.Leading = append(dstTri.Leading, convertTriviaList(srcTri.Leading)...)
	dstTri.Trailing = append(dstTri.Trailing, convertTriviaList(srcTri.Trailing)...)
}

// copyTriviaTo copies trivia onto any node exposing a Trivia() accessor.
// Used when the V2 target is not a Stmt (e.g. VarAssign in an ExprBody, or
// a MapDecl exposed via prog.Maps).
func copyTriviaTo(src v1ast.Stmt, dst interface{ Trivia() *syntax.TriviaSet }) {
	if dst == nil {
		return
	}
	srcTri := src.Trivia()
	dstTri := dst.Trivia()
	dstTri.Leading = append(dstTri.Leading, convertTriviaList(srcTri.Leading)...)
	dstTri.Trailing = append(dstTri.Trailing, convertTriviaList(srcTri.Trailing)...)
}
