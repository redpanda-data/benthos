package benchmark_test

import (
	"reflect"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/benchmark"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
	"github.com/redpanda-data/benthos/v4/public/bloblangv2"

	// Side-effect: register the public-bloblangv2 plugins so V2 sees
	// format/with/zip/find_by/etc. through Environment.
	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

// TestMigrationSmoke runs a small set of representative V1 mappings end
// to end: V1 source -> migrator -> V2 source, then executes both sides
// against the same input and asserts the outputs match. The cases
// deliberately exercise the recently-added translator rules (variadic
// methods, timestamp idiom shifts, query-form predicates, format
// rewrite) so a regression in any of those would surface here as a
// gating failure rather than as a quietly skipped corpus row.
//
// The V1 path runs through the public bloblang environment via the
// benchmark harness's V1Runner. The V2 path runs through
// bloblangv2.GlobalEnvironment so the public plugin registry (compress,
// with, zip, format, find_by, …) is in scope. Cases that depend on a
// bound message (batch_index, content, error, tracing_*) aren't
// covered here — they need a different harness because the V2 path is
// only reachable via Executor.QueryMessage.
func TestMigrationSmoke(t *testing.T) {
	type smokeCase struct {
		name string
		v1   string
		// input + meta drive both V1 and V2 evaluation.
		input any
		meta  map[string]any
		// wantV1 / wantV2 are checked independently. When wantV2 is
		// nil it falls back to wantV1 (the common case where the two
		// engines agree byte-for-byte).
		wantV1 any
		wantV2 any
	}

	cases := []smokeCase{
		{
			name:   "format method (V1 variadic -> V2 array arg)",
			v1:     `root.s = "%s/%v".format(this.name, this.age)`,
			input:  map[string]any{"name": "lance", "age": int64(37)},
			wantV1: map[string]any{"s": "lance/37"},
		},
		{
			name:   "with method (V1 variadic -> V2 array arg)",
			v1:     `root = this.with("inner.a", "d")`,
			input:  map[string]any{"inner": map[string]any{"a": "first", "b": "second"}, "d": "fourth", "e": "fifth"},
			wantV1: map[string]any{"d": "fourth", "inner": map[string]any{"a": "first"}},
		},
		{
			name:   "zip method (V1 variadic -> V2 array arg)",
			v1:     `root.foo = this.foo.zip(this.bar, this.baz)`,
			input:  map[string]any{"foo": []any{"a", "b"}, "bar": []any{int64(1), int64(2)}, "baz": []any{int64(4), int64(5)}},
			wantV1: map[string]any{"foo": []any{[]any{"a", int64(1), int64(4)}, []any{"b", int64(2), int64(5)}}},
		},
		{
			name:   "without method (V1 variadic -> V2 array arg)",
			v1:     `root = this.without("b")`,
			input:  map[string]any{"a": int64(1), "b": int64(2), "c": int64(3)},
			wantV1: map[string]any{"a": int64(1), "c": int64(3)},
		},
		{
			// V1 find_by returns Go's int; V2 stores it as int64. The
			// integer value is the same, but reflect.DeepEqual is
			// type-strict — assert the typed value per engine.
			name:   "find_by query-form -> V2 explicit lambda",
			v1:     `root.idx = this.items.find_by(this.id == 2)`,
			input:  map[string]any{"items": []any{map[string]any{"id": int64(1)}, map[string]any{"id": int64(2)}, map[string]any{"id": int64(3)}}},
			wantV1: map[string]any{"idx": 1},
			wantV2: map[string]any{"idx": int64(1)},
		},
		{
			name:   "filter query-form -> V2 explicit lambda",
			v1:     `root = this.nums.filter(this > 2)`,
			input:  map[string]any{"nums": []any{int64(1), int64(2), int64(3), int64(4)}},
			wantV1: []any{int64(3), int64(4)},
		},
		{
			name:   "ts_strftime method renamed to ts_format",
			v1:     `root.iso = this.t.parse_timestamp_strptime("%Y-%m-%dT%H:%M:%SZ").ts_strftime("%Y-%m-%d")`,
			input:  map[string]any{"t": "2020-08-14T05:54:23Z"},
			wantV1: map[string]any{"iso": "2020-08-14"},
		},
		{
			name:   "ts.format_timestamp_unix() -> ts.ts_unix()",
			v1:     `root.epoch = this.t.parse_timestamp_strptime("%Y-%m-%dT%H:%M:%SZ").format_timestamp_unix()`,
			input:  map[string]any{"t": "2020-08-14T05:54:23Z"},
			wantV1: map[string]any{"epoch": int64(1597384463)},
		},
		{
			name:   "metadata(key) -> input@[key]",
			v1:     `root.region = metadata("region")`,
			input:  map[string]any{},
			meta:   map[string]any{"region": "eu-west"},
			wantV1: map[string]any{"region": "eu-west"},
		},
		{
			name:   "this -> input plus bare-ident rebinding",
			v1:     `root.copy = this`,
			input:  map[string]any{"x": int64(1), "y": int64(2)},
			wantV1: map[string]any{"copy": map[string]any{"x": int64(1), "y": int64(2)}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := translator.Migrate(tc.v1, translator.Options{
				Verbose:     true,
				MinCoverage: 0.0001,
			})
			if err != nil {
				t.Fatalf("migrate: %v", err)
			}
			if rep.V2Mapping == "" {
				t.Fatalf("empty V2 mapping; coverage=%v", rep.Coverage)
			}

			v1, err := benchmark.NewV1Runner(tc.v1, nil)
			if err != nil {
				t.Fatalf("v1 compile: %v", err)
			}
			v1Out, err := v1.Exec(tc.input, tc.meta)
			if err != nil {
				t.Fatalf("v1 exec: %v", err)
			}

			exec, err := bloblangv2.GlobalEnvironment().Parse(rep.V2Mapping)
			if err != nil {
				t.Fatalf("v2 compile (translated):\n%s\nerr: %v", rep.V2Mapping, err)
			}
			v2Out, _, err := exec.QueryMetadata(tc.input, tc.meta)
			if err != nil {
				t.Fatalf("v2 exec (translated):\n%s\nerr: %v", rep.V2Mapping, err)
			}

			wantV2 := tc.wantV2
			if wantV2 == nil {
				wantV2 = tc.wantV1
			}
			if !reflect.DeepEqual(v1Out, tc.wantV1) {
				t.Errorf("V1 output mismatch.\nV1:   %#v\nwant: %#v\nv2 mapping:\n%s", v1Out, tc.wantV1, rep.V2Mapping)
			}
			if !reflect.DeepEqual(v2Out, wantV2) {
				t.Errorf("V2 output mismatch.\nV2:   %#v\nwant: %#v\nv2 mapping:\n%s", v2Out, wantV2, rep.V2Mapping)
			}
		})
	}
}
