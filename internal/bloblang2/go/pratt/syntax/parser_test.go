package syntax

import (
	"testing"
)

func mustParse(t *testing.T, src string) *Program {
	t.Helper()
	prog, errs := Parse(src, "", nil)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors:\n%s", FormatErrors(errs))
	}
	return prog
}

func expectError(t *testing.T, src string, substring string) {
	t.Helper()
	_, errs := Parse(src, "", nil)
	if len(errs) == 0 {
		t.Fatalf("expected parse error containing %q, but parsing succeeded", substring)
	}
	combined := FormatErrors(errs)
	for _, e := range errs {
		if contains(e.Msg, substring) {
			return
		}
	}
	t.Fatalf("no error contains %q, got:\n%s", substring, combined)
}

func contains(s, substr string) bool {
	return len(substr) == 0 || len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------
// Basic assignments
// -----------------------------------------------------------------------

func TestParse_SimpleAssignment(t *testing.T) {
	prog := mustParse(t, `output.x = 42`)
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Stmts))
	}
	assign, ok := prog.Stmts[0].(*Assignment)
	if !ok {
		t.Fatalf("expected *Assignment, got %T", prog.Stmts[0])
	}
	if assign.Target.Root != AssignOutput {
		t.Fatalf("expected AssignOutput root")
	}
	if len(assign.Target.Path) != 1 || assign.Target.Path[0].Name != "x" {
		t.Fatalf("expected path [x], got %v", assign.Target.Path)
	}
	lit, ok := assign.Value.(*LiteralExpr)
	if !ok || lit.Value != "42" {
		t.Fatalf("expected LiteralExpr(42), got %T %v", assign.Value, assign.Value)
	}
}

func TestParse_VarAssignment(t *testing.T) {
	prog := mustParse(t, `$x = 42`)
	assign := prog.Stmts[0].(*Assignment)
	if assign.Target.Root != AssignVar {
		t.Fatal("expected AssignVar root")
	}
	if assign.Target.VarName != "x" {
		t.Fatalf("expected var name 'x', got %q", assign.Target.VarName)
	}
}

func TestParse_MetadataAssignment(t *testing.T) {
	prog := mustParse(t, `output@.key = "value"`)
	assign := prog.Stmts[0].(*Assignment)
	if !assign.Target.MetaAccess {
		t.Fatal("expected metadata access")
	}
	if len(assign.Target.Path) != 1 || assign.Target.Path[0].Name != "key" {
		t.Fatal("expected path [key]")
	}
}

func TestParse_MultipleStatements(t *testing.T) {
	prog := mustParse(t, "output.a = 1\noutput.b = 2")
	if len(prog.Stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(prog.Stmts))
	}
}

// -----------------------------------------------------------------------
// Expressions
// -----------------------------------------------------------------------

func TestParse_BinaryExpr(t *testing.T) {
	prog := mustParse(t, `output = 1 + 2 * 3`)
	assign := prog.Stmts[0].(*Assignment)
	// Should be 1 + (2 * 3) due to precedence.
	bin, ok := assign.Value.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", assign.Value)
	}
	if bin.Op != PLUS {
		t.Fatalf("expected PLUS, got %s", bin.Op)
	}
	// Right should be 2 * 3.
	right, ok := bin.Right.(*BinaryExpr)
	if !ok || right.Op != STAR {
		t.Fatalf("expected right to be STAR, got %T %v", bin.Right, bin.Right)
	}
}

func TestParse_UnaryMinus(t *testing.T) {
	prog := mustParse(t, `output = -5`)
	assign := prog.Stmts[0].(*Assignment)
	unary, ok := assign.Value.(*UnaryExpr)
	if !ok || unary.Op != MINUS {
		t.Fatalf("expected UnaryExpr(-), got %T", assign.Value)
	}
}

func TestParse_MethodCallBindsTighterThanUnary(t *testing.T) {
	// -5.string() should parse as -(5.string())
	prog := mustParse(t, `output = -5.string()`)
	assign := prog.Stmts[0].(*Assignment)
	unary, ok := assign.Value.(*UnaryExpr)
	if !ok {
		t.Fatalf("expected UnaryExpr, got %T", assign.Value)
	}
	_, ok = unary.Operand.(*MethodCallExpr)
	if !ok {
		t.Fatalf("expected MethodCallExpr inside unary, got %T", unary.Operand)
	}
}

func TestParse_NonAssociativeChaining(t *testing.T) {
	expectError(t, `output = 1 < 2 < 3`, "chain")
	expectError(t, `output = 1 == 2 == 3`, "chain")
}

