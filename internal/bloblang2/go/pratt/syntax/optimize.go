package syntax

import (
	"math"
	"strconv"
)

// Optimize performs post-parse AST optimizations on a program:
//   - Path collapse: chains of field/index/method access → PathExpr
//   - Constant folding: literal-only expressions evaluated at compile time
//   - Dead code elimination: unreachable if/match branches pruned
//
// Call after Parse and before Resolve.
func Optimize(prog *Program) {
	o := &optimizer{}
	for i, stmt := range prog.Stmts {
		prog.Stmts[i] = o.optimizeStmt(stmt)
	}
	for _, m := range prog.Maps {
		o.optimizeExprBody(m.Body)
	}
}

type optimizer struct{}

// -----------------------------------------------------------------------
// Statement optimization
// -----------------------------------------------------------------------

func (o *optimizer) optimizeStmt(stmt Stmt) Stmt {
	switch s := stmt.(type) {
	case *Assignment:
		s.Value = o.optimizeExpr(s.Value)
	case *IfStmt:
		o.optimizeIfStmt(s)
	case *MatchStmt:
		o.optimizeMatchStmt(s)
	}
	return stmt
}

func (o *optimizer) optimizeIfStmt(s *IfStmt) {
	// Dead code elimination: prune branches with literal boolean conditions.
	var kept []IfBranch
	for _, branch := range s.Branches {
		branch.Cond = o.optimizeExpr(branch.Cond)
		if lit, ok := branch.Cond.(*LiteralExpr); ok {
			if lit.TokenType == TRUE {
				// Condition is always true — keep this branch, discard the rest.
				for i := range branch.Body {
					branch.Body[i] = o.optimizeStmt(branch.Body[i])
				}
				kept = append(kept, branch)
				s.Branches = kept
				s.Else = nil // unreachable
				return
			}
			if lit.TokenType == FALSE {
				// Condition is always false — skip this branch.
				continue
			}
			// Non-boolean literal (string, int, null, etc.) — keep it; will error at runtime.
		}
		for i := range branch.Body {
			branch.Body[i] = o.optimizeStmt(branch.Body[i])
		}
		kept = append(kept, branch)
	}
	s.Branches = kept
	for i := range s.Else {
		s.Else[i] = o.optimizeStmt(s.Else[i])
	}
}

func (o *optimizer) optimizeMatchStmt(s *MatchStmt) {
	if s.Subject != nil {
		s.Subject = o.optimizeExpr(s.Subject)
	}
	for i := range s.Cases {
		if s.Cases[i].Pattern != nil {
			s.Cases[i].Pattern = o.optimizeExpr(s.Cases[i].Pattern)
		}
		if body, ok := s.Cases[i].Body.([]Stmt); ok {
			for j := range body {
				body[j] = o.optimizeStmt(body[j])
			}
		}
	}
}

// -----------------------------------------------------------------------
// Expression optimization
// -----------------------------------------------------------------------

func (o *optimizer) optimizeExpr(expr Expr) Expr {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *BinaryExpr:
		e.Left = o.optimizeExpr(e.Left)
		e.Right = o.optimizeExpr(e.Right)
		if folded := o.foldBinary(e); folded != nil {
			return folded
		}
		return e

	case *UnaryExpr:
		e.Operand = o.optimizeExpr(e.Operand)
		if folded := o.foldUnary(e); folded != nil {
			return folded
		}
		return e

	case *FieldAccessExpr:
		e.Receiver = o.optimizeExpr(e.Receiver)
		return o.tryCollapsePath(e)

	case *IndexExpr:
		e.Receiver = o.optimizeExpr(e.Receiver)
		e.Index = o.optimizeExpr(e.Index)
		return o.tryCollapsePath(e)

	case *MethodCallExpr:
		e.Receiver = o.optimizeExpr(e.Receiver)
		for i := range e.Args {
			e.Args[i].Value = o.optimizeExpr(e.Args[i].Value)
		}
		return o.tryCollapsePath(e)

	case *CallExpr:
		for i := range e.Args {
			e.Args[i].Value = o.optimizeExpr(e.Args[i].Value)
		}
		return e

	case *ArrayLiteral:
		for i := range e.Elements {
			e.Elements[i] = o.optimizeExpr(e.Elements[i])
		}
		return e

	case *ObjectLiteral:
		for i := range e.Entries {
			e.Entries[i].Key = o.optimizeExpr(e.Entries[i].Key)
			e.Entries[i].Value = o.optimizeExpr(e.Entries[i].Value)
		}
		return e

	case *IfExpr:
		return o.optimizeIfExpr(e)

	case *MatchExpr:
		return o.optimizeMatchExpr(e)

	case *LambdaExpr:
		o.optimizeExprBody(e.Body)
		return e

	case *PathExpr:
		// Already collapsed — optimize sub-expressions in index segments.
		for i := range e.Segments {
			if e.Segments[i].Index != nil {
				e.Segments[i].Index = o.optimizeExpr(e.Segments[i].Index)
			}
			for j := range e.Segments[i].Args {
				e.Segments[i].Args[j].Value = o.optimizeExpr(e.Segments[i].Args[j].Value)
			}
		}
		return e

	default:
		// LiteralExpr, InputExpr, InputMetaExpr, OutputExpr, OutputMetaExpr,
		// VarExpr, IdentExpr — no children to optimize.
		return expr
	}
}

