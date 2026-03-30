package agentexam

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestRunRoundTrip(t *testing.T) {
	mockAgent := &AgentFunc{
		Fn: func(_ context.Context, dir, _ string, _ io.Writer) error {
			data, err := os.ReadFile(filepath.Join(dir, "input.json"))
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, "output.json"), data, 0o644)
		},
		Label: "mock-copier",
	}

	exam := &Exam{
		Name:   "test-roundtrip",
		Files:  map[string][]byte{"input.json": []byte(`{"value":42}`)},
		Prompt: "Copy input.json to output.json",
		Score: func(_ context.Context, room *Room, _ io.Writer) ([]Result, error) {
			got, ok := room.GetFile("output.json")
			if !ok {
				return []Result{{ID: "copy", Name: "copy input", Score: 0, Error: "output.json missing"}}, nil
			}
			expected := `{"value":42}`
			if strings.TrimSpace(got) != expected {
				return []Result{{
					ID: "copy", Name: "copy input", Score: 0,
					Error: "mismatch: got " + got,
				}}, nil
			}
			return []Result{{ID: "copy", Name: "copy input", Score: 1}}, nil
		},
	}

	results, err := Run(context.Background(), exam, &Options{
		Agent: mockAgent,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Score != 1 {
		t.Errorf("expected score 1, got: %f (%s)", results[0].Score, results[0].Error)
	}
}

func nopAgent() *AgentFunc {
	return &AgentFunc{
		Fn:    func(_ context.Context, _, _ string, _ io.Writer) error { return nil },
		Label: "nop",
	}
}

func TestRunAllMultipleExams(t *testing.T) {
	exams := []*Exam{
		{
			Name: "exam-a", Files: map[string][]byte{}, Prompt: "a",
			Score: func(context.Context, *Room, io.Writer) ([]Result, error) {
				return []Result{{ID: "a1", Score: 1}}, nil
			},
		},
		{
			Name: "exam-b", Files: map[string][]byte{}, Prompt: "b",
			Score: func(context.Context, *Room, io.Writer) ([]Result, error) {
				return []Result{
					{ID: "b1", Score: 1},
					{ID: "b2", Score: 0, Error: "wrong"},
				}, nil
			},
		},
	}

	all, err := RunAll(context.Background(), exams, &Options{Agent: nopAgent()})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 exam results, got %d", len(all))
	}
	if len(all["exam-a"]) != 1 {
		t.Errorf("exam-a: expected 1 result, got %d", len(all["exam-a"]))
	}
	if len(all["exam-b"]) != 2 {
		t.Errorf("exam-b: expected 2 results, got %d", len(all["exam-b"]))
	}
}

func TestSummarize(t *testing.T) {
	results := []Result{
		{ID: "a1", Group: "math", Score: 1},
		{ID: "a2", Group: "math", Score: 0.5, Error: "partial"},
		{ID: "a3", Group: "strings", Score: 1},
		{ID: "a4", Score: 0.75}, // ungrouped
	}

	s := Summarize(results)
	if s.Total != 4 {
		t.Errorf("total: got %d, want 4", s.Total)
	}
	const wantTotal = 3.25
	if s.TotalScore != wantTotal {
		t.Errorf("total score: got %f, want %f", s.TotalScore, wantTotal)
	}
	if g, ok := s.Groups["math"]; !ok || g.Total != 2 || g.TotalScore != 1.5 {
		t.Errorf("math group: got %+v", s.Groups["math"])
	}
	if g, ok := s.Groups["strings"]; !ok || g.Total != 1 || g.TotalScore != 1 {
		t.Errorf("strings group: got %+v", s.Groups["strings"])
	}
	if _, ok := s.Groups[""]; ok {
		t.Error("ungrouped results should not appear in Groups")
	}
}

