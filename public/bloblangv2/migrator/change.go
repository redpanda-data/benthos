// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	"context"
	"fmt"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// The types below are the stable PUBLIC surface of the migrator. They
// deliberately do NOT alias the internal translator types: the internal
// structs can gain/rename fields without breaking this package's API. Values
// are converted at the Migrate boundary (see reportFrom / coverageErrorFrom).
// The enum consts are pinned to the internal values via conversion so they
// stay in sync automatically while remaining a distinct public type.

// Severity classifies a Change record. Info means the rewrite was purely
// cosmetic / mechanical; Warning flags a semantic divergence the user should
// audit; Error signals an Unsupported V1 construct that produced no
// equivalent V2 output.
type Severity int

// Severity values.
const (
	SeverityInfo    Severity = Severity(translator.SeverityInfo)
	SeverityWarning Severity = Severity(translator.SeverityWarning)
	SeverityError   Severity = Severity(translator.SeverityError)
)

// String returns the lowercase severity name.
func (s Severity) String() string { return translator.Severity(s).String() }

// Category classifies the broad nature of a Change.
type Category int

// Category values.
const (
	CategoryIdiomRewrite   Category = Category(translator.CategoryIdiomRewrite)
	CategorySemanticChange Category = Category(translator.CategorySemanticChange)
	CategoryUnsupported    Category = Category(translator.CategoryUnsupported)
	CategoryUncertain      Category = Category(translator.CategoryUncertain)
)

// String returns the kebab-case category name.
func (c Category) String() string { return translator.Category(c).String() }

// RuleID identifies the translator rule that emitted a Change. Built-in rules
// use the constants exported below; custom rules can reuse them or use their
// own — RuleIDs are taxonomy hints, not authoritative.
type RuleID int

// Built-in RuleID values, useful for classifying custom diagnostics under the
// same taxonomy or for filtering a Report's Changes.
const (
	RuleUnknown                    RuleID = RuleID(translator.RuleUnknown)
	RuleRootToOutput               RuleID = RuleID(translator.RuleRootToOutput)
	RuleThisToInput                RuleID = RuleID(translator.RuleThisToInput)
	RuleThisTargetToOutput         RuleID = RuleID(translator.RuleThisTargetToOutput)
	RuleBareIdentToInput           RuleID = RuleID(translator.RuleBareIdentToInput)
	RuleBarePathToOutput           RuleID = RuleID(translator.RuleBarePathToOutput)
	RuleMetaTargetToOutputMeta     RuleID = RuleID(translator.RuleMetaTargetToOutputMeta)
	RuleMetaReadToInputMeta        RuleID = RuleID(translator.RuleMetaReadToInputMeta)
	RuleCoalescePrecedence         RuleID = RuleID(translator.RuleCoalescePrecedence)
	RuleAndOrSameLevel             RuleID = RuleID(translator.RuleAndOrSameLevel)
	RuleBoolNumberEquality         RuleID = RuleID(translator.RuleBoolNumberEquality)
	RuleModuloFloatTruncation      RuleID = RuleID(translator.RuleModuloFloatTruncation)
	RuleIntDivReturnsFloat         RuleID = RuleID(translator.RuleIntDivReturnsFloat)
	RuleOrCatchesErrors            RuleID = RuleID(translator.RuleOrCatchesErrors)
	RuleIfNoElseNothing            RuleID = RuleID(translator.RuleIfNoElseNothing)
	RuleMatchSubjectRebinds        RuleID = RuleID(translator.RuleMatchSubjectRebinds)
	RuleNoBracketIndexing          RuleID = RuleID(translator.RuleNoBracketIndexing)
	RuleStringLengthBytes          RuleID = RuleID(translator.RuleStringLengthBytes)
	RuleMethodDoesNotExist         RuleID = RuleID(translator.RuleMethodDoesNotExist)
	RuleNowReturnsString           RuleID = RuleID(translator.RuleNowReturnsString)
	RuleMapDeclTranslation         RuleID = RuleID(translator.RuleMapDeclTranslation)
	RuleImportStatement            RuleID = RuleID(translator.RuleImportStatement)
	RuleFromStatement              RuleID = RuleID(translator.RuleFromStatement)
	RuleUnsupportedConstruct       RuleID = RuleID(translator.RuleUnsupportedConstruct)
	RuleEmittedInvalidV2           RuleID = RuleID(translator.RuleEmittedInvalidV2)
	RuleBlockScopedLet             RuleID = RuleID(translator.RuleBlockScopedLet)
	RuleSliceByteVsCodepoint       RuleID = RuleID(translator.RuleSliceByteVsCodepoint)
	RuleCoalesceDuplicatesFallback RuleID = RuleID(translator.RuleCoalesceDuplicatesFallback)
)