func (o *optimizer) optimizeExprBody(body *ExprBody) {
	if body == nil {
		return
	}
	for i := range body.Assignments {
		body.Assignments[i].Value = o.optimizeExpr(body.Assignments[i].Value)
	}
	body.Result = o.optimizeExpr(body.Result)
}

func (o *optimizer) optimizeIfExpr(e *IfExpr) Expr {
	// Dead code elimination: prune branches with literal boolean conditions.
	var kept []IfExprBranch
	for _, branch := range e.Branches {
		branch.Cond = o.optimizeExpr(branch.Cond)
		if lit, ok := branch.Cond.(*LiteralExpr); ok {
			if lit.TokenType == TRUE {
				// Always true — this branch always executes.
				o.optimizeExprBody(branch.Body)
				kept = append(kept, branch)
				e.Branches = kept
				e.Else = nil
				return e
			}
			if lit.TokenType == FALSE {
				// Always false — skip this branch.
				continue
			}
			// Non-boolean literal — keep it; will error at runtime.
		}
		o.optimizeExprBody(branch.Body)
		kept = append(kept, branch)
	}
	e.Branches = kept
	if e.Else != nil {
		o.optimizeExprBody(e.Else)
	}
	return e
}

func (o *optimizer) optimizeMatchExpr(e *MatchExpr) Expr {
	if e.Subject != nil {
		e.Subject = o.optimizeExpr(e.Subject)
	}
	for i := range e.Cases {
		if e.Cases[i].Pattern != nil {
			e.Cases[i].Pattern = o.optimizeExpr(e.Cases[i].Pattern)
		}
		switch body := e.Cases[i].Body.(type) {
		case Expr:
			e.Cases[i].Body = o.optimizeExpr(body)
		case *ExprBody:
			o.optimizeExprBody(body)
		}
	}
	return e
}

// -----------------------------------------------------------------------
// Path collapse
// -----------------------------------------------------------------------

// tryCollapsePath attempts to collapse a postfix chain (field access, index,
// method call) rooted at a known context (input, output, variable, etc.)
// into a single PathExpr.
func (o *optimizer) tryCollapsePath(expr Expr) Expr {
	// Unwind the chain bottom-up to find the root and collect segments.
	var segments []PathSegment
	current := expr

	for {
		switch e := current.(type) {
		case *FieldAccessExpr:
			segments = append(segments, PathSegment{
				Kind:     PathSegField,
				Name:     e.Field,
				NullSafe: e.NullSafe,
				Pos:      e.FieldPos,
			})
			current = e.Receiver
			continue

		case *IndexExpr:
			segments = append(segments, PathSegment{
				Kind:     PathSegIndex,
				Index:    e.Index,
				NullSafe: e.NullSafe,
				Pos:      e.LBracketPos,
			})
			current = e.Receiver
			continue

		case *MethodCallExpr:
			// Intrinsic methods (catch, or) require special dispatch in the
			// interpreter (short-circuit evaluation, error interception) and
			// cannot be collapsed into PathExpr segments.
			if e.Method == "catch" || e.Method == "or" {
				return expr
			}
			segments = append(segments, PathSegment{
				Kind:     PathSegMethod,
				Name:     e.Method,
				Args:     e.Args,
				Named:    e.Named,
				NullSafe: e.NullSafe,
				Pos:      e.MethodPos,
			})
			current = e.Receiver
			continue

		case *InputExpr:
			if len(segments) == 0 {
				return expr
			}
			reverseSegments(segments)
			return &PathExpr{TokenPos: e.TokenPos, Root: PathRootInput, Segments: segments}

		case *InputMetaExpr:
			if len(segments) == 0 {
				return expr
			}
			reverseSegments(segments)
			return &PathExpr{TokenPos: e.TokenPos, Root: PathRootInputMeta, Segments: segments}

		case *OutputExpr:
			if len(segments) == 0 {
				return expr
			}
			reverseSegments(segments)
			return &PathExpr{TokenPos: e.TokenPos, Root: PathRootOutput, Segments: segments}

		case *OutputMetaExpr:
			if len(segments) == 0 {
				return expr
			}
			reverseSegments(segments)
			return &PathExpr{TokenPos: e.TokenPos, Root: PathRootOutputMeta, Segments: segments}

		case *VarExpr:
			if len(segments) == 0 {
				return expr
			}
			reverseSegments(segments)
			return &PathExpr{TokenPos: e.TokenPos, Root: PathRootVar, VarName: e.Name, Segments: segments}

		case *IdentExpr:
			// Bare identifier (parameter, match binding) — cannot collapse
			// because we don't have a PathRoot for bare identifiers.
			return expr

		default:
			// Non-collapsible root (call expression, literal, etc.)
			return expr
		}
	}
}

