package translator_test

import (
	"context"
	"testing"

	v1blobl "github.com/redpanda-data/benthos/v4/public/bloblang"
	bloblangv2 "github.com/redpanda-data/benthos/v4/public/bloblangv2"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestModeRootSemantics is the regression for the mapping/mutation inversion
// (silent data corruption). It asserts the DATA behaviour end-to-end, not just
// the presence of an `output = input` prelude string — the string-only tests
// happily encoded the wrong model.
//
// Ground truth (internal/bloblang/mapping/executor.go):
//   - V1 `mapping`  → MapPart  → root starts empty (new document).
//   - V1 `mutation` → MapOnto  → root starts as the input document.
//
// So a migrated `mapping` must NOT pass unassigned input fields through
// (else it leaks data/PII), and a migrated `mutation` MUST preserve them
// (else it drops the document).
func TestModeRootSemantics(t *testing.T) {
	runV2 := func(t *testing.T, v2 string, in any) any {
		t.Helper()
		ex, err := bloblangv2.GlobalEnvironment().Parse(v2)
		if err != nil {
			t.Fatalf("V2 compile: %v\n%s", err, v2)
		}
		out, err := ex.Query(in)
		if err != nil {
			t.Fatalf("V2 exec: %v\n%s", err, v2)
		}
		return out
	}
	migrate := func(t *testing.T, v1 string, mode translator.Mode) string {
		t.Helper()
		rep, err := translator.Migrate(context.Background(), v1, translator.Options{Mode: mode, MinCoverage: 0})
		if err != nil {
			t.Fatalf("Migrate: %v", err)
		}
		return rep.V2Mapping
	}

	t.Run("mapping does not leak unassigned input fields", func(t *testing.T) {
		in := map[string]any{"id": float64(1), "secret": "leak"}
		v2 := migrate(t, `root.id = this.id`, translator.ModeMapping)
		got := runV2(t, v2, in)
		// V1 `mapping` (empty-start) yields {id:1}; `secret` must NOT survive.
		want := map[string]any{"id": float64(1)}
		if !jsonEqual(got, want) {
			t.Fatalf("mapping leaked input fields.\n got: %v\nwant: %v\nV2:\n%s", got, want, v2)
		}
		// Cross-check the empty-start semantics against the V1 engine, whose
		// bare mapping executor also starts root empty.
		v1ex, err := v1blobl.Parse(`root.id = this.id`)
		if err != nil {
			t.Fatalf("V1 parse: %v", err)
		}
		gotV1, err := v1ex.Query(in)
		if err != nil {
			t.Fatalf("V1 exec: %v", err)
		}
		if !jsonEqual(gotV1, got) {
			t.Fatalf("mapping V1/V2 mismatch.\n V1: %v\n V2: %v", gotV1, got)
		}
	})

	t.Run("mutation preserves unassigned input fields", func(t *testing.T) {
		in := map[string]any{"id": float64(1), "secret": "keep"}
		v2 := migrate(t, `root.foo = 1`, translator.ModeMutation)
		got := runV2(t, v2, in)
		// V1 `mutation` (input-start) yields the whole input plus foo.
		want := map[string]any{"id": float64(1), "secret": "keep", "foo": int64(1)}
		if !jsonEqual(got, want) {
			t.Fatalf("mutation dropped input fields.\n got: %v\nwant: %v\nV2:\n%s", got, want, v2)
		}
	})
}