func TestDirFiles(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("bbb"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := DirFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if string(files["a.txt"]) != "aaa" {
		t.Errorf("a.txt: got %q", files["a.txt"])
	}
	if string(files[filepath.Join("sub", "b.txt")]) != "bbb" {
		t.Errorf("sub/b.txt: got %q", files[filepath.Join("sub", "b.txt")])
	}
}

func TestPrintTable(t *testing.T) {
	results := []Result{
		{ID: "a1", Group: "math", Score: 1},
		{ID: "a2", Group: "math", Score: 0},
		{ID: "a3", Group: "strings", Score: 1},
	}

	var buf bytes.Buffer
	PrintTable(&buf, results)
	out := buf.String()

	for _, want := range []string{"math", "strings", "TOTAL", "50.0%", "100.0%"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintComparisonTable(t *testing.T) {
	runs := map[string][]Result{
		"run-a": {
			{ID: "1", Group: "g1", Score: 1},
			{ID: "2", Group: "g1", Score: 1},
		},
		"run-b": {
			{ID: "1", Group: "g1", Score: 1},
			{ID: "2", Group: "g1", Score: 0},
		},
	}

	var buf bytes.Buffer
	PrintComparisonTable(&buf, runs)
	out := buf.String()

	if !strings.Contains(out, "run-a") || !strings.Contains(out, "run-b") {
		t.Errorf("output missing run names:\n%s", out)
	}
	if !strings.Contains(out, "100.0%") || !strings.Contains(out, "50.0%") {
		t.Errorf("output missing expected percentages:\n%s", out)
	}
}

func TestStableWorkDir(t *testing.T) {
	a := stableWorkDir("exam-1")
	b := stableWorkDir("exam-1")
	c := stableWorkDir("exam-2")

	if a != b {
		t.Errorf("same name should produce same dir: %s vs %s", a, b)
	}
	if a == c {
		t.Error("different names should produce different dirs")
	}
	if !strings.Contains(a, "agentexam-") {
		t.Errorf("dir should contain agentexam- prefix: %s", a)
	}
}

func TestWriteFilesCreatesSubdirs(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		"a.txt":                []byte("a"),
		"sub/dir/deep/file.go": []byte("package main"),
	}

	if err := writeFiles(dir, files); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "sub", "dir", "deep", "file.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package main" {
		t.Errorf("got %q", got)
	}
}

func TestRunNilAgent(t *testing.T) {
	exam := &Exam{
		Name: "nil-agent", Files: map[string][]byte{}, Prompt: "x",
		Score: func(context.Context, *Room, io.Writer) ([]Result, error) { return nil, nil },
	}
	_, err := Run(context.Background(), exam, &Options{})
	if err == nil || !strings.Contains(err.Error(), "Agent is required") {
		t.Errorf("expected agent required error, got: %v", err)
	}
}

func TestTHelper(t *testing.T) {
	exam := &Exam{
		Name: "t-helper", Files: map[string][]byte{}, Prompt: "x",
		Score: func(context.Context, *Room, io.Writer) ([]Result, error) {
			return []Result{
				{ID: "pass-item", Name: "should pass", Score: 1},
			}, nil
		},
	}

	T(t, exam, &Options{Agent: nopAgent()})
}

func TestDirFilesDeterministic(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"c.txt", "a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := DirFiles(root)
	if err != nil {
		t.Fatal(err)
	}

	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	want := []string{"a.txt", "b.txt", "c.txt"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("keys: got %v, want %v", keys, want)
	}
}

func TestRoomGetFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	room := &Room{dir: dir}

	got, ok := room.GetFile("hello.txt")
	if !ok {
		t.Fatal("expected file to exist")
	}
	if got != "world" {
		t.Errorf("got %q, want %q", got, "world")
	}

	_, ok = room.GetFile("missing.txt")
	if ok {
		t.Error("expected missing file to return false")
	}
}

func TestRoomListFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}

	room := &Room{dir: dir}
	got := room.ListFiles()
	want := []string{"a.txt", "b.txt", filepath.Join("sub", "c.txt")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListFiles: got %v, want %v", got, want)
	}
}

func TestRoomDir(t *testing.T) {
	dir := t.TempDir()
	room := &Room{dir: dir}
	if room.Dir() != dir {
		t.Errorf("Dir: got %q, want %q", room.Dir(), dir)
	}
}

