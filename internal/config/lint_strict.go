// Copyright 2026 Redpanda Data, Inc.

package config

import (
	"gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/internal/docs"
)

const strictCatchLintMsg = "a `catch` processor does not recover messages while `error_handling.strict` is enabled, as a failed message short-circuits past it. Wrap the failing step and its recovery within a `try_catch` processor (or a `retry` for transient failures) instead."

// lintStrictErrorHandling warns about configuration that will silently stop
// handling errors once strict error handling is enabled.
//
// Under strict mode a failed message short-circuits the remaining processors in
// its chain, so a standalone `catch` processor — a downstream catcher of an
// upstream failure — is skipped and never recovers anything. There is no longer
// any scope in which a legacy `catch` observes a failed message: a `try_catch`
// clears the failure flag (moving the error into metadata) before running its
// `catch` block, and every other composition processor that runs children also
// either short-circuits or starts them unflagged. Recovery is therefore done
// exclusively with `try_catch`/`retry`, and any `catch` processor (at any
// nesting depth) is reported when strict is enabled.
func lintStrictErrorHandling(conf Type, node *yaml.Node, prov docs.Provider) []docs.Lint {
	if !conf.ErrorHandling.Strict || node == nil {
		return nil
	}

	var lints []docs.Lint
	walkConf := docs.WalkComponentConfig{
		Provider: prov,
		Func: func(c docs.WalkedComponent) error {
			if c.Name == "catch" {
				lints = append(lints, docs.NewLintWarning(c.LineStart, docs.LintCustom, strictCatchLintMsg))
			}
			return nil
		},
	}

	// A structurally invalid config would already have failed to parse before
	// this point; if the walk still errors we skip the rule rather than fail.
	_ = Spec().WalkComponentsYAML(walkConf, node)
	return lints
}
