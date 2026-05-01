// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	bloblmig "github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// Migrator rewrites Benthos stream configs by replacing one plugin
// instance with another. Construct one with New, register any custom
// rules with RegisterRule, then call Migrate. The Migrator is not safe
// for concurrent registration but is safe for concurrent Migrate
// calls once registration is complete.
//
// The built-in rules — bloblang -> bloblang_v2, mapping ->
// bloblang_v2, mutation -> bloblang_v2 — are always active. Custom
// rules layer on top and shadow built-ins on Target collision.
type Migrator struct {
	rules map[Target]Rule
}

// New creates a Migrator with the built-in plugin migration rules
// registered. Custom rules can be layered on top with RegisterRule.
func New() *Migrator {
	m := &Migrator{rules: map[Target]Rule{}}
	for t, r := range builtInRules() {
		m.rules[t] = r
	}
	return m
}

// RegisterRule registers a custom rule for the given Target. If a
// rule is already registered for the same Target the new rule
// replaces it (so downstream rules can override the built-ins).
func (m *Migrator) RegisterRule(target Target, rule Rule) {
	m.rules[target] = rule
}

// Migrate rewrites the supplied stream config YAML by applying every
// registered rule whose Target matches a component instance found in
// the config. Returns a *Report on success.
//
// Returns *CoverageError when the resulting Coverage.Ratio falls
// below opts.MinCoverage; the Report is reachable via the error.
func (m *Migrator) Migrate(yamlBytes []byte, opts Options) (*Report, error) {
	bm := opts.BloblangMigrator
	if bm == nil {
		bm = bloblmig.New()
	}
	ctx := &Context{
		bloblang:     bm,
		bloblangOpts: opts.BloblangOptions,
	}

	out, changes, err := walk(yamlBytes, m.rules, ctx, opts.Verbose)
	if err != nil {
		return nil, err
	}

	cov := computeCoverage(changes)
	rep := &Report{
		OutputYAML: out,
		Changes:    changes,
		Coverage:   cov,
	}
	if opts.MinCoverage > 0 && cov.Ratio < opts.MinCoverage {
		return nil, &CoverageError{
			Coverage: cov,
			Min:      opts.MinCoverage,
			Report:   rep,
		}
	}
	return rep, nil
}

// Migrate is a package-level convenience that builds a default
// Migrator (built-in rules only) and runs it against the supplied
// YAML. Equivalent to `New().Migrate(src, opts)`.
func Migrate(yamlBytes []byte, opts Options) (*Report, error) {
	return New().Migrate(yamlBytes, opts)
}

func computeCoverage(changes []Change) Coverage {
	var c Coverage
	for _, ch := range changes {
		c.Matched++
		switch ch.Outcome {
		case OutcomeRewritten:
			c.Rewritten++
		case OutcomeSkipped:
			c.Skipped++
		case OutcomeUnsupported:
			c.Unsupported++
		}
	}
	denom := c.Rewritten + c.Unsupported
	if denom == 0 {
		c.Ratio = 1
	} else {
		c.Ratio = float64(c.Rewritten) / float64(denom)
	}
	return c
}