func reverseSegments(segs []PathSegment) {
	for i, j := 0, len(segs)-1; i < j; i, j = i+1, j-1 {
		segs[i], segs[j] = segs[j], segs[i]
	}
}

// -----------------------------------------------------------------------
// Constant folding
// -----------------------------------------------------------------------

// foldBinary attempts to evaluate a binary expression with literal operands
// at compile time. Returns nil if folding is not possible.
func (o *optimizer) foldBinary(e *BinaryExpr) Expr {
	left, lok := e.Left.(*LiteralExpr)
	right, rok := e.Right.(*LiteralExpr)
	if !lok || !rok {
		return nil
	}

	pos := left.TokenPos

	// String concatenation.
	if e.Op == PLUS && isStringLiteral(left) && isStringLiteral(right) {
		return &LiteralExpr{TokenPos: pos, TokenType: STRING, Value: left.Value + right.Value}
	}

	// Integer arithmetic.
	if left.TokenType == INT && right.TokenType == INT {
		a, aErr := strconv.ParseInt(left.Value, 10, 64)
		b, bErr := strconv.ParseInt(right.Value, 10, 64)
		if aErr != nil || bErr != nil {
			return nil
		}
		result, ok := foldIntOp(a, b, e.Op)
		if !ok {
			return nil // overflow or unsupported op
		}
		return &LiteralExpr{TokenPos: pos, TokenType: INT, Value: strconv.FormatInt(result, 10)}
	}

	// Float arithmetic (only when at least one operand is a float literal).
	// Skip folding if an integer operand exceeds 2^53 (would lose precision).
	if isNumericLiteral(left) && isNumericLiteral(right) &&
		(left.TokenType == FLOAT || right.TokenType == FLOAT) {
		if !canSafelyPromoteToFloat(left) || !canSafelyPromoteToFloat(right) {
			return nil // precision loss — let runtime handle the error
		}
		a, aOk := parseLiteralFloat(left)
		b, bOk := parseLiteralFloat(right)
		if !aOk || !bOk {
			return nil
		}
		result, ok := foldFloatOp(a, b, e.Op)
		if !ok {
			return nil
		}
		return &LiteralExpr{TokenPos: pos, TokenType: FLOAT, Value: strconv.FormatFloat(result, 'g', -1, 64)}
	}

	// Boolean logic.
	if isBoolLiteral(left) && isBoolLiteral(right) {
		a := left.TokenType == TRUE
		b := right.TokenType == TRUE
		var result bool
		switch e.Op {
		case AND:
			result = a && b
		case OR:
			result = a || b
		case EQ:
			result = a == b
		case NE:
			result = a != b
		default:
			return nil
		}
		return boolLiteral(pos, result)
	}

	// Equality of same-type literals.
	if e.Op == EQ || e.Op == NE {
		if left.TokenType == right.TokenType {
			eq := left.Value == right.Value
			if e.Op == NE {
				eq = !eq
			}
			return boolLiteral(pos, eq)
		}
		// Cross-type equality is always false.
		if isLiteralCrossType(left, right) {
			return boolLiteral(pos, e.Op == NE)
		}
	}

	return nil
}

