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
	"fmt"
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
	bloblangv2 "github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// TestV1NamedParamSpellingsCompile sweeps every V1 method's named-argument
// spellings through the migrator: for each V1 method, a call providing every
// parameter by V1 name is migrated, and whenever the migrator does NOT flag
// the construct (no Warning/Error change — i.e. it claims a clean
// translation), the migrated mapping must compile on the real V2 engine.
//
// This closes the `.catch(fallback: ...)` class systematically: a V1
// parameter name passed through 1:1 onto a V2 method whose parameter table
// spells it differently is a compile error in V2 (unknown named argument),
// which this sweep surfaces the moment it appears.
func TestV1NamedParamSpellingsCompile(t *testing.T) {
	env := bloblangv2.GlobalEnvironment()

	for _, spec := range query.AllMethods.Docs() {
		defs := spec.Params.Definitions
		if len(defs) == 0 || spec.Params.Variadic {
			continue
		}
		args := make([]string, 0, len(defs))
		for _, p := range defs {
			args = append(args, fmt.Sprintf("%s: 1", p.Name))
		}
		v1 := fmt.Sprintf("root = this.x.%s(%s)", spec.Name, strings.Join(args, ", "))

		t.Run(spec.Name, func(t *testing.T) {
			rep, err := translator.Migrate(context.Background(), v1, translator.Options{
				MinCoverage: 0,
			})
			var cerr *translator.CoverageError
			switch {
			case err == nil:
			case errors.As(err, &cerr):
				rep = cerr.Report
			default:
				t.Skipf("migration failed outright (acceptable for exotic methods): %v", err)
			}

			// Flagged constructs are the user's to audit — the contract under
			// test is only "unflagged translations compile".
			for _, c := range rep.Changes {
				if c.Severity >= translator.SeverityWarning {
					t.Skipf("flagged (%s) — user-audited path", c.Explanation)
				}
			}

			if _, err := env.Parse(rep.V2Mapping); err != nil {
				t.Errorf("migrator claimed a clean translation for V1 .%s(named args: %s) but the output does not compile on V2:\n  error: %v\n  migrated:\n%s",
					spec.Name, strings.Join(args, ", "), err, rep.V2Mapping)
			}
		})
	}
}