func TestParse_ComparisonBeforeEquality(t *testing.T) {
	// 3 > 2 == true is valid: (3 > 2) == true
	prog := mustParse(t, `output = 3 > 2 == true`)
	assign := prog.Stmts[0].(*Assignment)
	bin, ok := assign.Value.(*BinaryExpr)
	if !ok || bin.Op != EQ {
		t.Fatalf("expected outer EQ, got %T %v", assign.Value, assign.Value)
	}
}

// -----------------------------------------------------------------------
// Field access and method calls
// -----------------------------------------------------------------------

func TestParse_FieldAccess(t *testing.T) {
	prog := mustParse(t, `output = input.user.name`)
	assign := prog.Stmts[0].(*Assignment)
	fa, ok := assign.Value.(*FieldAccessExpr)
	if !ok {
		t.Fatalf("expected FieldAccessExpr, got %T", assign.Value)
	}
	if fa.Field != "name" {
		t.Fatalf("expected field 'name', got %q", fa.Field)
	}
}

func TestParse_NullSafeFieldAccess(t *testing.T) {
	prog := mustParse(t, `output = input?.name`)
	assign := prog.Stmts[0].(*Assignment)
	fa := assign.Value.(*FieldAccessExpr)
	if !fa.NullSafe {
		t.Fatal("expected null-safe")
	}
}

func TestParse_MethodCall(t *testing.T) {
	prog := mustParse(t, `output = input.name.uppercase()`)
	assign := prog.Stmts[0].(*Assignment)
	mc, ok := assign.Value.(*MethodCallExpr)
	if !ok {
		t.Fatalf("expected MethodCallExpr, got %T", assign.Value)
	}
	if mc.Method != "uppercase" {
		t.Fatalf("expected method 'uppercase', got %q", mc.Method)
	}
}

func TestParse_KeywordAsFieldName(t *testing.T) {
	// input.map is valid (map is a keyword but valid as field name after .)
	prog := mustParse(t, `output = input.map`)
	assign := prog.Stmts[0].(*Assignment)
	fa := assign.Value.(*FieldAccessExpr)
	if fa.Field != "map" {
		t.Fatalf("expected field 'map', got %q", fa.Field)
	}
}

func TestParse_IndexAccess(t *testing.T) {
	prog := mustParse(t, `output = input.items[0]`)
	assign := prog.Stmts[0].(*Assignment)
	idx, ok := assign.Value.(*IndexExpr)
	if !ok {
		t.Fatalf("expected IndexExpr, got %T", assign.Value)
	}
	if idx.NullSafe {
		t.Fatal("expected non-null-safe")
	}
}

func TestParse_NullSafeIndex(t *testing.T) {
	prog := mustParse(t, `output = input?[0]`)
	assign := prog.Stmts[0].(*Assignment)
	idx := assign.Value.(*IndexExpr)
	if !idx.NullSafe {
		t.Fatal("expected null-safe")
	}
}

// -----------------------------------------------------------------------
// Calls
// -----------------------------------------------------------------------

func TestParse_FunctionCall(t *testing.T) {
	prog := mustParse(t, `output = uuid_v4()`)
	assign := prog.Stmts[0].(*Assignment)
	call, ok := assign.Value.(*CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr, got %T", assign.Value)
	}
	if call.Name != "uuid_v4" || len(call.Args) != 0 {
		t.Fatalf("expected uuid_v4(), got %s(%d args)", call.Name, len(call.Args))
	}
}

func TestParse_QualifiedCall(t *testing.T) {
	prog := mustParse(t, `output = math::double(5)`)
	assign := prog.Stmts[0].(*Assignment)
	call := assign.Value.(*CallExpr)
	if call.Namespace != "math" || call.Name != "double" {
		t.Fatalf("expected math::double, got %s::%s", call.Namespace, call.Name)
	}
}

func TestParse_NamedArgs(t *testing.T) {
	prog := mustParse(t, `output = foo(a: 1, b: 2)`)
	assign := prog.Stmts[0].(*Assignment)
	call := assign.Value.(*CallExpr)
	if !call.Named {
		t.Fatal("expected named args")
	}
	if len(call.Args) != 2 || call.Args[0].Name != "a" || call.Args[1].Name != "b" {
		t.Fatalf("expected named args a, b")
	}
}