// foldUnary attempts to evaluate a unary expression with a literal operand
// at compile time.
func (o *optimizer) foldUnary(e *UnaryExpr) Expr {
	lit, ok := e.Operand.(*LiteralExpr)
	if !ok {
		return nil
	}
	pos := lit.TokenPos

	switch e.Op {
	case BANG:
		if lit.TokenType == TRUE {
			return boolLiteral(pos, false)
		}
		if lit.TokenType == FALSE {
			return boolLiteral(pos, true)
		}
	case MINUS:
		if lit.TokenType == INT {
			n, err := strconv.ParseInt(lit.Value, 10, 64)
			if err != nil {
				return nil
			}
			if n == math.MinInt64 {
				return nil // -MinInt64 overflows
			}
			return &LiteralExpr{TokenPos: pos, TokenType: INT, Value: strconv.FormatInt(-n, 10)}
		}
		if lit.TokenType == FLOAT {
			f, err := strconv.ParseFloat(lit.Value, 64)
			if err != nil {
				return nil
			}
			return &LiteralExpr{TokenPos: pos, TokenType: FLOAT, Value: strconv.FormatFloat(-f, 'g', -1, 64)}
		}
	}
	return nil
}

// -----------------------------------------------------------------------
// Constant folding helpers
// -----------------------------------------------------------------------

// canSafelyPromoteToFloat checks whether an integer literal can be exactly
// represented as float64. Integers with magnitude > 2^53 cannot.
func canSafelyPromoteToFloat(l *LiteralExpr) bool {
	if l.TokenType == FLOAT {
		return true // already float
	}
	if l.TokenType != INT {
		return false
	}
	n, err := strconv.ParseInt(l.Value, 10, 64)
	if err != nil {
		return false
	}
	const maxSafeInt = int64(1 << 53)
	return n >= -maxSafeInt && n <= maxSafeInt
}

func isStringLiteral(l *LiteralExpr) bool {
	return l.TokenType == STRING || l.TokenType == RAW_STRING
}

func isNumericLiteral(l *LiteralExpr) bool {
	return l.TokenType == INT || l.TokenType == FLOAT
}

func isBoolLiteral(l *LiteralExpr) bool {
	return l.TokenType == TRUE || l.TokenType == FALSE
}

func parseLiteralFloat(l *LiteralExpr) (float64, bool) {
	f, err := strconv.ParseFloat(l.Value, 64)
	return f, err == nil
}

func boolLiteral(pos Pos, v bool) *LiteralExpr {
	if v {
		return &LiteralExpr{TokenPos: pos, TokenType: TRUE, Value: "true"}
	}
	return &LiteralExpr{TokenPos: pos, TokenType: FALSE, Value: "false"}
}

func isLiteralCrossType(a, b *LiteralExpr) bool {
	aFamily := literalFamily(a)
	bFamily := literalFamily(b)
	return aFamily != bFamily && aFamily != 0 && bFamily != 0
}

func literalFamily(l *LiteralExpr) int {
	switch l.TokenType {
	case INT, FLOAT:
		return 1 // numeric
	case STRING, RAW_STRING:
		return 2
	case TRUE, FALSE:
		return 3
	case NULL:
		return 4
	default:
		return 0
	}
}

func foldIntOp(a, b int64, op TokenType) (int64, bool) {
	switch op {
	case PLUS:
		r := a + b
		if (b > 0 && r < a) || (b < 0 && r > a) {
			return 0, false // overflow
		}
		return r, true
	case MINUS:
		r := a - b
		if (b > 0 && r > a) || (b < 0 && r < a) {
			return 0, false
		}
		return r, true
	case STAR:
		if a == 0 || b == 0 {
			return 0, true
		}
		r := a * b
		if r/a != b {
			return 0, false
		}
		return r, true
	case PERCENT:
		if b == 0 {
			return 0, false
		}
		return a % b, true
	default:
		return 0, false
	}
}

func foldFloatOp(a, b float64, op TokenType) (float64, bool) {
	switch op {
	case PLUS:
		return a + b, true
	case MINUS:
		return a - b, true
	case STAR:
		return a * b, true
	case SLASH:
		if b == 0 {
			return 0, false
		}
		return a / b, true
	case PERCENT:
		if b == 0 {
			return 0, false
		}
		return math.Mod(a, b), true
	default:
		return 0, false
	}
}
