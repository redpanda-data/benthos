package v1ast

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

//
// YAML test-corpus loader
//

type corpusCase struct {
	Name         string    `yaml:"name"`
	Skip         string    `yaml:"skip"`
	Mapping      string    `yaml:"mapping"`
	CompileError string    `yaml:"compile_error"`
	Cases        []subCase `yaml:"cases"`
	// Files is intentionally untyped — it may be either a map or a slice
	// across the corpus, and we don't use it in these tests.
	Files any `yaml:"files"`
}

type subCase struct {
	Name string `yaml:"name"`
}

type corpusFileDoc struct {
	Tests []corpusCase `yaml:"tests"`
}

// corpusRoot returns the directory holding the V1 spec YAML tests.
func corpusRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "v1spec", "tests")
}

func listCorpusFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".yaml") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking corpus: %v", err)
	}
	sort.Strings(files)
	return files
}

// loadCorpusFile reads and decodes a single YAML file.
func loadCorpusFile(path string) ([]corpusCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc corpusFileDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc.Tests, nil
}

// TestParseCorpus: parse every non-skipped mapping. Report per-file stats
// and assert overall success rate >=95%.
func TestParseCorpus(t *testing.T) {
	root := corpusRoot(t)
	files := listCorpusFiles(t, root)
	if len(files) == 0 {
		t.Fatalf("no corpus files found under %s", root)
	}

	var total, passed, skipped, expectedFail int
	type failure struct {
		file, name string
		err        error
		src        string
	}
	var failures []failure

	for _, file := range files {
		cases, err := loadCorpusFile(file)
		if err != nil {
			t.Errorf("%s: load error: %v", file, err)
			continue
		}
		for _, c := range cases {
			if c.Skip != "" {
				skipped++
				continue
			}
			if strings.TrimSpace(c.Mapping) == "" {
				continue
			}
			total++
			_, perr := Parse(c.Mapping)
			if perr == nil {
				passed++
				continue
			}
			// A `compile_error` test case expects semantic failure, not
			// lexical failure, so we still consider parse success the
			// desired outcome in most cases. Bucket these separately so
			// the user sees the breakdown.
			if c.CompileError != "" {
				expectedFail++
				continue
			}
			failures = append(failures, failure{file: file, name: c.Name, err: perr, src: c.Mapping})
		}
	}

	// Per-file failure counts.
	perFile := map[string]int{}
	for _, f := range failures {
		perFile[f.file]++
	}
	keys := make([]string, 0, len(perFile))
	for k := range perFile {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return perFile[keys[i]] > perFile[keys[j]] })

	rate := float64(passed) / float64(total)
	t.Logf("corpus: %d total, %d passed (%.1f%%), %d failed, %d compile_error (ignored), %d skipped",
		total, passed, rate*100, len(failures), expectedFail, skipped)
	for _, k := range keys {
		rel, _ := filepath.Rel(root, k)
		t.Logf("  %d failures: %s", perFile[k], rel)
	}

	// Emit up to 15 failure samples for debugging.
	if len(failures) > 0 {
		show := len(failures)
		if show > 15 {
			show = 15
		}
		for i := 0; i < show; i++ {
			f := failures[i]
			rel, _ := filepath.Rel(root, f.file)
			t.Logf("-- %s :: %s --\n%s\nerror: %v", rel, f.name, f.src, f.err)
		}
	}

	if rate < 0.95 {
		t.Fatalf("corpus parse rate %.1f%% < 95%%", rate*100)
	}
}