func TestParse_DeletedCall(t *testing.T) {
	prog := mustParse(t, `output = deleted()`)
	assign := prog.Stmts[0].(*Assignment)
	call, ok := assign.Value.(*CallExpr)
	if !ok || call.Name != "deleted" {
		t.Fatalf("expected CallExpr(deleted), got %T", assign.Value)
	}
}

func TestParse_ThrowCall(t *testing.T) {
	prog := mustParse(t, `output = throw("error")`)
	assign := prog.Stmts[0].(*Assignment)
	call := assign.Value.(*CallExpr)
	if call.Name != "throw" || len(call.Args) != 1 {
		t.Fatal("expected throw with 1 arg")
	}
}

// -----------------------------------------------------------------------
// Literals
// -----------------------------------------------------------------------

func TestParse_ArrayLiteral(t *testing.T) {
	prog := mustParse(t, `output = [1, 2, 3]`)
	assign := prog.Stmts[0].(*Assignment)
	arr, ok := assign.Value.(*ArrayLiteral)
	if !ok {
		t.Fatalf("expected ArrayLiteral, got %T", assign.Value)
	}
	if len(arr.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr.Elements))
	}
}

func TestParse_ObjectLiteral(t *testing.T) {
	prog := mustParse(t, `output = {"a": 1, "b": 2}`)
	assign := prog.Stmts[0].(*Assignment)
	obj, ok := assign.Value.(*ObjectLiteral)
	if !ok {
		t.Fatalf("expected ObjectLiteral, got %T", assign.Value)
	}
	if len(obj.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(obj.Entries))
	}
}

func TestParse_TrailingComma(t *testing.T) {
	// Trailing comma in array.
	mustParse(t, `output = [1, 2, 3,]`)
	// Trailing comma in object.
	mustParse(t, `output = {"a": 1, "b": 2,}`)
}

// -----------------------------------------------------------------------
// Lambdas
// -----------------------------------------------------------------------

func TestParse_SingleParamLambda(t *testing.T) {
	prog := mustParse(t, `output = input.map(x -> x * 2)`)
	assign := prog.Stmts[0].(*Assignment)
	mc := assign.Value.(*MethodCallExpr)
	lambda, ok := mc.Args[0].Value.(*LambdaExpr)
	if !ok {
		t.Fatalf("expected LambdaExpr, got %T", mc.Args[0].Value)
	}
	if len(lambda.Params) != 1 || lambda.Params[0].Name != "x" {
		t.Fatal("expected single param x")
	}
}

func TestParse_MultiParamLambda(t *testing.T) {
	prog := mustParse(t, `output = input.fold(0, (acc, x) -> acc + x)`)
	assign := prog.Stmts[0].(*Assignment)
	mc := assign.Value.(*MethodCallExpr)
	lambda, ok := mc.Args[1].Value.(*LambdaExpr)
	if !ok {
		t.Fatalf("expected LambdaExpr, got %T", mc.Args[1].Value)
	}
	if len(lambda.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(lambda.Params))
	}
}

func TestParse_DiscardParamLambda(t *testing.T) {
	prog := mustParse(t, `output = input.map(_ -> 42)`)
	assign := prog.Stmts[0].(*Assignment)
	mc := assign.Value.(*MethodCallExpr)
	lambda := mc.Args[0].Value.(*LambdaExpr)
	if !lambda.Params[0].Discard {
		t.Fatal("expected discard param")
	}
}

func TestParse_LambdaBlock(t *testing.T) {
	prog := mustParse(t, "output = input.map(x -> {\n  $y = x * 2\n  $y + 1\n})")
	assign := prog.Stmts[0].(*Assignment)
	mc := assign.Value.(*MethodCallExpr)
	lambda := mc.Args[0].Value.(*LambdaExpr)
	if len(lambda.Body.Assignments) != 1 {
		t.Fatalf("expected 1 var assignment in lambda body, got %d", len(lambda.Body.Assignments))
	}
}

func TestParse_GroupedExprNotLambda(t *testing.T) {
	// (1 + 2) is a grouped expression, not a lambda.
	prog := mustParse(t, `output = (1 + 2) * 3`)
	assign := prog.Stmts[0].(*Assignment)
	bin := assign.Value.(*BinaryExpr)
	if bin.Op != STAR {
		t.Fatalf("expected STAR, got %s", bin.Op)
	}
}

// -----------------------------------------------------------------------
// If expression
// -----------------------------------------------------------------------

