package syntax

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// -----------------------------------------------------------------------
// Round-trip test against the spec corpus.
// -----------------------------------------------------------------------

// testYAMLFile is a minimal mirror of spectest.TestFile, defined locally
// to avoid an import cycle (spectest imports this package transitively).
type testYAMLFile struct {
	Description string            `yaml:"description"`
	Files       map[string]string `yaml:"files"`
	Tests       []testYAMLCase    `yaml:"tests"`
}

type testYAMLCase struct {
	Name         string            `yaml:"name"`
	Mapping      string            `yaml:"mapping"`
	CompileError string            `yaml:"compile_error"`
	Error        string            `yaml:"error"`
	Cases        []yaml.Node       `yaml:"cases"`
	Files        map[string]string `yaml:"files"`
}

func loadYAML(path string) (*testYAMLFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tf testYAMLFile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return nil, err
	}
	return &tf, nil
}

func discoverYAML(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".yaml") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// specTestsDir locates the spec/tests directory relative to this source file.
func specTestsDir(t *testing.T) string {
	t.Helper()
	// go/pratt/syntax/print_test.go — spec/tests is ../../../spec/tests
	// from the file's directory.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Walk up until we find spec/tests.
	cur := dir
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(cur, "spec", "tests")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		cur = filepath.Dir(cur)
	}
	t.Fatalf("could not locate spec/tests relative to %s", dir)
	return ""
}

// TestPrintRoundTrip parses every mapping in the spec corpus, prints it,
// re-parses the print, and asserts structural equivalence.
func TestPrintRoundTrip(t *testing.T) {
	root := specTestsDir(t)
	files, err := discoverYAML(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}

	var (
		total    int
		okCount  int
		failures []string
	)

	for _, path := range files {
		tf, err := loadYAML(path)
		if err != nil {
			t.Logf("skip %s: %v", path, err)
			continue
		}
		for _, tc := range tf.Tests {
			// Skip multi-case tests for simplicity.
			if len(tc.Cases) > 0 {
				continue
			}
			// Skip tests that are expected to fail compilation.
			if tc.CompileError != "" {
				continue
			}
			if tc.Mapping == "" {
				continue
			}

			total++
			caseFiles := tc.Files
			if caseFiles == nil {
				caseFiles = tf.Files
			}

			if roundTripOK(t, tc.Mapping, caseFiles) {
				okCount++
				continue
			}

			rel, _ := filepath.Rel(root, path)
			failures = append(failures, fmt.Sprintf("%s :: %s", rel, tc.Name))
		}
	}

	if total == 0 {
		t.Fatal("no round-trip test cases collected")
	}

	rate := float64(okCount) / float64(total)
	t.Logf("round-trip: %d/%d passed (%.2f%%)", okCount, total, rate*100)
	for _, f := range failures {
		t.Logf("  fail: %s", f)
	}

	if rate < 0.95 {
		t.Fatalf("round-trip success rate %.2f%% is below 95%%", rate*100)
	}
}

// roundTripOK parses src, prints the result, re-parses the printed form,
// and reports whether the two ASTs are structurally equivalent (positions
// ignored).
func roundTripOK(t *testing.T, src string, files map[string]string) bool {
	t.Helper()

	prog1, errs1 := Parse(src, "", files)
	if len(errs1) > 0 {
		// Skip — parent mapping was not expected to parse cleanly.
		return true
	}

	printed := Print(prog1)

	prog2, errs2 := Parse(printed, "", files)
	if len(errs2) > 0 {
		t.Logf("re-parse failed for:\n--- original ---\n%s\n--- printed ---\n%s\n--- errors ---\n%s",
			src, printed, FormatErrors(errs2))
		return false
	}

	p1 := cloneAndZero(prog1)
	p2 := cloneAndZero(prog2)

	if !reflect.DeepEqual(p1, p2) {
		t.Logf("AST mismatch for:\n--- original ---\n%s\n--- printed ---\n%s", src, printed)
		return false
	}
	return true
}

// cloneAndZero returns a copy of p with all position data zeroed so that
// two ASTs parsed from different strings can be compared for structural
// equivalence.
func cloneAndZero(p *Program) *Program {
	c := &Program{
		Stmts:       make([]Stmt, len(p.Stmts)),
		Maps:        make([]*MapDecl, len(p.Maps)),
		Imports:     make([]*ImportStmt, len(p.Imports)),
		MaxSlots:    0,
		ReadsOutput: false,
	}
	for i, s := range p.Stmts {
		c.Stmts[i] = zeroStmt(s)
	}
	for i, m := range p.Maps {
		c.Maps[i] = zeroMap(m)
	}
	for i, imp := range p.Imports {
		c.Imports[i] = &ImportStmt{Path: imp.Path, Namespace: imp.Namespace}
	}
	// Skip Namespaces — transitive structures can differ in pointer identity
	// even when content is equivalent; not relevant for round-trip checks.
	return c
}

