package translator_test

import (
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestTriviaPropagation is the end-to-end test for comment + blank-line
// preservation. For each case we run Migrate over a V1 source and assert
// the V2 output contains the expected formatting.
func TestTriviaPropagation(t *testing.T) {
	cases := []struct {
		name  string
		v1    string
		wants []string // substrings required in V2 output, in order
	}{
		{
			name: "file-leading comment",
			v1: `# top comment
root.a = this.a
`,
			wants: []string{"# top comment\n", "output.a = input?.a"},
		},
		{
			name: "comment between statements",
			v1: `root.a = this.a
# why B
root.b = this.b
`,
			wants: []string{"output.a = input?.a", "# why B\n", "output.b = input?.b"},
		},
		{
			name:  "trailing comment on same line",
			v1:    `root.a = this.a  # inline reason` + "\n",
			wants: []string{"output.a = input?.a  # inline reason"},
		},
		{
			name: "blank line between statements",
			v1: `root.a = this.a

root.b = this.b
`,
			wants: []string{"output.a = input?.a\n\noutput.b = input?.b"},
		},
		{
			name: "comment block + blank + comment",
			v1: `# section A
root.a = this.a

# section B
root.b = this.b
`,
			wants: []string{"# section A\n", "output.a = input?.a", "# section B\n", "output.b = input?.b"},
		},
		{
			name: "comment inside map body is preserved",
			v1: `map double {
  # the core math
  let doubled = this * 2
  root = $doubled
}
root.x = 21.apply("double")
`,
			wants: []string{"# the core math\n", "$doubled = in * 2"},
		},
		{
			name: "comment before import is preserved",
			v1: `# used for helpers
import "helpers.blobl"
root.x = "hi"
`,
			wants: []string{"# used for helpers\n", `import "helpers.blobl"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := translator.Migrate(tc.v1, translator.Options{MinCoverage: 0})
			if err != nil {
				t.Fatalf("Migrate: %v", err)
			}
			out := rep.V2Mapping
			// Check each required substring appears in order.
			idx := 0
			for _, want := range tc.wants {
				j := strings.Index(out[idx:], want)
				if j < 0 {
					t.Fatalf("V2 output missing %q (or out of order).\nOutput:\n%s", want, out)
				}
				idx += j + len(want)
			}
		})
	}
}

// TestTriviaMultipleBlankLinesCollapse — collapsing many blank lines into one.
func TestTriviaMultipleBlankLinesCollapse(t *testing.T) {
	v1 := `root.a = this.a



root.b = this.b
`
	rep, err := translator.Migrate(v1, translator.Options{MinCoverage: 0})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Expect exactly one blank line between the two statements (three \n in a row:
	// trailing \n from stmt 1, blank-line marker \n, then output.b line).
	if !strings.Contains(rep.V2Mapping, "output.a = input?.a\n\noutput.b = input?.b") {
		t.Errorf("expected exactly one blank line between stmts, got:\n%s", rep.V2Mapping)
	}
	// And NOT two blank lines.
	if strings.Contains(rep.V2Mapping, "output.a = input?.a\n\n\noutput.b") {
		t.Errorf("expected blank lines to be collapsed, got:\n%s", rep.V2Mapping)
	}
}
