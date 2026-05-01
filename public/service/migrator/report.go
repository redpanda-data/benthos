// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	"fmt"

	bloblmig "github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// Severity classifies a Change record. Info means the rewrite was
// purely mechanical; Warning flags a divergence the user should
// audit; Error signals an Unsupported plugin that produced no
// equivalent output (the plugin is left untouched).
type Severity int

// Severity values.
const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

// String satisfies fmt.Stringer.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	}
	return fmt.Sprintf("severity(%d)", s)
}

// Outcome classifies the disposition of a single matched component.
type Outcome int

// Outcome values.
const (
	// OutcomeRewritten — a rule matched and replaced the plugin.
	OutcomeRewritten Outcome = iota
	// OutcomeSkipped — a rule matched but declined to rewrite.
	OutcomeSkipped
	// OutcomeUnsupported — a rule matched but flagged the plugin as
	// untranslatable; the plugin is left in place.
	OutcomeUnsupported
)

// String satisfies fmt.Stringer.
func (o Outcome) String() string {
	switch o {
	case OutcomeRewritten:
		return "rewritten"
	case OutcomeSkipped:
		return "skipped"
	case OutcomeUnsupported:
		return "unsupported"
	}
	return fmt.Sprintf("outcome(%d)", o)
}

// Change records the disposition of one matched component.
type Change struct {
	// Target identifies the (ComponentType, Name) the rule was
	// registered against.
	Target Target
	// Path is the dotted location of the component within the config.
	Path string
	// Label, if non-empty, is the YAML `label` of the component.
	Label string
	// LineStart, LineEnd is the 1-indexed line span of the component
	// in the source YAML.
	LineStart, LineEnd int
	// Outcome is the disposition of the component.
	Outcome Outcome
	// Severity classifies the Change for filtering / CI gating.
	Severity Severity
	// NewName is the plugin name the rule rewrote the component into,
	// or "" if the component was not rewritten.
	NewName string
	// Reason carries the explanation supplied by Skip / Unsupported,
	// or a short summary of the rewrite.
	Reason string
	// BloblangReport, if non-nil, is the V1->V2 translation report
	// for the embedded Bloblang body that was rewritten. Inspect it
	// for per-mapping coverage and warnings.
	BloblangReport *bloblmig.Report
}

// Coverage summarises the migrator's progress over the input config.
// Only matched components are counted; components without a registered
// rule are ignored.
type Coverage struct {
	// Matched is the number of components for which a rule fired.
	Matched int
	// Rewritten is the number of components with OutcomeRewritten.
	Rewritten int
	// Skipped is the number of components with OutcomeSkipped.
	Skipped int
	// Unsupported is the number of components with OutcomeUnsupported.
	Unsupported int
	// Ratio is Rewritten / (Rewritten + Unsupported), or 1 when there
	// are no Rewritten or Unsupported components.
	Ratio float64
}

// Report is the result of a successful Migrate call.
type Report struct {
	// OutputYAML is the rewritten config. When no rule fires, this
	// equals the input.
	OutputYAML string
	// Changes records every component a rule fired against, in the
	// order the migrator visited them.
	Changes []Change
	// Coverage aggregates Changes into a coverage ratio.
	Coverage Coverage
}

// CoverageError is returned by Migrate when the resulting
// Coverage.Ratio falls below Options.MinCoverage. The Report is
// reachable through the error.
type CoverageError struct {
	Coverage Coverage
	Min      float64
	Report   *Report
}

// Error satisfies the error interface.
func (e *CoverageError) Error() string {
	return fmt.Sprintf(
		"migrator: coverage %.2f is below threshold %.2f (rewritten=%d unsupported=%d skipped=%d)",
		e.Coverage.Ratio, e.Min,
		e.Coverage.Rewritten, e.Coverage.Unsupported, e.Coverage.Skipped,
	)
}
