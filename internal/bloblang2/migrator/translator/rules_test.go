package translator_test

import (
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestRuleUnits — Layer 2. Each entry documents one translation rule with a
// representative V1 input, the expected V2 substring in the output, and the
// expected Change fingerprint (a subset-match on RuleID / Severity).
//
// The tests are deliberately specific: they assert the RuleID emitted rather
// than the exact wording of the Explanation, so wording tweaks don't break
// the tests. V2 output is substring-matched to keep the tests insensitive to
// whitespace / layout drift.
func TestRuleUnits(t *testing.T) {
	for _, c := range []struct {
		name    string
		v1      string
		wantV2  []string // all substrings must appear in the V2 text
		wantRule translator.RuleID
	}{
		{
			name:     "root -> output (no change recorded; this is the identity rename)",
			v1:       `root = "hi"`,
			wantV2:   []string{"output", `"hi"`},
			wantRule: 0,
		},
		{
			name:     "this -> input",
			v1:       `root = this`,
			wantV2:   []string{"output", "input"},
			wantRule: translator.RuleThisToInput,
		},
		{
			name:     "bare ident -> input.ident",
			v1:       `root = foo`,
			wantV2:   []string{"input.foo"},
			wantRule: translator.RuleBareIdentToInput,
		},
		{
			name:     "bare path target -> output.path",
			v1:       `foo.bar = 1`,
			wantV2:   []string{"output.foo.bar"},
			wantRule: translator.RuleBarePathToOutput,
		},
		{
			name:     "this-target -> output",
			v1:       `this.foo = "bar"`,
			wantV2:   []string{"output.foo"},
			wantRule: translator.RuleThisTargetToOutput,
		},
		{
			name:     "meta target",
			v1:       `meta foo = "bar"`,
			wantV2:   []string{"output@", "foo"},
			wantRule: translator.RuleMetaTargetToOutputMeta,
		},
		{
			name:     "coalesce | -> .or()",
			v1:       `root = this.x | "fb"`,
			wantV2:   []string{".or(", `"fb"`},
			wantRule: translator.RuleCoalescePrecedence,
		},
		{
			name:     "method rename: map_each -> map",
			v1:       `root = [1,2,3].map_each(x -> x)`,
			wantV2:   []string{".map(x -> x)"},
			wantRule: 0,
		},
		{
			name:     "method rename: .index(n) -> [n]",
			v1:       `root = this.items.index(0)`,
			wantV2:   []string{"[0]"},
			wantRule: translator.RuleNoBracketIndexing,
		},
		{
			name:     ".apply('name') -> name(recv)",
			v1:       "map double { root = this * 2 }\nroot.v = (5).apply(\"double\")",
			wantV2:   []string{"double(", "map double(in)"},
			wantRule: translator.RuleMapDeclTranslation,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			rep, err := translator.Migrate(c.v1, translator.Options{Verbose: true})
			if err != nil {
				t.Fatalf("Migrate(%q) failed: %v", c.v1, err)
			}
			for _, want := range c.wantV2 {
				if !strings.Contains(rep.V2Mapping, want) {
					t.Errorf("V2 output missing %q\nGot:\n%s", want, rep.V2Mapping)
				}
			}
			if c.wantRule != 0 {
				if !hasRule(rep.Changes, c.wantRule) {
					t.Errorf("expected a Change with RuleID %s; got:\n%s", c.wantRule, changeList(rep.Changes))
				}
			}
		})
	}
}

// hasRule reports whether any change in the slice has the given RuleID.
func hasRule(changes []translator.Change, id translator.RuleID) bool {
	for _, c := range changes {
		if c.RuleID == id {
			return true
		}
	}
	return false
}

// changeList returns a human-readable summary of a Change slice for failing
// test output.
func changeList(changes []translator.Change) string {
	var out strings.Builder
	for _, c := range changes {
		out.WriteString("  - ")
		out.WriteString(c.RuleID.String())
		out.WriteString(" (")
		out.WriteString(c.Severity.String())
		out.WriteString("): ")
		out.WriteString(c.Explanation)
		out.WriteString("\n")
	}
	return out.String()
}
