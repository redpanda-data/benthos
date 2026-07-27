// Copyright 2026 Redpanda Data, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package translator_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	v1blobl "github.com/redpanda-data/benthos/v4/public/bloblang"
	bloblangv2 "github.com/redpanda-data/benthos/v4/public/bloblangv2"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestMergeDeepDualEngine covers the object-.merge rewrite (V1 deep merge →
// V2 .merge_deep) on shapes where the two semantics AGREE: nested-object
// recursion and disjoint keys. Scalar collisions (where V1 combines into an
// array and .merge_deep replaces) are covered by the divergence test below.
func TestMergeDeepDualEngine(t *testing.T) {
	runDualCases(t, []dualCase{
		{
			// V1 deep merge and V2 .merge_deep() agree exactly when every
			// collision is object-vs-object (recursion) and leaf keys are
			// disjoint — V1's collision-into-array behaviour never fires.
			name: "nested objects recurse identically on disjoint leaves",
			v1:   `root = this.a.merge(this.b)`,
			in: map[string]any{
				"a": map[string]any{"cfg": map[string]any{"port": 8080.0}},
				"b": map[string]any{"cfg": map[string]any{"host": "remote"}},
			},
		},
		{
			name: "disjoint keys union",
			v1:   `root = this.a.merge(this.b)`,
			in: map[string]any{
				"a": map[string]any{"x": 1.0},
				"b": map[string]any{"y": 2.0},
			},
		},
		{
			name: "deep recursion at two levels disjoint leaves",
			v1:   `root = this.a.merge(this.b)`,
			in: map[string]any{
				"a": map[string]any{"o": map[string]any{"p": map[string]any{"d": 2.0}}},
				"b": map[string]any{"o": map[string]any{"p": map[string]any{"c": 9.0}}},
			},
		},
	})
}

// TestMergeScalarCollisionDiverges documents the residual divergence: V1
// combines colliding non-object values into an array; V2 .merge_deep()
// replaces with the argument's value. The migrator must flag it, the migrated
// mapping must still run, and the engines must genuinely differ on the
// documented input (so this case cannot rot into a false alarm).
func TestMergeScalarCollisionDiverges(t *testing.T) {
	const v1src = `root = this.a.merge(this.b)`
	divergesOn := map[string]any{
		"a": map[string]any{"k": 1.0},
		"b": map[string]any{"k": 2.0},
	}

	rep, err := translator.Migrate(context.Background(), v1src, translator.Options{
		Mode: translator.ModeMapping, MinCoverage: 0,
	})
	var cerr *translator.CoverageError
	if errors.As(err, &cerr) {
		rep = cerr.Report
	} else if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 1. The rewrite targets .merge_deep and warns about the collision hazard.
	if !strings.Contains(rep.V2Mapping, ".merge_deep(") {
		t.Errorf("expected .merge_deep( in migrated mapping; got:\n%s", rep.V2Mapping)
	}
	warned := false
	for _, c := range rep.Changes {
		if c.Severity >= translator.SeverityWarning && strings.Contains(c.Explanation, "collision") {
			warned = true
			break
		}
	}
	if !warned {
		t.Errorf("expected a Warning naming the collision divergence; got:\n%s", changeList(rep.Changes))
	}

	// 2. Migrated V2 still compiles and runs.
	v2ex, err := bloblangv2.GlobalEnvironment().Parse(rep.V2Mapping)
	if err != nil {
		t.Fatalf("migrated V2 does not compile: %v\n%s", err, rep.V2Mapping)
	}
	gotV2, err := v2ex.Query(divergesOn)
	if err != nil {
		t.Fatalf("migrated V2 exec: %v", err)
	}

	// 3. The engines genuinely diverge on this input.
	v1ex, err := v1blobl.Parse(v1src)
	if err != nil {
		t.Fatalf("V1 parse: %v", err)
	}
	gotV1, err := v1ex.Query(divergesOn)
	if err != nil {
		t.Fatalf("V1 exec: %v", err)
	}
	if jsonEqual(gotV1, gotV2) {
		t.Errorf("expected V1 and V2 to diverge on scalar collision (V1 arrayifies, V2 replaces); both produced %v — if semantics converged, reconsider the flag", gotV1)
	}
}

// TestAssignRewrite covers the V1 .assign() rewrite: object receivers map
// exactly to V2 .merge_deep() (deep merge, argument wins on collision);
// statically-array shapes map to .concat().
func TestAssignRewrite(t *testing.T) {
	t.Run("dual engine", func(t *testing.T) {
		runDualCases(t, []dualCase{
			{
				// V1 .assign replaces on scalar collision while recursing into
				// nested objects — exactly V2 .merge_deep semantics.
				name: "object assign with nested collision",
				v1:   `root = this.a.assign(this.b)`,
				in: map[string]any{
					"a": map[string]any{"n": map[string]any{"x": 1.0, "y": 2.0}},
					"b": map[string]any{"n": map[string]any{"y": 9.0, "z": 3.0}},
				},
			},
			{
				name: "array assign concatenates",
				v1:   `root = [1, 2].assign([3])`,
				in:   map[string]any{},
			},
		})
	})

	t.Run("object assign emits merge_deep with array caveat", func(t *testing.T) {
		rep := migrateForTest(t, `root = this.a.assign(this.b)`)
		if !strings.Contains(rep.V2Mapping, ".merge_deep(") {
			t.Errorf("expected .merge_deep( in migrated mapping; got:\n%s", rep.V2Mapping)
		}
		if !hasWarningContaining(rep, "merge_deep") {
			t.Errorf("expected a Warning mentioning merge_deep; got:\n%s", changeList(rep.Changes))
		}
	})
}

// TestMergeArrayDispatch covers the V1 .merge() array-family dispatch:
// array+array → .concat (exact), array+non-array-literal → .append (exact,
// V1 appends single values), partially-known shapes → .concat + Warning.
func TestMergeArrayDispatch(t *testing.T) {
	t.Run("dual engine", func(t *testing.T) {
		runDualCases(t, []dualCase{
			{
				name: "array merge array concatenates",
				v1:   `root = [1, 2].merge([3, 4])`,
				in:   map[string]any{},
			},
			{
				name: "array merge scalar appends",
				v1:   `root = [1, 2].merge("x")`,
				in:   map[string]any{},
			},
			{
				name: "array merge object literal appends",
				v1:   `root = [1, 2].merge({"k": 1})`,
				in:   map[string]any{},
			},
		})
	})

	t.Run("array receiver with dynamic arg warns about append", func(t *testing.T) {
		rep := migrateForTest(t, `root = [1, 2].merge(this.x)`)
		if !strings.Contains(rep.V2Mapping, ".concat(") {
			t.Errorf("expected .concat( in migrated mapping; got:\n%s", rep.V2Mapping)
		}
		if !hasWarningContaining(rep, "APPENDS") {
			t.Errorf("expected a Warning about V1's append behaviour; got:\n%s", changeList(rep.Changes))
		}
	})

	t.Run("dynamic receiver with array arg warns about object no-op", func(t *testing.T) {
		rep := migrateForTest(t, `root = this.r.merge([1, 2])`)
		if !strings.Contains(rep.V2Mapping, ".concat(") {
			t.Errorf("expected .concat( in migrated mapping; got:\n%s", rep.V2Mapping)
		}
		if !hasWarningContaining(rep, "no-op") {
			t.Errorf("expected a Warning about V1's object-receiver no-op; got:\n%s", changeList(rep.Changes))
		}
	})
}

// TestCatchNamedLambda covers the V1 .catch(fallback: <lambda>) form: V1's
// parameter is named "fallback" but V2's is "fn", so the migrator must not
// pass the name through (the migrated call would not compile).
func TestCatchNamedLambda(t *testing.T) {
	rep := migrateForTest(t, `root.v = this.n.number().catch(fallback: err -> "fell")`)
	if strings.Contains(rep.V2Mapping, "fallback") {
		t.Errorf("V1 arg name 'fallback' must not survive into V2; got:\n%s", rep.V2Mapping)
	}
	runDualCases(t, []dualCase{
		{
			name: "named lambda catch",
			v1:   `root.v = this.n.number().catch(fallback: err -> "fell")`,
			in:   map[string]any{"n": "not-a-number"},
		},
	})
}

// migrateForTest migrates a V1 mapping, tolerating coverage errors.
func migrateForTest(t *testing.T, v1 string) *translator.Report {
	t.Helper()
	rep, err := translator.Migrate(context.Background(), v1, translator.Options{MinCoverage: 0})
	var cerr *translator.CoverageError
	switch {
	case err == nil:
	case errors.As(err, &cerr):
		rep = cerr.Report
	default:
		t.Fatalf("Migrate(%q) failed: %v", v1, err)
	}
	return rep
}

// hasWarningContaining reports whether any Warning-or-higher change's
// explanation contains the given substring.
func hasWarningContaining(rep *translator.Report, sub string) bool {
	for _, c := range rep.Changes {
		if c.Severity >= translator.SeverityWarning && strings.Contains(c.Explanation, sub) {
			return true
		}
	}
	return false
}
