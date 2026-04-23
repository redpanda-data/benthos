package translator_test

import (
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestFoldContextToTwoParam is the regression test for the GA4 case study's
// .fold lambda — V1's one-param context object becomes V2's two explicit
// (tally, value) params. The rewrite walks the V1 body, replacing
// `item.tally` and `item.value` field accesses with bare `tally`/`value`
// identifiers that resolve as lambda-param references on the V2 side.
func TestFoldContextToTwoParam(t *testing.T) {
	cases := []struct {
		name  string
		v1    string
		wants []string // substrings that must appear in V2 output, in order
	}{
		{
			name: "GA4 key-value merge pattern",
			v1: `let params = this.params.
  map_each(p -> {"key": p.key, "value": p.value}).
  fold({}, item -> item.tally.merge({(item.value.key): item.value.value}))
root.x = $params
`,
			wants: []string{
				".fold({},",
				"(tally, value) ->",
				"tally.merge(",
				"value?.key",
				"value?.value",
			},
		},
		{
			name: "integer accumulator",
			v1: `root.sum = this.items.fold(0, item -> item.tally + item.value)
`,
			wants: []string{
				".fold(0,",
				"(tally, value) ->",
				"tally + value",
			},
		},
		{
			name: "array accumulator using .merge on tally",
			v1: `root.out = this.xs.fold([], item -> item.tally.merge([item.value]))
`,
			wants: []string{
				".fold([],",
				"(tally, value) ->",
				"tally.merge([value])",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := translator.Migrate(tc.v1, translator.Options{MinCoverage: 0})
			if err != nil {
				t.Fatalf("Migrate: %v", err)
			}
			out := rep.V2Mapping
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

// TestFoldBareContextRefIsFlagged — if the V1 lambda references the
// context param directly (not via .tally/.value) the translator can't
// mechanically rewrite; it falls through and records a Warning so the
// user knows manual conversion is needed.
func TestFoldBareContextRefIsFlagged(t *testing.T) {
	v1 := `root.x = this.xs.fold({}, item -> item)
`
	rep, err := translator.Migrate(v1, translator.Options{MinCoverage: 0})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	var sawWarning bool
	for _, c := range rep.Changes {
		if strings.Contains(c.Explanation, "fold") && c.Severity == translator.SeverityWarning {
			sawWarning = true
			break
		}
	}
	if !sawWarning {
		t.Errorf("expected a Warning change referring to .fold; got changes:\n%+v", rep.Changes)
	}
}
