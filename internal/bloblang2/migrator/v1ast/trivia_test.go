package v1ast_test

import (
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

func TestScannerEmitsCommentTokens(t *testing.T) {
	src := "root.a = 1  # trail\n# standalone\nroot.b = 2\n"
	toks, err := v1ast.NewScanner(src).All()
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	var comments []string
	for _, tok := range toks {
		if tok.Kind == v1ast.TokComment {
			comments = append(comments, tok.Text)
		}
	}
	want := []string{" trail", " standalone"}
	if len(comments) != len(want) {
		t.Fatalf("expected %d comments, got %d: %v", len(want), len(comments), comments)
	}
	for i, c := range comments {
		if c != want[i] {
			t.Errorf("comment[%d] = %q, want %q", i, c, want[i])
		}
	}
}

func TestParserAttachesLeadingTrivia(t *testing.T) {
	src := `# file header
# second line

# after blank line
root.x = 1
`
	prog, err := v1ast.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("want 1 stmt, got %d", len(prog.Stmts))
	}
	tri := prog.Stmts[0].Trivia()
	var kinds []string
	for _, t := range tri.Leading {
		switch t.Kind {
		case v1ast.TriviaComment:
			kinds = append(kinds, "comment:"+strings.TrimSpace(t.Text))
		case v1ast.TriviaBlankLine:
			kinds = append(kinds, "blank")
		}
	}
	want := []string{"comment:file header", "comment:second line", "blank", "comment:after blank line"}
	if len(kinds) != len(want) {
		t.Fatalf("leading trivia = %v, want %v", kinds, want)
	}
	for i := range kinds {
		if kinds[i] != want[i] {
			t.Errorf("kinds[%d] = %s, want %s", i, kinds[i], want[i])
		}
	}
}

func TestParserAttachesTrailingComment(t *testing.T) {
	src := "root.x = 1  # why\n"
	prog, err := v1ast.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("want 1 stmt, got %d", len(prog.Stmts))
	}
	tri := prog.Stmts[0].Trivia()
	if len(tri.Trailing) != 1 {
		t.Fatalf("want 1 trailing, got %d", len(tri.Trailing))
	}
	if tri.Trailing[0].Kind != v1ast.TriviaComment || strings.TrimSpace(tri.Trailing[0].Text) != "why" {
		t.Errorf("trailing = %+v", tri.Trailing[0])
	}
}

func TestParserCollectsTriviaInsideMapBody(t *testing.T) {
	src := `map process {
  # inside map
  root.x = this.x

  # after blank
  root.y = this.y  # trail
}
`
	prog, err := v1ast.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prog.Maps) != 1 {
		t.Fatalf("want 1 map, got %d", len(prog.Maps))
	}
	body := prog.Maps[0].Body
	if len(body) != 2 {
		t.Fatalf("want 2 body stmts, got %d", len(body))
	}
	firstLeading := body[0].Trivia().Leading
	if len(firstLeading) != 1 || firstLeading[0].Kind != v1ast.TriviaComment {
		t.Errorf("first stmt leading = %+v", firstLeading)
	}
	secondLeading := body[1].Trivia().Leading
	if len(secondLeading) != 2 {
		t.Fatalf("second stmt leading = %+v (want blank + comment)", secondLeading)
	}
	if secondLeading[0].Kind != v1ast.TriviaBlankLine {
		t.Errorf("want blank line first, got %+v", secondLeading[0])
	}
	if secondLeading[1].Kind != v1ast.TriviaComment {
		t.Errorf("want comment second, got %+v", secondLeading[1])
	}
	secondTrailing := body[1].Trivia().Trailing
	if len(secondTrailing) != 1 || strings.TrimSpace(secondTrailing[0].Text) != "trail" {
		t.Errorf("second stmt trailing = %+v", secondTrailing)
	}
}

func TestParserCollapsesMultipleBlankLines(t *testing.T) {
	src := "root.a = 1\n\n\n\nroot.b = 2\n"
	prog, err := v1ast.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prog.Stmts) != 2 {
		t.Fatalf("want 2 stmts, got %d", len(prog.Stmts))
	}
	leading := prog.Stmts[1].Trivia().Leading
	var blanks int
	for _, t := range leading {
		if t.Kind == v1ast.TriviaBlankLine {
			blanks++
		}
	}
	if blanks != 1 {
		t.Errorf("want 1 collapsed blank-line trivia, got %d (full leading: %+v)", blanks, leading)
	}
}
