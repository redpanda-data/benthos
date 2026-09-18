// Package speccompat verifies that the tree-sitter-bloblang2 grammar can parse
// every mapping from the Bloblang V2 spec test suite without producing ERROR
// nodes. This catches grammar regressions against the reference spec.
//
// Tests with compile_error are included — some are parse errors (which
// tree-sitter should also flag as ERROR), others are semantic errors (which
// should parse cleanly). We skip compile_error tests from the error check
// since the grammar can't distinguish parse vs semantic errors.
//
// Run with:
//
//	go test -tags treesitter ./internal/bloblang2/tree-sitter/test/speccompat/
//
//go:build treesitter

package speccompat

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type specFile struct {
	Description string            `yaml:"description"`
	Files       map[string]string `yaml:"files"`
	Tests       []specTest        `yaml:"tests"`
}

type specTest struct {
	Name         string `yaml:"name"`
	Mapping      string `yaml:"mapping"`
	CompileError string `yaml:"compile_error"`
}

func TestSpecMappingsParse(t *testing.T) {
	// Find tree-sitter CLI.
	tsPath, err := exec.LookPath("tree-sitter")
	if err != nil {
		// Try npx.
		tsPath = "npx"
	}

	// Find the grammar root (two levels up from test/speccompat/).
	grammarDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	// Ensure the parser is generated.
	parserC := filepath.Join(grammarDir, "src", "parser.c")
	if _, err := os.Stat(parserC); os.IsNotExist(err) {
		t.Skipf("parser not generated — run 'npx tree-sitter generate' in %s first", grammarDir)
	}

	// Find spec test directory.
	specDir := filepath.Join(grammarDir, "..", "spec", "tests")
	if _, err := os.Stat(specDir); os.IsNotExist(err) {
		t.Skipf("spec tests not found at %s", specDir)
	}

	// Walk all YAML files.
	var files []string
	err = filepath.Walk(specDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".yaml") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(files) == 0 {
		t.Fatal("no spec test files found")
	}

	var total, passed, skipped, failed int

	for _, file := range files {
		relPath, _ := filepath.Rel(specDir, file)

		data, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("failed to read %s: %v", relPath, err)
			continue
		}

		var sf specFile
		if err := yaml.Unmarshal(data, &sf); err != nil {
			t.Errorf("failed to parse %s: %v", relPath, err)
			continue
		}

		for _, tc := range sf.Tests {
			total++
			testName := fmt.Sprintf("%s/%s", relPath, tc.Name)

			if tc.Mapping == "" {
				skipped++
				continue
			}

			// Skip compile_error tests — they may have intentional parse errors
			// that tree-sitter would flag as ERROR. The grammar can't distinguish
			// parse errors from semantic errors.
			if tc.CompileError != "" {
				skipped++
				continue
			}

			// Also parse any imported files to verify they parse cleanly.
			mappings := map[string]string{"main": tc.Mapping}
			for name, content := range sf.Files {
				mappings[name] = content
			}

			for label, mapping := range mappings {
				// Write mapping to temp file.
				tmpFile, err := os.CreateTemp("", "blobl2-*.blobl2")
				if err != nil {
					t.Fatal(err)
				}
				_, _ = tmpFile.WriteString(mapping)
				tmpFile.Close()

				// Run tree-sitter parse.
				var cmd *exec.Cmd
				if tsPath == "npx" {
					cmd = exec.Command("npx", "tree-sitter", "parse", tmpFile.Name())
				} else {
					cmd = exec.Command(tsPath, "parse", tmpFile.Name())
				}
				cmd.Dir = grammarDir
				output, err := cmd.CombinedOutput()
				os.Remove(tmpFile.Name())

				outStr := string(output)

				if strings.Contains(outStr, "(ERROR") || strings.Contains(outStr, "(MISSING") {
					failed++
					suffix := ""
					if label != "main" {
						suffix = fmt.Sprintf(" (file: %s)", label)
					}
					// Show just the first few lines of the parse tree for context.
					lines := strings.Split(outStr, "\n")
					preview := outStr
					if len(lines) > 20 {
						preview = strings.Join(lines[:20], "\n") + "\n..."
					}
					t.Errorf("ERROR in parse tree for %s%s:\n  mapping:\n    %s\n  parse tree:\n%s",
						testName, suffix,
						strings.ReplaceAll(strings.TrimSpace(mapping), "\n", "\n    "),
						preview)
					break
				} else if err != nil && !strings.Contains(outStr, "(source_file") {
					// tree-sitter parse failed entirely (not just ERROR nodes).
					failed++
					t.Errorf("tree-sitter parse failed for %s: %v\n%s", testName, err, outStr)
					break
				} else {
					// Only count as passed if all mappings (main + files) parsed.
					if label == "main" && len(mappings) == 1 {
						passed++
					} else if label != "main" {
						// Will be counted when main passes.
					} else {
						passed++
					}
				}
			}
		}
	}

	t.Logf("Spec compatibility: %d total, %d passed, %d skipped (compile_error), %d failed",
		total, passed, skipped, failed)
}
