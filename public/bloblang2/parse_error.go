// Copyright 2026 Redpanda Data, Inc.

package bloblang2

import (
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

// ParseError is a structured error type for Bloblang V2 parser and resolver
// errors that provides access to the line and column of the first reported
// issue. The full list of diagnostics is preserved and rendered by Error.
type ParseError struct {
	// Line and Column describe the position of the first diagnostic.
	Line   int
	Column int

	errs []syntax.PosError
}

// Error returns a multi-line error string listing every diagnostic.
func (p *ParseError) Error() string {
	if len(p.errs) == 0 {
		return "bloblang2: unknown parse error"
	}
	var b strings.Builder
	for i, e := range p.errs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(e.Error())
	}
	return b.String()
}

func parseErrorFromPosErrors(errs []syntax.PosError) *ParseError {
	if len(errs) == 0 {
		return nil
	}
	first := errs[0].Pos
	return &ParseError{
		Line:   first.Line,
		Column: first.Column,
		errs:   errs,
	}
}
