package translator_test

import (
	"context"
	"errors"
	"testing"

	v1blobl "github.com/redpanda-data/benthos/v4/public/bloblang"
	bloblangv2 "github.com/redpanda-data/benthos/v4/public/bloblangv2"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// dualCase is one V1 mapping + input for differential (V1-vs-V2) testing.
type dualCase struct {
	name string
	v1   string
	in   any
}

// runDualCases is the shared differential-testing harness: for each V1 mapping
// it migrates to V2 and asserts the migrated V2 — executed on the REAL V2
// environment (bloblangv2.GlobalEnvironment, what users actually run) —
// produces the same value as the REAL V1 engine (public/bloblang).
//
// This is the check that catches silent translation divergences (the class
// behind the mapping/mutation inversion and the match this-rebind bug). It
// migrates in ModeMapping because public/bloblang.Query starts root empty
// (MapPart / bare-mapping semantics), which ModeMapping matches (no
// `output = input` prelude); the mutation/pass-through mode is covered
// separately by TestModeRootSemantics.
//
// Cases here are ones confirmed EQUIVALENT across both engines. A case that
// legitimately diverges belongs either in a fix (if the divergence is a bug)
// or in a test that asserts the divergence is flagged — not here.
func runDualCases(t *testing.T, cases []dualCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := translator.Migrate(context.Background(), tc.v1, translator.Options{
				Mode:        translator.ModeMapping,
				MinCoverage: 0,
			})
			var cerr *translator.CoverageError
			if errors.As(err, &cerr) {
				rep = cerr.Report
			} else if err != nil {
				t.Fatalf("migrate: %v", err)
			}

			v1ex, err := v1blobl.Parse(tc.v1)
			if err != nil {
				t.Fatalf("V1 parse (test setup): %v", err)
			}
			gotV1, v1Err := v1ex.Query(tc.in)
			if v1Err != nil {
				t.Fatalf("V1 exec (pick an input V1 accepts for an equivalence case): %v", v1Err)
			}

			v2ex, err := bloblangv2.GlobalEnvironment().Parse(rep.V2Mapping)
			if err != nil {
				t.Fatalf("migrated V2 did not compile: %v\nV2:\n%s", err, rep.V2Mapping)
			}
			gotV2, v2Err := v2ex.Query(tc.in)
			if v2Err != nil {
				t.Fatalf("migrated V2 errored at exec: %v\nV2:\n%s", v2Err, rep.V2Mapping)
			}

			if !jsonEqual(gotV1, gotV2) {
				t.Fatalf("V1/V2 divergence:\n  V1: %v\n  V2: %v\nV2 mapping:\n%s", gotV1, gotV2, rep.V2Mapping)
			}
		})
	}
}

// TestDualEngineSmoke proves the harness and seeds a handful of obviously-safe
// equivalence cases. Per-area case tables (populated from the dual-engine
// sweep) live in dual_*_test.go and call runDualCases.
func TestDualEngineSmoke(t *testing.T) {
	runDualCases(t, []dualCase{
		{"field passthrough", `root.a = this.a`, map[string]any{"a": float64(1)}},
		{"string upper", `root.s = this.s.uppercase()`, map[string]any{"s": "hi"}},
		{"array map", `root.xs = this.xs.map_each(x -> x + 1)`, map[string]any{"xs": []any{float64(1), float64(2)}}},
	})
}
