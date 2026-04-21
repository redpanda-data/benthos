package migrator

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/spectest"
	"gopkg.in/yaml.v3"
)

// skipProbe is used to read just the `skip` field of each test case so we can
// filter them out before handing the file to the shared spectest runner, which
// does not understand `skip`.
type skipProbe struct {
	Tests []struct {
		Name string `yaml:"name"`
		Skip string `yaml:"skip"`
	} `yaml:"tests"`
}

// skippedNames returns a set of test-case names (and reasons) marked with
// `skip:` in the raw YAML. The spectest schema discards unknown fields, so we
// read the file twice — once for this probe, once for the real loader.
func skippedNames(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var probe skipProbe
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("parsing skip probe for %s: %w", path, err)
	}
	out := map[string]string{}
	for _, t := range probe.Tests {
		if t.Skip != "" {
			out[t.Name] = t.Skip
		}
	}
	return out, nil
}

// RunT walks every YAML file under dir, filters tests marked `skip:` (recording
// them via t.Skip for visibility), and runs the remainder against the given
// interpreter through spectest.
//
// Layout matches spectest.RunT: a subtest per file, a sub-subtest per case.
func RunT(t *testing.T, dir string, interp spectest.Interpreter) {
	t.Helper()

	files, err := spectest.DiscoverFiles(dir)
	if err != nil {
		t.Fatalf("discovering test files: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no test files found in %s", dir)
	}

	for _, path := range files {
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = path
		}
		t.Run(rel, func(t *testing.T) {
			skips, err := skippedNames(path)
			if err != nil {
				t.Fatalf("loading skip probe: %v", err)
			}
			tf, err := spectest.LoadFile(path)
			if err != nil {
				t.Fatalf("loading test file: %v", err)
			}
			for i := range tf.Tests {
				tc := &tf.Tests[i]
				if reason, ok := skips[tc.Name]; ok {
					t.Run(tc.Name, func(t *testing.T) {
						t.Skipf("skipped by test file: %s", reason)
					})
					continue
				}
				runSingle(t, tf, tc, rel, interp)
			}
		})
	}
}

// runSingle replicates spectest.RunT's per-test dispatch for one test case,
// inline so we can interleave skip handling. Multi-case tests are handed off to
// spectest's internal runner via a slim TestFile copy.
func runSingle(t *testing.T, tf *spectest.TestFile, tc *spectest.TestCase, rel string, interp spectest.Interpreter) {
	t.Helper()

	// Isolate this test case in a single-test TestFile so spectest.RunFile
	// handles the compile/exec/compare dance — including multi-case
	// (`cases:`) tests.
	single := &spectest.TestFile{
		Description: tf.Description,
		Files:       tf.Files,
		Tests:       []spectest.TestCase{*tc},
	}
	results := spectest.RunFile(single, rel, interp)

	if len(tc.Cases) > 0 {
		t.Run(tc.Name, func(t *testing.T) {
			for _, r := range results {
				caseName := r.Case
				if caseName == "" {
					caseName = "(case)"
				}
				t.Run(caseName, func(t *testing.T) {
					if r.Err != nil {
						t.Fatal(r.Err)
					}
				})
			}
		})
		return
	}

	t.Run(tc.Name, func(t *testing.T) {
		for _, r := range results {
			if r.Err != nil {
				t.Fatal(r.Err)
			}
		}
	})
}
