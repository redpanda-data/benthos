// Copyright 2026 Redpanda Data, Inc.

package migrator

import "github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"

// Severity classifies a Change record. Info means the rewrite was
// purely cosmetic / mechanical; Warning flags a semantic divergence
// the user should audit; Error signals an Unsupported V1 construct
// that produced no equivalent V2 output.
type Severity = translator.Severity

// Severity values.
const (
	SeverityInfo    = translator.SeverityInfo
	SeverityWarning = translator.SeverityWarning
	SeverityError   = translator.SeverityError
)

// Category classifies the broad nature of a Change.
type Category = translator.Category

// Category values.
const (
	CategoryIdiomRewrite   = translator.CategoryIdiomRewrite
	CategorySemanticChange = translator.CategorySemanticChange
	CategoryUnsupported    = translator.CategoryUnsupported
	CategoryUncertain      = translator.CategoryUncertain
)

// RuleID identifies the translator rule that emitted a Change.
// Built-in rules use the constants exported below; custom rules can
// either reuse them or define their own (any int64 not colliding with
// a built-in is fine — RuleIDs are taxonomy hints, not authoritative).
type RuleID = translator.RuleID

// Built-in RuleID values useful for custom rules that want to
// classify their own diagnostics under the same taxonomy.
const (
	RuleUnknown                = translator.RuleUnknown
	RuleRootToOutput           = translator.RuleRootToOutput
	RuleThisToInput            = translator.RuleThisToInput
	RuleThisTargetToOutput     = translator.RuleThisTargetToOutput
	RuleBareIdentToInput       = translator.RuleBareIdentToInput
	RuleBarePathToOutput       = translator.RuleBarePathToOutput
	RuleMetaTargetToOutputMeta = translator.RuleMetaTargetToOutputMeta
	RuleMetaReadToInputMeta    = translator.RuleMetaReadToInputMeta
	RuleCoalescePrecedence     = translator.RuleCoalescePrecedence
	RuleAndOrSameLevel         = translator.RuleAndOrSameLevel
	RuleBoolNumberEquality     = translator.RuleBoolNumberEquality
	RuleModuloFloatTruncation  = translator.RuleModuloFloatTruncation
	RuleIntDivReturnsFloat     = translator.RuleIntDivReturnsFloat
	RuleOrCatchesErrors        = translator.RuleOrCatchesErrors
	RuleIfNoElseNothing        = translator.RuleIfNoElseNothing
	RuleMatchSubjectRebinds    = translator.RuleMatchSubjectRebinds
	RuleNoBracketIndexing      = translator.RuleNoBracketIndexing
	RuleStringLengthBytes      = translator.RuleStringLengthBytes
	RuleMethodDoesNotExist     = translator.RuleMethodDoesNotExist
	RuleNowReturnsString       = translator.RuleNowReturnsString
	RuleMapDeclTranslation     = translator.RuleMapDeclTranslation
	RuleImportStatement        = translator.RuleImportStatement
	RuleFromStatement          = translator.RuleFromStatement
	RuleUnsupportedConstruct   = translator.RuleUnsupportedConstruct
	RuleEmittedInvalidV2       = translator.RuleEmittedInvalidV2
	RuleBlockScopedLet         = translator.RuleBlockScopedLet
)

// Change records one translator decision: a rewrite, a semantic
// divergence, an unsupported construct.
type Change = translator.Change

// Report is the result of a successful Migrate call.
type Report = translator.Report

// Coverage summarises how successfully a V1 source was translated.
type Coverage = translator.Coverage

// CoverageError is returned by Migrate when the resulting Coverage.Ratio
// falls below Options.MinCoverage. The Report is reachable through the
// error.
type CoverageError = translator.CoverageError

// Mode classifies the V1 execution context the translated mapping
// will replace.
type Mode = translator.Mode

// FileResolver lazily resolves a V1 import path during Migrate. See
// Options.FileResolver for semantics.
type FileResolver = translator.FileResolver

// V2ImportPathRewriter rewrites V1 import path strings to their V2
// equivalents. See Options.V2ImportPathRewriter.
type V2ImportPathRewriter = translator.V2ImportPathRewriter

// Mode values.
const (
	ModeMutation = translator.ModeMutation
	ModeMapping  = translator.ModeMapping
)