// String returns the kebab-case rule name.
func (r RuleID) String() string { return translator.RuleID(r).String() }

// Mode classifies the V1 execution context the translated mapping replaces.
type Mode int

// Mode values.
const (
	ModeMutation Mode = Mode(translator.ModeMutation)
	ModeMapping  Mode = Mode(translator.ModeMapping)
)

// String returns the mode name.
func (m Mode) String() string { return translator.Mode(m).String() }

// FileResolver lazily resolves a V1 import path during Migrate. See
// Options.FileResolver for semantics.
type FileResolver func(ctx context.Context, parentKey, importPath string) (canonicalKey, content string, ok bool)

// V2ImportPathRewriter rewrites V1 import path strings to their V2
// equivalents. See Options.V2ImportPathRewriter.
type V2ImportPathRewriter func(v1Path string) string

// Change records one translator decision: a rewrite, a semantic divergence,
// or an unsupported construct.
type Change struct {
	Line        int // start line of the affected V1 span
	Column      int // start column
	EndLine     int // end line (may equal Line)
	EndColumn   int // end column
	Severity    Severity
	Category    Category
	RuleID      RuleID
	SpecRef     string // current spec anchor, e.g. "§14#48"
	Original    string // V1 snippet (for citation)
	Translated  string // V2 snippet emitted; empty if dropped
	Explanation string // one-line human-readable
}

// Coverage summarises how successfully a V1 source was translated.
type Coverage struct {
	Total       int
	Translated  int
	Rewritten   int
	Unsupported int
	Ratio       float64
}

// Report is the result of a successful Migrate call.
type Report struct {
	V2Mapping string
	// V2Files holds imported files translated from V1 to V2, keyed by the
	// import path from the V1 source. Empty when Options.Files was empty.
	V2Files  map[string]string
	Changes  []Change
	Coverage Coverage
}

// CoverageError is returned by Migrate when the resulting Coverage.Ratio falls
// below Options.MinCoverage. The Report is reachable through the error.
type CoverageError struct {
	Coverage Coverage
	Min      float64
	Report   *Report
}

// Error satisfies the error interface.
func (e *CoverageError) Error() string {
	return fmt.Sprintf(
		"migrator: translation coverage %.2f is below threshold %.2f (translated=%d rewritten=%d unsupported=%d total=%d)",
		e.Coverage.Ratio, e.Min,
		e.Coverage.Translated, e.Coverage.Rewritten, e.Coverage.Unsupported, e.Coverage.Total,
	)
}

// ---- conversions from the internal translator types (boundary only) ----

func changeFrom(c translator.Change) Change {
	return Change{
		Line:        c.Line,
		Column:      c.Column,
		EndLine:     c.EndLine,
		EndColumn:   c.EndColumn,
		Severity:    Severity(c.Severity),
		Category:    Category(c.Category),
		RuleID:      RuleID(c.RuleID),
		SpecRef:     c.SpecRef,
		Original:    c.Original,
		Translated:  c.Translated,
		Explanation: c.Explanation,
	}
}

// changeTo converts a public Change back to the internal type, used when a
// custom rule emits a Change via Context.Note.
func changeTo(c Change) translator.Change {
	return translator.Change{
		Line:        c.Line,
		Column:      c.Column,
		EndLine:     c.EndLine,
		EndColumn:   c.EndColumn,
		Severity:    translator.Severity(c.Severity),
		Category:    translator.Category(c.Category),
		RuleID:      translator.RuleID(c.RuleID),
		SpecRef:     c.SpecRef,
		Original:    c.Original,
		Translated:  c.Translated,
		Explanation: c.Explanation,
	}
}

func coverageFrom(c translator.Coverage) Coverage {
	return Coverage{
		Total:       c.Total,
		Translated:  c.Translated,
		Rewritten:   c.Rewritten,
		Unsupported: c.Unsupported,
		Ratio:       c.Ratio,
	}
}

func reportFrom(r *translator.Report) *Report {
	if r == nil {
		return nil
	}
	out := &Report{
		V2Mapping: r.V2Mapping,
		Coverage:  coverageFrom(r.Coverage),
	}
	if len(r.V2Files) > 0 {
		out.V2Files = make(map[string]string, len(r.V2Files))
		for k, v := range r.V2Files {
			out.V2Files[k] = v
		}
	}
	if len(r.Changes) > 0 {
		out.Changes = make([]Change, len(r.Changes))
		for i, c := range r.Changes {
			out.Changes[i] = changeFrom(c)
		}
	}
	return out
}

func coverageErrorFrom(e *translator.CoverageError) *CoverageError {
	if e == nil {
		return nil
	}
	return &CoverageError{
		Coverage: coverageFrom(e.Coverage),
		Min:      e.Min,
		Report:   reportFrom(e.Report),
	}
}
