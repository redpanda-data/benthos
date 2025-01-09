// Copyright 2025 Redpanda Data, Inc.

package mapping

import (
	"fmt"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/value"
)

// Statement represents a bloblang mapping statement.
type Statement interface {
	QueryTargets(ctx query.TargetsContext) (query.TargetsContext, []query.TargetPath)
	AssignmentTargets() []TargetPath
	Input() []rune
	Execute(fnContext query.FunctionContext, asContext AssignmentContext) error
}

//------------------------------------------------------------------------------

// SingleStatement describes an isolated mapping statement, where the result of
// a query function is to be mapped according to an Assignment.
type SingleStatement struct {
	input      []rune
	assignment Assignment
	query      query.Function
}

// NewSingleStatement initialises a new mapping statement from an Assignment and
// query.Function. The input parameter is an optional slice pointing to the
// parsed expression that created the statement.
func NewSingleStatement(input []rune, assignment Assignment, query query.Function) *SingleStatement {
	return &SingleStatement{
		input:      input,
		assignment: assignment,
		query:      query,
	}
}

// QueryTargets returns the query targets for the underlying query.
func (s *SingleStatement) QueryTargets(ctx query.TargetsContext) (query.TargetsContext, []query.TargetPath) {
	return s.query.QueryTargets(ctx)
}

// AssignmentTargets returns a representation of what the underlying assignment
// targets.
func (s *SingleStatement) AssignmentTargets() []TargetPath {
	return []TargetPath{s.assignment.Target()}
}

// Input returns the underlying parsed expression of this statement.
func (s *SingleStatement) Input() []rune {
	return s.input
}

// Execute executes this statement and applies the result onto the assigned
// destination.
func (s *SingleStatement) Execute(fnContext query.FunctionContext, asContext AssignmentContext) error {
	res, err := s.query.Exec(fnContext)
	if err != nil {
		return err
	}
	if _, isNothing := res.(value.Nothing); isNothing {
		// Skip assignment entirely
		return nil
	}
	return s.assignment.Apply(res, asContext)
}

//------------------------------------------------------------------------------

type rootLevelIfStatementPair struct {
	query      query.Function
	statements []Statement
}

// RootLevelIfStatement describes an isolated conditional mapping statement.
type RootLevelIfStatement struct {
	input []rune
	pairs []rootLevelIfStatementPair
}

// NewRootLevelIfStatement initialises a new conditional mapping statement. The
// input parameter is a slice pointing to the parsed expression that created the
// statement.
func NewRootLevelIfStatement(input []rune) *RootLevelIfStatement {
	return &RootLevelIfStatement{
		input: input,
	}
}

// Add adds query statement pairs to the root level if statement.
func (r *RootLevelIfStatement) Add(query query.Function, statements ...Statement) *RootLevelIfStatement {
	r.pairs = append(r.pairs, rootLevelIfStatementPair{query: query, statements: statements})
	return r
}

// QueryTargets returns the query targets for the underlying conditional mapping
// statement.
func (r *RootLevelIfStatement) QueryTargets(ctx query.TargetsContext) (query.TargetsContext, []query.TargetPath) {
	var paths []query.TargetPath
	for _, p := range r.pairs {
		if p.query != nil {
			_, tmp := p.query.QueryTargets(ctx)
			paths = append(paths, tmp...)
		}
		for _, s := range p.statements {
			_, tmp := s.QueryTargets(ctx)
			paths = append(paths, tmp...)
		}
	}
	return ctx, paths
}

// AssignmentTargets returns a representation of what the underlying conditional
// mapping statement targets.
func (r *RootLevelIfStatement) AssignmentTargets() []TargetPath {
	var paths []TargetPath
	for _, p := range r.pairs {
		for _, s := range p.statements {
			paths = append(paths, s.AssignmentTargets()...)
		}
	}
	return paths
}

// Input returns the underlying parsed expression of this conditional mapping
// statement.
func (r *RootLevelIfStatement) Input() []rune {
	return r.input
}

// Execute executes this statement if the underlying condition evaluates to
// true.
func (r *RootLevelIfStatement) Execute(fnContext query.FunctionContext, asContext AssignmentContext) error {
	for i, p := range r.pairs {
		if p.query != nil {
			queryVal, err := p.query.Exec(fnContext)
			if err != nil {
				return fmt.Errorf("failed to check if condition %v: %w", i+1, err)
			}
			queryRes, isBool := queryVal.(bool)
			if !isBool {
				return fmt.Errorf("%v resolved to a non-boolean value %v (%T)", p.query.Annotation(), queryVal, queryVal)
			}
			if !queryRes {
				continue
			}
		}
		for _, stmt := range p.statements {
			if err := stmt.Execute(fnContext, asContext); err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}