func TestParse_IfExpr(t *testing.T) {
	prog := mustParse(t, `output = if true { 1 } else { 2 }`)
	assign := prog.Stmts[0].(*Assignment)
	ifExpr, ok := assign.Value.(*IfExpr)
	if !ok {
		t.Fatalf("expected IfExpr, got %T", assign.Value)
	}
	if len(ifExpr.Branches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(ifExpr.Branches))
	}
	if ifExpr.Else == nil {
		t.Fatal("expected else branch")
	}
}

func TestParse_IfExprWithoutElse(t *testing.T) {
	prog := mustParse(t, `output = if true { 1 }`)
	assign := prog.Stmts[0].(*Assignment)
	ifExpr := assign.Value.(*IfExpr)
	if ifExpr.Else != nil {
		t.Fatal("expected no else branch")
	}
}

func TestParse_IfElseIfElse(t *testing.T) {
	prog := mustParse(t, `output = if false { 1 } else if true { 2 } else { 3 }`)
	assign := prog.Stmts[0].(*Assignment)
	ifExpr := assign.Value.(*IfExpr)
	if len(ifExpr.Branches) != 2 {
		t.Fatalf("expected 2 branches (if + else-if), got %d", len(ifExpr.Branches))
	}
	if ifExpr.Else == nil {
		t.Fatal("expected else branch")
	}
}

// -----------------------------------------------------------------------
// If statement
// -----------------------------------------------------------------------

func TestParse_IfStmt(t *testing.T) {
	prog := mustParse(t, "if true {\n  output.x = 1\n}")
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Stmts))
	}
	ifStmt, ok := prog.Stmts[0].(*IfStmt)
	if !ok {
		t.Fatalf("expected IfStmt, got %T", prog.Stmts[0])
	}
	if len(ifStmt.Branches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(ifStmt.Branches))
	}
	if len(ifStmt.Branches[0].Body) != 1 {
		t.Fatalf("expected 1 statement in body, got %d", len(ifStmt.Branches[0].Body))
	}
}

// -----------------------------------------------------------------------
// Match expression
// -----------------------------------------------------------------------

func TestParse_MatchEqualityExpr(t *testing.T) {
	prog := mustParse(t, `output = match input.x { "a" => 1, "b" => 2, _ => 3 }`)
	assign := prog.Stmts[0].(*Assignment)
	matchExpr, ok := assign.Value.(*MatchExpr)
	if !ok {
		t.Fatalf("expected MatchExpr, got %T", assign.Value)
	}
	if len(matchExpr.Cases) != 3 {
		t.Fatalf("expected 3 cases, got %d", len(matchExpr.Cases))
	}
	if !matchExpr.Cases[2].Wildcard {
		t.Fatal("expected last case to be wildcard")
	}
}

func TestParse_MatchAsExpr(t *testing.T) {
	prog := mustParse(t, `output = match input.score as s { s >= 90 => "A", _ => "F" }`)
	assign := prog.Stmts[0].(*Assignment)
	matchExpr := assign.Value.(*MatchExpr)
	if matchExpr.Binding != "s" {
		t.Fatalf("expected binding 's', got %q", matchExpr.Binding)
	}
}

func TestParse_MatchBooleanExpr(t *testing.T) {
	// match { bool_cases }
	prog := mustParse(t, `output = match { input.x > 0 => "pos", _ => "neg" }`)
	assign := prog.Stmts[0].(*Assignment)
	matchExpr := assign.Value.(*MatchExpr)
	if matchExpr.Subject != nil {
		t.Fatal("expected no subject for boolean match")
	}
}

func TestParse_MatchCaseWithBracedBody(t *testing.T) {
	src := `output = match input.x {
  "a" => {
    $v = 1
    $v + 10
  },
  _ => 0,
}`
	prog := mustParse(t, src)
	assign := prog.Stmts[0].(*Assignment)
	matchExpr := assign.Value.(*MatchExpr)
	body, ok := matchExpr.Cases[0].Body.(*ExprBody)
	if !ok {
		t.Fatalf("expected *ExprBody for braced case, got %T", matchExpr.Cases[0].Body)
	}
	if len(body.Assignments) != 1 {
		t.Fatalf("expected 1 var assignment, got %d", len(body.Assignments))
	}
}

// -----------------------------------------------------------------------
// Map declarations
// -----------------------------------------------------------------------

func TestParse_MapDecl(t *testing.T) {
	prog := mustParse(t, "map double(x) {\n  x * 2\n}")
	if len(prog.Maps) != 1 {
		t.Fatalf("expected 1 map, got %d", len(prog.Maps))
	}
	m := prog.Maps[0]
	if m.Name != "double" {
		t.Fatalf("expected map name 'double', got %q", m.Name)
	}
	if len(m.Params) != 1 || m.Params[0].Name != "x" {
		t.Fatal("expected single param x")
	}
}

