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

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestFormatJSONDualEngine asserts migrated V1 .format_json() calls produce
// byte-identical output on the real V2 engine. The V1 and V2 signatures
// differ on every axis: V1 defaults to 4-space indent + HTML escaping and
// has a no_indent parameter; V2 defaults to compact + unescaped and has no
// no_indent. The migrator compensates by always emitting explicit
// indent/escape_html arguments.
func TestFormatJSONDualEngine(t *testing.T) {
	in := map[string]any{"b": int64(2), "a": "<b>&amp;</b>"}
	runDualCases(t, []dualCase{
		{
			name: "bare call preserves V1 indent and escaping",
			v1:   `root = this.format_json().string()`,
			in:   in,
		},
		{
			name: "no_indent true becomes compact indent",
			v1:   `root = this.format_json(no_indent: true).string()`,
			in:   in,
		},
		{
			name: "explicit escape_html false passes through",
			v1:   `root = this.format_json(escape_html: false).string()`,
			in:   in,
		},
		{
			name: "custom positional indent passes through",
			v1:   `root = this.format_json("  ").string()`,
			in:   in,
		},
		{
			name: "no_indent with escape_html false",
			v1:   `root = this.format_json(no_indent: true, escape_html: false).string()`,
			in:   in,
		},
		{
			name: "named indent with escaping kept",
			v1:   `root = this.format_json(indent: "\t").string()`,
			in:   in,
		},
	})
}

// TestFormatJSONRewriteShape asserts the emitted V2 argument shapes.
func TestFormatJSONRewriteShape(t *testing.T) {
	migrate := func(t *testing.T, v1 string) *translator.Report {
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

	t.Run("bare call emits V1 defaults explicitly", func(t *testing.T) {
		rep := migrate(t, `root = this.format_json()`)
		want := `format_json(indent: "    ", escape_html: true)`
		if !strings.Contains(rep.V2Mapping, want) {
			t.Errorf("expected %q in migrated mapping; got:\n%s", want, rep.V2Mapping)
		}
	})

	t.Run("no_indent literal true becomes empty indent", func(t *testing.T) {
		rep := migrate(t, `root = this.format_json(no_indent: true)`)
		want := `format_json(indent: "", escape_html: true)`
		if !strings.Contains(rep.V2Mapping, want) {
			t.Errorf("expected %q in migrated mapping; got:\n%s", want, rep.V2Mapping)
		}
	})

	t.Run("no_indent literal false keeps V1 default indent", func(t *testing.T) {
		rep := migrate(t, `root = this.format_json(no_indent: false)`)
		want := `format_json(indent: "    ", escape_html: true)`
		if !strings.Contains(rep.V2Mapping, want) {
			t.Errorf("expected %q in migrated mapping; got:\n%s", want, rep.V2Mapping)
		}
	})

	t.Run("dynamic no_indent is dropped with a warning", func(t *testing.T) {
		rep := migrate(t, `root = this.format_json(no_indent: this.compact)`)
		if strings.Contains(rep.V2Mapping, "no_indent") {
			t.Errorf("no_indent must not survive into V2; got:\n%s", rep.V2Mapping)
		}
		found := false
		for _, c := range rep.Changes {
			if c.Severity == translator.SeverityWarning && strings.Contains(c.Explanation, "no_indent") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected a Warning about the dropped dynamic no_indent; got:\n%s", changeList(rep.Changes))
		}
	})

	t.Run("explicit escape_html expression passes through", func(t *testing.T) {
		rep := migrate(t, `root = this.format_json(escape_html: this.esc)`)
		want := `escape_html: input?.esc`
		if !strings.Contains(rep.V2Mapping, want) {
			t.Errorf("expected %q in migrated mapping; got:\n%s", want, rep.V2Mapping)
		}
	})
}