// TestRoundTrip: parse corpus, print, re-parse, compare ASTs.
func TestRoundTrip(t *testing.T) {
	root := corpusRoot(t)
	files := listCorpusFiles(t, root)

	var total, passed int
	type failure struct {
		file, name string
		err        string
		src        string
		printed    string
	}
	var failures []failure

	for _, file := range files {
		cases, err := loadCorpusFile(file)
		if err != nil {
			continue
		}
		for _, c := range cases {
			if c.Skip != "" {
				continue
			}
			if strings.TrimSpace(c.Mapping) == "" {
				continue
			}
			first, perr := Parse(c.Mapping)
			if perr != nil {
				continue
			}
			total++
			printed := PrintString(first)
			second, perr := Parse(printed)
			if perr != nil {
				failures = append(failures, failure{
					file: file, name: c.Name,
					err: "re-parse: " + perr.Error(), src: c.Mapping, printed: printed,
				})
				continue
			}
			if diff, equal := astEqual(first, second); !equal {
				failures = append(failures, failure{
					file: file, name: c.Name, err: "AST diff: " + diff,
					src: c.Mapping, printed: printed,
				})
				continue
			}
			passed++
		}
	}

	rate := 1.0
	if total > 0 {
		rate = float64(passed) / float64(total)
	}
	t.Logf("roundtrip: %d/%d passed (%.1f%%)", passed, total, rate*100)
	if len(failures) > 0 {
		show := len(failures)
		if show > 15 {
			show = 15
		}
		for i := 0; i < show; i++ {
			f := failures[i]
			rel, _ := filepath.Rel(root, f.file)
			t.Logf("-- %s :: %s --\nORIG: %s\nPRINT: %s\n%s", rel, f.name, f.src, f.printed, f.err)
		}
	}
	if rate < 1.0 {
		t.Fatalf("roundtrip rate %.1f%% < 100%%", rate*100)
	}
}

// astEqual compares two programs structurally, ignoring Pos values.
// Returns a diff description on mismatch.
func astEqual(a, b *Program) (string, bool) {
	if diff, ok := nodeEqual(a, b, ""); !ok {
		return diff, false
	}
	return "", true
}

// nodeEqual is a best-effort reflection-based comparator that skips fields
// whose names contain "Pos".
func nodeEqual(a, b any, path string) (string, bool) {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if !va.IsValid() && !vb.IsValid() {
		return "", true
	}
	if va.IsValid() != vb.IsValid() {
		return fmt.Sprintf("%s: validity mismatch", path), false
	}
	if va.Kind() != vb.Kind() {
		return fmt.Sprintf("%s: kind %s vs %s", path, va.Kind(), vb.Kind()), false
	}
	switch va.Kind() {
	case reflect.Ptr, reflect.Interface:
		if va.IsNil() && vb.IsNil() {
			return "", true
		}
		if va.IsNil() != vb.IsNil() {
			return fmt.Sprintf("%s: nil mismatch", path), false
		}
		return nodeEqual(va.Elem().Interface(), vb.Elem().Interface(), path)
	case reflect.Slice:
		if va.Len() != vb.Len() {
			return fmt.Sprintf("%s: slice len %d vs %d", path, va.Len(), vb.Len()), false
		}
		for i := 0; i < va.Len(); i++ {
			if diff, ok := nodeEqual(va.Index(i).Interface(), vb.Index(i).Interface(),
				fmt.Sprintf("%s[%d]", path, i)); !ok {
				return diff, false
			}
		}
		return "", true
	case reflect.Struct:
		t := va.Type()
		for i := 0; i < va.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			// Skip positional metadata.
			if strings.Contains(f.Name, "Pos") {
				continue
			}
			if diff, ok := nodeEqual(va.Field(i).Interface(), vb.Field(i).Interface(),
				path+"."+f.Name); !ok {
				return diff, false
			}
		}
		return "", true
	case reflect.Map:
		if va.Len() != vb.Len() {
			return fmt.Sprintf("%s: map len %d vs %d", path, va.Len(), vb.Len()), false
		}
		keys := va.MapKeys()
		for _, k := range keys {
			bv := vb.MapIndex(k)
			if !bv.IsValid() {
				return fmt.Sprintf("%s: missing key %v", path, k.Interface()), false
			}
			if diff, ok := nodeEqual(va.MapIndex(k).Interface(), bv.Interface(),
				fmt.Sprintf("%s[%v]", path, k.Interface())); !ok {
				return diff, false
			}
		}
		return "", true
	default:
		if !reflect.DeepEqual(a, b) {
			return fmt.Sprintf("%s: %#v vs %#v", path, a, b), false
		}
		return "", true
	}
}