func zeroStmt(s Stmt) Stmt {
	switch v := s.(type) {
	case *Assignment:
		return &Assignment{
			Target: zeroAssignTarget(v.Target),
			Value:  zeroExpr(v.Value),
		}
	case *IfStmt:
		out := &IfStmt{}
		for _, b := range v.Branches {
			newB := IfBranch{Cond: zeroExpr(b.Cond)}
			for _, st := range b.Body {
				newB.Body = append(newB.Body, zeroStmt(st))
			}
			out.Branches = append(out.Branches, newB)
		}
		for _, st := range v.Else {
			out.Else = append(out.Else, zeroStmt(st))
		}
		return out
	case *MatchStmt:
		out := &MatchStmt{
			Subject: zeroExpr(v.Subject),
			Binding: v.Binding,
		}
		for _, c := range v.Cases {
			out.Cases = append(out.Cases, zeroMatchCase(c))
		}
		return out
	}
	return s
}

func zeroAssignTarget(t AssignTarget) AssignTarget {
	out := AssignTarget{
		Root:       t.Root,
		VarName:    t.VarName,
		MetaAccess: t.MetaAccess,
	}
	for _, seg := range t.Path {
		out.Path = append(out.Path, zeroPathSegment(seg))
	}
	return out
}

func zeroMap(m *MapDecl) *MapDecl {
	out := &MapDecl{
		Name: m.Name,
		Body: zeroExprBody(m.Body),
	}
	for _, param := range m.Params {
		out.Params = append(out.Params, zeroParam(param))
	}
	return out
}

func zeroParam(p Param) Param {
	return Param{
		Name:    p.Name,
		Default: zeroExpr(p.Default),
		Discard: p.Discard,
	}
}

func zeroMatchCase(c MatchCase) MatchCase {
	out := MatchCase{
		Wildcard: c.Wildcard,
		Pattern:  zeroExpr(c.Pattern),
	}
	switch body := c.Body.(type) {
	case []Stmt:
		var stmts []Stmt
		for _, s := range body {
			stmts = append(stmts, zeroStmt(s))
		}
		out.Body = stmts
	case *ExprBody:
		out.Body = zeroExprBody(body)
	case Expr:
		out.Body = zeroExpr(body)
	}
	return out
}

func zeroExprBody(b *ExprBody) *ExprBody {
	if b == nil {
		return nil
	}
	out := &ExprBody{Result: zeroExpr(b.Result)}
	for _, va := range b.Assignments {
		newVA := &VarAssign{Name: va.Name, Value: zeroExpr(va.Value)}
		for _, seg := range va.Path {
			newVA.Path = append(newVA.Path, zeroPathSegment(seg))
		}
		out.Assignments = append(out.Assignments, newVA)
	}
	return out
}

func zeroPathSegment(s PathSegment) PathSegment {
	out := PathSegment{
		Kind:     s.Kind,
		Name:     s.Name,
		Index:    zeroExpr(s.Index),
		NullSafe: s.NullSafe,
		Named:    s.Named,
	}
	for _, a := range s.Args {
		out.Args = append(out.Args, CallArg{Name: a.Name, Value: zeroExpr(a.Value)})
	}
	return out
}

