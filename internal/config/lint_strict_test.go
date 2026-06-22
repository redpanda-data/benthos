// Copyright 2026 Redpanda Data, Inc.

package config_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

// strictCatchLints runs the full linted read over a config and returns only the
// strict-catch warnings, isolating them from any incidental lints.
func strictCatchLints(t *testing.T, conf string) []docs.Lint {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(conf), 0o644))

	rdr := config.NewReader(path, nil)
	_, lints, err := rdr.ReadYAMLFileLinted(context.Background(), config.Spec(), path, false, docs.NewLintConfig(bundle.GlobalEnvironment))
	require.NoError(t, err)

	var out []docs.Lint
	for _, l := range lints {
		if strings.Contains(l.What, "error_handling.strict") {
			out = append(out, l)
		}
	}
	return out
}

func TestLintStrictCatchInPipeline(t *testing.T) {
	lints := strictCatchLints(t, `
error_handling:
  strict: true
input:
  generate:
    mapping: 'root = {}'
    count: 1
output:
  drop: {}
pipeline:
  processors:
    - mapping: 'root = this'
    - catch: []
`)
	require.Len(t, lints, 1)
	assert.Equal(t, docs.LintWarning, lints[0].Level)
	assert.Contains(t, lints[0].What, "try_catch")
	assert.Greater(t, lints[0].Line, 0, "the lint should carry the line of the catch processor")
}

func TestLintStrictCatchInInputAndOutput(t *testing.T) {
	lints := strictCatchLints(t, `
error_handling:
  strict: true
input:
  generate:
    mapping: 'root = {}'
    count: 1
  processors:
    - catch: []
output:
  drop: {}
  processors:
    - catch: []
`)
	assert.Len(t, lints, 2, "a catch in both input and output processors should each warn")
}

func TestLintStrictDisabledNoWarning(t *testing.T) {
	lints := strictCatchLints(t, `
input:
  generate:
    mapping: 'root = {}'
    count: 1
output:
  drop: {}
pipeline:
  processors:
    - catch: []
`)
	assert.Empty(t, lints)
}

// try_catch is the recommended recovery scope under strict and must not warn.
func TestLintStrictTryCatchNotFlagged(t *testing.T) {
	lints := strictCatchLints(t, `
error_handling:
  strict: true
input:
  generate:
    mapping: 'root = {}'
    count: 1
output:
  drop: {}
pipeline:
  processors:
    - try_catch:
        processors:
          - mapping: 'root = this'
        catch:
          - mapping: 'root = "recovered"'
`)
	assert.Empty(t, lints)
}

// A catch nested inside a switch case is still dead under strict and must warn.
func TestLintStrictCatchNestedInSwitch(t *testing.T) {
	lints := strictCatchLints(t, `
error_handling:
  strict: true
input:
  generate:
    mapping: 'root = {}'
    count: 1
output:
  drop: {}
pipeline:
  processors:
    - switch:
        - processors:
            - catch: []
`)
	require.Len(t, lints, 1)
	assert.Greater(t, lints[0].Line, 0)
}

// Under the metadata-based try_catch model, a catch processor is dead at every
// position under strict — including within a try_catch's `catch` field, which
// runs on messages whose failure flag has already been cleared. Both catches
// warn.
func TestLintStrictCatchNestedInTryCatch(t *testing.T) {
	lints := strictCatchLints(t, `
error_handling:
  strict: true
input:
  generate:
    mapping: 'root = {}'
    count: 1
output:
  drop: {}
pipeline:
  processors:
    - try_catch:
        processors:
          - catch: []
        catch:
          - catch: []
`)
	require.Len(t, lints, 2, "a catch processor is dead under strict at any position, including within a try_catch catch block")
}

func TestLintStrictNoCatchNoWarning(t *testing.T) {
	lints := strictCatchLints(t, `
error_handling:
  strict: true
input:
  generate:
    mapping: 'root = {}'
    count: 1
output:
  drop: {}
pipeline:
  processors:
    - mapping: 'root = this'
`)
	assert.Empty(t, lints)
}