// TestUnit: small hand-crafted assertions for specific grammar quirks.
func TestUnit(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want func(t *testing.T, p *Program)
	}{
		{
			name: "root assignment with literal",
			src:  `root.foo = "bar"`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				if a.Target.Kind != TargetRoot {
					t.Fatalf("target kind = %v, want TargetRoot", a.Target.Kind)
				}
				if len(a.Target.Path) != 1 || a.Target.Path[0].Name != "foo" {
					t.Fatalf("target path = %+v", a.Target.Path)
				}
			},
		},
		{
			name: "this target preserved, not rewritten",
			src:  `this.foo = "bar"`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				if a.Target.Kind != TargetThis {
					t.Fatalf("target kind = %v, want TargetThis", a.Target.Kind)
				}
			},
		},
		{
			name: "bare-identifier target preserved",
			src:  `foo.bar = 1`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				if a.Target.Kind != TargetBare {
					t.Fatalf("target kind = %v, want TargetBare", a.Target.Kind)
				}
			},
		},
		{
			name: "let binding",
			src:  `let x = 5`,
			want: func(t *testing.T, p *Program) {
				if _, ok := p.Stmts[0].(*LetStmt); !ok {
					t.Fatalf("expected LetStmt, got %T", p.Stmts[0])
				}
			},
		},
		{
			name: "meta bare key assignment",
			src:  `meta foo = 1`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				if a.Target.Kind != TargetMeta {
					t.Fatalf("target kind = %v, want TargetMeta", a.Target.Kind)
				}
				if len(a.Target.Path) != 1 || a.Target.Path[0].Quoted {
					t.Fatalf("path = %+v", a.Target.Path)
				}
			},
		},
		{
			name: "meta quoted key assignment",
			src:  `meta "foo bar" = 1`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				if !a.Target.Path[0].Quoted || a.Target.Path[0].Name != "foo bar" {
					t.Fatalf("path = %+v", a.Target.Path)
				}
			},
		},
		{
			name: "meta whole replacement",
			src:  `meta = {"x": 1}`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				if len(a.Target.Path) != 0 {
					t.Fatalf("expected empty path for whole meta assign, got %+v", a.Target.Path)
				}
			},
		},
		{
			name: "variable reference",
			src:  `root = $x`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				if _, ok := a.Value.(*VarRef); !ok {
					t.Fatalf("value = %T, want *VarRef", a.Value)
				}
			},
		},
		{
			name: "bare @ reference",
			src:  `root = @`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				mr, ok := a.Value.(*MetaRef)
				if !ok || mr.Name != "" {
					t.Fatalf("value = %+v", a.Value)
				}
			},
		},
		{
			name: "meta(expr) call",
			src:  `root = meta("x")`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				if _, ok := a.Value.(*MetaCall); !ok {
					t.Fatalf("value = %T, want *MetaCall", a.Value)
				}
			},
		},
		{
			name: "arithmetic precedence: high-precedence |",
			src:  `root = a + b | c`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				bin, ok := a.Value.(*BinaryExpr)
				if !ok || bin.Op != TokPlus {
					t.Fatalf("expected top-level '+', got %T %+v", a.Value, a.Value)
				}
				if _, ok := bin.Right.(*BinaryExpr); !ok {
					t.Fatalf("right should be '|' BinaryExpr, got %T", bin.Right)
				}
			},
		},
		{
			name: "method chain",
			src:  `root = this.foo.uppercase()`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				if _, ok := a.Value.(*MethodCall); !ok {
					t.Fatalf("value = %T, want *MethodCall", a.Value)
				}
			},
		},
		{
			name: "lambda in method arg",
			src:  `root = items.map_each(x -> x.value)`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				mc, ok := a.Value.(*MethodCall)
				if !ok {
					t.Fatalf("expected method call, got %T", a.Value)
				}
				if len(mc.Args) != 1 {
					t.Fatalf("args = %+v", mc.Args)
				}
				if _, ok := mc.Args[0].Value.(*Lambda); !ok {
					t.Fatalf("arg = %T, want *Lambda", mc.Args[0].Value)
				}
			},
		},
		{
			name: ".(expr) map expression",
			src:  `root = this.thing.(article | comment)`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				if _, ok := a.Value.(*MapExpr); !ok {
					t.Fatalf("value = %T, want *MapExpr", a.Value)
				}
			},
		},
		{
			name: "if expression",
			src:  `root = if this.x > 0 { "big" } else { "small" }`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				if _, ok := a.Value.(*IfExpr); !ok {
					t.Fatalf("value = %T, want *IfExpr", a.Value)
				}
			},
		},
		{
			name: "match expression",
			src: "root = match this {\n" +
				"  \"a\" => 1\n" +
				"  _ => 2\n" +
				"}",
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				m, ok := a.Value.(*MatchExpr)
				if !ok {
					t.Fatalf("value = %T, want *MatchExpr", a.Value)
				}
				if len(m.Cases) != 2 {
					t.Fatalf("cases = %+v", m.Cases)
				}
			},
		},
		{
			name: "array literal",
			src:  `root = [1, 2, 3]`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				if _, ok := a.Value.(*ArrayLit); !ok {
					t.Fatalf("value = %T", a.Value)
				}
			},
		},
		{
			name: "object literal",
			src:  `root = {"a": 1, "b": 2}`,
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				if _, ok := a.Value.(*ObjectLit); !ok {
					t.Fatalf("value = %T", a.Value)
				}
			},
		},
		{
			name: "map decl",
			src: "map greet {\n" +
				"  root = \"hi\"\n" +
				"}",
			want: func(t *testing.T, p *Program) {
				if len(p.Maps) != 1 || p.Maps[0].Name != "greet" {
					t.Fatalf("maps = %+v", p.Maps)
				}
			},
		},
		{
			name: "import",
			src:  `import "./lib.blobl"`,
			want: func(t *testing.T, p *Program) {
				if len(p.Imports) != 1 {
					t.Fatalf("imports = %+v", p.Imports)
				}
			},
		},
		{
			name: "triple-quoted string preserved raw",
			src:  "root = \"\"\"a\\nb\"\"\"",
			want: func(t *testing.T, p *Program) {
				a := firstAssign(t, p)
				lit, ok := a.Value.(*Literal)
				if !ok || lit.Kind != LitRawString {
					t.Fatalf("value = %+v", a.Value)
				}
				if lit.Str != `a\nb` {
					t.Fatalf("raw body = %q, want %q", lit.Str, `a\nb`)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := Parse(tc.src)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			tc.want(t, prog)
		})
	}
}

func firstAssign(t *testing.T, p *Program) *Assignment {
	t.Helper()
	if len(p.Stmts) == 0 {
		t.Fatalf("no statements parsed")
	}
	a, ok := p.Stmts[0].(*Assignment)
	if !ok {
		t.Fatalf("stmt[0] = %T, want *Assignment", p.Stmts[0])
	}
	return a
}

// TestParseErrorCases covers specific quirks that MUST be parse errors.
func TestParseErrorCases(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"double-not", `root = !!x`},
		{"this[0] bracket indexing", `root = this[0]`},
		{"equals without spaces", `root.a=1`},
		{"equals no space right", `root.a =1`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.src)
			if err == nil {
				t.Fatalf("expected parse error for %q", tc.src)
			}
		})
	}
}