func TestParse_MapDeclWithDefaults(t *testing.T) {
	prog := mustParse(t, `map fmt(amount, currency = "USD") { currency + " " + amount.string() }`)
	m := prog.Maps[0]
	if len(m.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(m.Params))
	}
	if m.Params[1].Default == nil {
		t.Fatal("expected default for second param")
	}
}

func TestParse_MapDeclWithDiscard(t *testing.T) {
	prog := mustParse(t, `map ignore(_, x) { x }`)
	m := prog.Maps[0]
	if !m.Params[0].Discard {
		t.Fatal("expected first param to be discard")
	}
}

// -----------------------------------------------------------------------
// Imports
// -----------------------------------------------------------------------

func TestParse_Import(t *testing.T) {
	files := map[string]string{
		"helpers.blobl": `map double(x) { x * 2 }`,
	}
	prog, errs := Parse(`import "helpers.blobl" as h`+"\n"+`output = h::double(5)`, "", files)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors:\n%s", FormatErrors(errs))
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	if len(prog.Namespaces["h"]) != 1 {
		t.Fatalf("expected 1 map in namespace h, got %d", len(prog.Namespaces["h"]))
	}
}

func TestParse_ImportFileNotFound(t *testing.T) {
	_, errs := Parse(`import "missing.blobl" as m`, "", nil)
	if len(errs) == 0 {
		t.Fatal("expected error for missing file")
	}
}

func TestParse_CircularImport(t *testing.T) {
	files := map[string]string{
		"a.blobl": `import "b.blobl" as b`,
		"b.blobl": `import "a.blobl" as a`,
	}
	_, errs := Parse(`import "a.blobl" as a`, "", files)
	found := false
	for _, e := range errs {
		if containsStr(e.Msg, "circular") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected circular import error, got:\n%s", FormatErrors(errs))
	}
}

// -----------------------------------------------------------------------
// Discard _ as lambda parameter
// -----------------------------------------------------------------------

func TestParse_UnderscoreInExprIsError(t *testing.T) {
	expectError(t, `output = _ + 1`, "_")
}

// -----------------------------------------------------------------------
// Expression body with var assignments
// -----------------------------------------------------------------------

func TestParse_ExprBodyWithVarAssign(t *testing.T) {
	prog := mustParse(t, "output = if true {\n  $x = 10\n  $x + 1\n}")
	assign := prog.Stmts[0].(*Assignment)
	ifExpr := assign.Value.(*IfExpr)
	body := ifExpr.Branches[0].Body
	if len(body.Assignments) != 1 {
		t.Fatalf("expected 1 var assignment, got %d", len(body.Assignments))
	}
	if body.Assignments[0].Name != "x" {
		t.Fatalf("expected var name 'x', got %q", body.Assignments[0].Name)
	}
}

// -----------------------------------------------------------------------
// Input/output atoms
// -----------------------------------------------------------------------

func TestParse_InputAtom(t *testing.T) {
	prog := mustParse(t, `output = input`)
	assign := prog.Stmts[0].(*Assignment)
	_, ok := assign.Value.(*InputExpr)
	if !ok {
		t.Fatalf("expected InputExpr, got %T", assign.Value)
	}
}

func TestParse_InputMetaAtom(t *testing.T) {
	prog := mustParse(t, `output = input@`)
	assign := prog.Stmts[0].(*Assignment)
	_, ok := assign.Value.(*InputMetaExpr)
	if !ok {
		t.Fatalf("expected InputMetaExpr, got %T", assign.Value)
	}
}

func TestParse_OutputMetaAssignment(t *testing.T) {
	prog := mustParse(t, `output@ = {}`)
	assign := prog.Stmts[0].(*Assignment)
	if assign.Target.Root != AssignOutput || !assign.Target.MetaAccess {
		t.Fatal("expected output@ target")
	}
}

// -----------------------------------------------------------------------
// Error recovery
// -----------------------------------------------------------------------

func TestParse_ErrorRecovery(t *testing.T) {
	// First line has error, second line should still parse.
	prog, errs := Parse("output = @@@\noutput.x = 1", "", nil)
	if len(errs) == 0 {
		t.Fatal("expected errors")
	}
	// Should have recovered and parsed the second statement.
	if len(prog.Stmts) < 1 {
		t.Fatal("expected at least 1 statement after error recovery")
	}
}
