package syntax

import (
	"strings"
	"testing"
)

// Helper: build a bare output.x = input.x assignment with the given trivia.
func testAssign(name string, leading, trailing []Trivia) *Assignment {
	return &Assignment{
		TriviaSet: TriviaSet{Leading: leading, Trailing: trailing},
		Target: AssignTarget{
			Root: AssignOutput,
			Path: []PathSegment{{Kind: PathSegField, Name: name}},
		},
		Value: &PathExpr{Root: PathRootInput, Segments: []PathSegment{{Kind: PathSegField, Name: name}}},
	}
}

func TestPrintLeadingComment(t *testing.T) {
	prog := &Program{
		Stmts: []Stmt{
			testAssign("a", []Trivia{{Kind: TriviaComment, Text: " the answer"}}, nil),
		},
	}
	got := Print(prog)
	want := "# the answer\noutput.a = input.a\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrintTrailingComment(t *testing.T) {
	prog := &Program{
		Stmts: []Stmt{
			testAssign("a", nil, []Trivia{{Kind: TriviaComment, Text: " why"}}),
		},
	}
	got := Print(prog)
	want := "output.a = input.a  # why\n"
	if got != want {
		t.Errorf("got: %q, want: %q", got, want)
	}
}

func TestPrintBlankLineBetweenStatements(t *testing.T) {
	prog := &Program{
		Stmts: []Stmt{
			testAssign("a", nil, nil),
			testAssign("b", []Trivia{{Kind: TriviaBlankLine}}, nil),
		},
	}
	got := Print(prog)
	want := "output.a = input.a\n\noutput.b = input.b\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestPrintCommentBlockAndBlankBeforeStatement(t *testing.T) {
	prog := &Program{
		Stmts: []Stmt{
			testAssign("a", nil, nil),
			testAssign("b", []Trivia{
				{Kind: TriviaBlankLine},
				{Kind: TriviaComment, Text: " section B"},
			}, []Trivia{{Kind: TriviaComment, Text: " inline"}}),
		},
	}
	got := Print(prog)
	want := "output.a = input.a\n\n# section B\noutput.b = input.b  # inline\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrintCommentInsideMapBody(t *testing.T) {
	prog := &Program{
		Maps: []*MapDecl{{
			TokenPos: Pos{Line: 1, Column: 1},
			Name:     "m",
			Params:   []Param{{Name: "v"}},
			Body: &ExprBody{
				Assignments: []*VarAssign{
					{
						TriviaSet: TriviaSet{Leading: []Trivia{{Kind: TriviaComment, Text: " inside body"}}},
						Name:      "t",
						Value:     &LiteralExpr{TokenType: INT, Value: "1"},
					},
				},
				Result: &VarExpr{Name: "t"},
			},
		}},
	}
	got := Print(prog)
	// Must contain the comment indented one level inside the map body.
	if !strings.Contains(got, "  # inside body\n") {
		t.Errorf("missing indented comment in:\n%s", got)
	}
	if !strings.Contains(got, "$t = 1") {
		t.Errorf("missing var assign in:\n%s", got)
	}
}
