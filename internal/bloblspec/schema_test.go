package bloblspec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	content := `description: "test file"
files:
  "helper.blobl": |
    map double(x) { x * 2 }
tests:
  - name: "basic test"
    mapping: |
      output.v = 42
    output: {"v": 42}
  - name: "error test"
    mapping: |
      output = bad
    compile_error: "bad"
  - name: "with input"
    input: {"x": 1}
    input_metadata: {"key": "val"}
    mapping: |
      output = input.x
    output: 1
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tf, err := LoadFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tf.Description != "test file" {
		t.Fatalf("expected description %q, got %q", "test file", tf.Description)
	}
	if len(tf.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(tf.Files))
	}
	if _, ok := tf.Files["helper.blobl"]; !ok {
		t.Fatal("expected helper.blobl in files")
	}
	if len(tf.Tests) != 3 {
		t.Fatalf("expected 3 tests, got %d", len(tf.Tests))
	}
	if tf.Tests[0].Name != "basic test" {
		t.Fatalf("expected first test name %q, got %q", "basic test", tf.Tests[0].Name)
	}
	if tf.Tests[1].CompileError != "bad" {
		t.Fatalf("expected compile_error %q, got %q", "bad", tf.Tests[1].CompileError)
	}
}

func TestLoadFile_NotFound(t *testing.T) {
	_, err := LoadFile("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{invalid yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestDiscoverFiles(t *testing.T) {
	dir := t.TempDir()

	// Create nested structure.
	subdir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		filepath.Join(dir, "b.yaml"),
		filepath.Join(dir, "a.yaml"),
		filepath.Join(subdir, "c.yaml"),
		filepath.Join(dir, "skip.txt"),
	} {
		if err := os.WriteFile(name, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := DiscoverFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 yaml files, got %d: %v", len(files), files)
	}

	// Should be sorted.
	names := make([]string, len(files))
	for i, f := range files {
		rel, _ := filepath.Rel(dir, f)
		names[i] = rel
	}
	if names[0] != "a.yaml" || names[1] != "b.yaml" || names[2] != filepath.Join("sub", "c.yaml") {
		t.Fatalf("unexpected order: %v", names)
	}
}

func TestDiscoverFiles_NonexistentDir(t *testing.T) {
	_, err := DiscoverFiles("/nonexistent/dir")
	if err == nil {
		t.Fatal("expected error for nonexistent dir")
	}
}

func TestDiscoverFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	files, err := DiscoverFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}