func TestDebugBanner(t *testing.T) {
	var buf bytes.Buffer
	exam := &Exam{
		Name:   "banner-test",
		Files:  map[string][]byte{"a.txt": []byte("a"), "b.txt": []byte("b")},
		Prompt: "do nothing",
		Score: func(context.Context, *Room, io.Writer) ([]Result, error) {
			return nil, nil
		},
	}

	_, err := Run(context.Background(), exam, &Options{
		Agent:  nopAgent(),
		Output: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{
		"=== exam: banner-test ===",
		"agent:     nop",
		"files:     2",
		"=== scoring: banner-test ===",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("banner missing %q:\n%s", want, out)
		}
	}
}

func TestRunWithTimeout(t *testing.T) {
	slowAgent := &AgentFunc{
		Fn: func(ctx context.Context, _, _ string, _ io.Writer) error {
			<-ctx.Done()
			return ctx.Err()
		},
		Label: "slow",
	}

	exam := &Exam{
		Name: "timeout-test", Files: map[string][]byte{}, Prompt: "x",
		Score: func(context.Context, *Room, io.Writer) ([]Result, error) {
			return nil, nil
		},
	}

	_, err := Run(context.Background(), exam, &Options{
		Agent:   slowAgent,
		Timeout: 50 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "agent run") {
		t.Errorf("expected agent run error, got: %v", err)
	}
}

func TestRunAgentError(t *testing.T) {
	failAgent := &AgentFunc{
		Fn:    func(context.Context, string, string, io.Writer) error { return errors.New("boom") },
		Label: "fail",
	}

	exam := &Exam{
		Name: "fail-test", Files: map[string][]byte{}, Prompt: "x",
		Score: func(context.Context, *Room, io.Writer) ([]Result, error) {
			return nil, nil
		},
	}

	_, err := Run(context.Background(), exam, &Options{Agent: failAgent})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected boom error, got: %v", err)
	}
}

func TestRunKeepDir(t *testing.T) {
	dir := t.TempDir()
	exam := &Exam{
		Name: "keepdir-test", Files: map[string][]byte{"a.txt": []byte("a")}, Prompt: "x",
		Score: func(context.Context, *Room, io.Writer) ([]Result, error) {
			return []Result{{ID: "ok", Score: 1}}, nil
		},
	}

	workDir := filepath.Join(dir, "work")
	_, err := Run(context.Background(), exam, &Options{
		Agent:   nopAgent(),
		WorkDir: workDir,
		KeepDir: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Dir should still exist.
	if _, err := os.Stat(workDir); err != nil {
		t.Errorf("work dir should still exist: %v", err)
	}

	// File should be there.
	data, err := os.ReadFile(filepath.Join(workDir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "a" {
		t.Errorf("got %q", data)
	}
}

func TestRunCleansDirBetweenRuns(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")

	makeExam := func(files map[string][]byte) *Exam {
		return &Exam{
			Name: "stale-test", Files: files, Prompt: "x",
			Score: func(_ context.Context, room *Room, _ io.Writer) ([]Result, error) {
				return []Result{{ID: "ok", Score: 1}}, nil
			},
		}
	}

	// First run writes stale.txt.
	_, err := Run(context.Background(), makeExam(map[string][]byte{"stale.txt": []byte("old")}), &Options{
		Agent: nopAgent(), WorkDir: workDir, KeepDir: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Second run with different files.
	_, err = Run(context.Background(), makeExam(map[string][]byte{"fresh.txt": []byte("new")}), &Options{
		Agent: nopAgent(), WorkDir: workDir, KeepDir: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// stale.txt should be gone.
	if _, err := os.Stat(filepath.Join(workDir, "stale.txt")); !os.IsNotExist(err) {
		t.Error("stale.txt should have been cleaned up")
	}
	// fresh.txt should exist.
	if _, err := os.Stat(filepath.Join(workDir, "fresh.txt")); err != nil {
		t.Errorf("fresh.txt should exist: %v", err)
	}
}

func TestRunAllPartialError(t *testing.T) {
	exams := []*Exam{
		{
			Name: "pass", Files: map[string][]byte{}, Prompt: "x",
			Score: func(context.Context, *Room, io.Writer) ([]Result, error) {
				return []Result{{ID: "p1", Score: 1}}, nil
			},
		},
		{
			Name: "fail", Files: map[string][]byte{}, Prompt: "x",
			Score: func(context.Context, *Room, io.Writer) ([]Result, error) {
				return nil, errors.New("scoring failed")
			},
		},
	}

	out, err := RunAll(context.Background(), exams, &Options{Agent: nopAgent()})
	if err == nil || !strings.Contains(err.Error(), "scoring failed") {
		t.Errorf("expected scoring error, got: %v", err)
	}
	// First exam results should still be in the map.
	if len(out["pass"]) != 1 {
		t.Errorf("expected pass results, got: %v", out)
	}
}

func TestFormatStatsZero(t *testing.T) {
	var buf bytes.Buffer
	PrintComparisonTable(&buf, map[string][]Result{"empty": {}})
	if !strings.Contains(buf.String(), "N/A") {
		t.Errorf("expected N/A for empty results:\n%s", buf.String())
	}
}

func TestAgentFuncString(t *testing.T) {
	a := &AgentFunc{Fn: nil, Label: "my-agent"}
	if a.String() != "my-agent" {
		t.Errorf("got %q", a.String())
	}

	b := &AgentFunc{Fn: nil}
	if b.String() != "AgentFunc" {
		t.Errorf("got %q", b.String())
	}
}

func TestRoomGetFileJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"name":"test","count":42}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}

	room := &Room{dir: dir}

	var v struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	if err := room.GetFileJSON("data.json", &v); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Name != "test" || v.Count != 42 {
		t.Errorf("got %+v", v)
	}

	// Missing file.
	if err := room.GetFileJSON("missing.json", &v); err == nil {
		t.Error("expected error for missing file")
	} else if !strings.Contains(err.Error(), "reading") {
		t.Errorf("expected reading error, got: %v", err)
	}

	// Bad JSON.
	if err := room.GetFileJSON("bad.json", &v); err == nil {
		t.Error("expected error for bad JSON")
	} else if !strings.Contains(err.Error(), "decoding") {
		t.Errorf("expected decoding error, got: %v", err)
	}
}

func TestLogResult(t *testing.T) {
	var buf bytes.Buffer

	LogResult(&buf, Result{ID: "test/001", Name: "addition", Score: 1})
	LogResult(&buf, Result{ID: "test/002", Name: "division", Score: 0, Error: "wrong answer"})

	out := buf.String()
	if !strings.Contains(out, "PASS  test/001") {
		t.Errorf("missing PASS line:\n%s", out)
	}
	if !strings.Contains(out, "FAIL  test/002") {
		t.Errorf("missing FAIL line:\n%s", out)
	}
	if !strings.Contains(out, "wrong answer") {
		t.Errorf("missing error detail:\n%s", out)
	}
}