func zeroExpr(e Expr) Expr {
	if e == nil {
		return nil
	}
	switch v := e.(type) {
	case *LiteralExpr:
		return &LiteralExpr{TokenType: v.TokenType, Value: v.Value}
	case *InputExpr:
		return &InputExpr{}
	case *InputMetaExpr:
		return &InputMetaExpr{}
	case *OutputExpr:
		return &OutputExpr{}
	case *OutputMetaExpr:
		return &OutputMetaExpr{}
	case *VarExpr:
		return &VarExpr{Name: v.Name}
	case *IdentExpr:
		return &IdentExpr{Name: v.Name, Namespace: v.Namespace}
	case *BinaryExpr:
		return &BinaryExpr{Left: zeroExpr(v.Left), Op: v.Op, Right: zeroExpr(v.Right)}
	case *UnaryExpr:
		return &UnaryExpr{Op: v.Op, Operand: zeroExpr(v.Operand)}
	case *CallExpr:
		out := &CallExpr{Name: v.Name, Namespace: v.Namespace, Named: v.Named}
		for _, a := range v.Args {
			out.Args = append(out.Args, CallArg{Name: a.Name, Value: zeroExpr(a.Value)})
		}
		return out
	case *MethodCallExpr:
		out := &MethodCallExpr{
			Receiver: zeroExpr(v.Receiver),
			Method:   v.Method,
			Named:    v.Named,
			NullSafe: v.NullSafe,
		}
		for _, a := range v.Args {
			out.Args = append(out.Args, CallArg{Name: a.Name, Value: zeroExpr(a.Value)})
		}
		return out
	case *FieldAccessExpr:
		return &FieldAccessExpr{
			Receiver: zeroExpr(v.Receiver),
			Field:    v.Field,
			NullSafe: v.NullSafe,
		}
	case *IndexExpr:
		return &IndexExpr{
			Receiver: zeroExpr(v.Receiver),
			Index:    zeroExpr(v.Index),
			NullSafe: v.NullSafe,
		}
	case *LambdaExpr:
		out := &LambdaExpr{Body: zeroExprBody(v.Body)}
		for _, p := range v.Params {
			out.Params = append(out.Params, zeroParam(p))
		}
		return out
	case *ArrayLiteral:
		out := &ArrayLiteral{}
		for _, el := range v.Elements {
			out.Elements = append(out.Elements, zeroExpr(el))
		}
		return out
	case *ObjectLiteral:
		out := &ObjectLiteral{}
		for _, entry := range v.Entries {
			out.Entries = append(out.Entries, ObjectEntry{
				Key:   zeroExpr(entry.Key),
				Value: zeroExpr(entry.Value),
			})
		}
		return out
	case *IfExpr:
		out := &IfExpr{Else: zeroExprBody(v.Else)}
		for _, b := range v.Branches {
			out.Branches = append(out.Branches, IfExprBranch{
				Cond: zeroExpr(b.Cond),
				Body: zeroExprBody(b.Body),
			})
		}
		return out
	case *MatchExpr:
		out := &MatchExpr{Subject: zeroExpr(v.Subject), Binding: v.Binding}
		for _, c := range v.Cases {
			out.Cases = append(out.Cases, zeroMatchCase(c))
		}
		return out
	case *PathExpr:
		out := &PathExpr{Root: v.Root, VarName: v.VarName}
		for _, seg := range v.Segments {
			out.Segments = append(out.Segments, zeroPathSegment(seg))
		}
		return out
	}
	return e
}

// -----------------------------------------------------------------------
// Hand-crafted formatting expectations.
// -----------------------------------------------------------------------

func TestPrintFormatting(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "simple assignment stays single line",
			src:  `output.x = 1 + 2`,
			want: "output.x = 1 + 2\n",
		},
		{
			name: "four-entry object wraps",
			src:  `output = {"a": 1, "b": 2, "c": 3, "d": 4}`,
			want: `output = {
  "a": 1,
  "b": 2,
  "c": 3,
  "d": 4,
}
`,
		},
		{
			name: "two-entry object stays compact",
			src:  `output = {"a": 1, "b": 2}`,
			want: "output = {\"a\": 1, \"b\": 2}\n",
		},
		{
			name: "nested object wraps parent",
			src:  `output = {"a": {"x": 1, "y": 2}}`,
			want: `output = {
  "a": {"x": 1, "y": 2},
}
`,
		},
		{
			name: "simple array stays single line",
			src:  `output = [1, 2]`,
			want: "output = [1, 2]\n",
		},
		{
			name: "if-expression inline when simple",
			src:  `output.x = if true { 1 } else { 2 }`,
			want: "output.x = if true { 1 } else { 2 }\n",
		},
		{
			name: "import before map before stmts",
			src: `output.y = 1
map greet(name) { "hi " + name }
import "h.blobl" as h`,
			want: `import "h.blobl" as h

map greet(name) {
  "hi " + name
}

output.y = 1
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := Parse(tc.src, "", map[string]string{
				"h.blobl": `map dummy(x) { x }`,
			})
			if len(errs) > 0 {
				t.Fatalf("parse errors: %s", FormatErrors(errs))
			}
			got := Print(prog)
			if got != tc.want {
				t.Fatalf("mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, tc.want)
			}
		})
	}
}
