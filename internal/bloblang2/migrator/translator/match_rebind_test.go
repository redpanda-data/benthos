package translator_test

import (
	"context"
	"testing"

	v1blobl "github.com/redpanda-data/benthos/v4/public/bloblang"
	bloblangv2 "github.com/redpanda-data/benthos/v4/public/bloblangv2"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestMatchSubjectRebind is the regression for the match this-rebind bug: V1
// `match <subject> { … }` rebinds `this` to the subject inside case patterns
// AND bodies. The translator must reproduce that (V2 `match … as __match`),
// or the migrated mapping silently returns the wrong value / errors at runtime.
// Dual-engine (real V1 vs real V2 GlobalEnvironment) so both are ground truth.
func TestMatchSubjectRebind(t *testing.T) {
	cases := []struct {
		name string
		v1   string
		in   any
	}{
		{
			// `this` in the body denotes the subject (an object here, so no
			// field-access-strictness confounder).
			name: "this-in-body, object subject",
			v1:   `root.x = match this.p { _ => this.name }`,
			in:   map[string]any{"p": map[string]any{"name": "Bob", "kind": "vip"}},
		},
		{
			// Equality-value pattern + `this` (the scalar subject) in body.
			name: "equality pattern + this method in body",
			v1:   `root.x = match this.kind { "vip" => this.uppercase(), _ => "?" }`,
			in:   map[string]any{"kind": "vip"},
		},
		{
			// Predicate pattern on the subject — V1 evaluates it with
			// this=subject; V2 needs `__match > 10`.
			name: "predicate pattern on subject",
			v1:   `root.y = match this.score { this > 10 => "big", _ => "small" }`,
			in:   map[string]any{"score": float64(42)},
		},
		{
			// this-free match keeps the simpler equality form (no `as`).
			name: "this-free equality match",
			v1:   `root.z = match this.kind { "a" => "A", "b" => "B", _ => "?" }`,
			in:   map[string]any{"kind": "b"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := translator.Migrate(context.Background(), tc.v1, translator.Options{Mode: translator.ModeMapping, MinCoverage: 0})
			if err != nil {
				t.Fatalf("Migrate: %v", err)
			}
			ex, err := bloblangv2.GlobalEnvironment().Parse(rep.V2Mapping)
			if err != nil {
				t.Fatalf("V2 compile: %v\n%s", err, rep.V2Mapping)
			}
			gotV2, err := ex.Query(tc.in)
			if err != nil {
				t.Fatalf("V2 exec: %v\n%s", err, rep.V2Mapping)
			}
			v1ex, err := v1blobl.Parse(tc.v1)
			if err != nil {
				t.Fatalf("V1 parse: %v", err)
			}
			gotV1, err := v1ex.Query(tc.in)
			if err != nil {
				t.Fatalf("V1 exec: %v", err)
			}
			if !jsonEqual(gotV1, gotV2) {
				t.Fatalf("V1/V2 mismatch:\n  V1: %v\n  V2: %v\nV2:\n%s", gotV1, gotV2, rep.V2Mapping)
			}
		})
	}
}
