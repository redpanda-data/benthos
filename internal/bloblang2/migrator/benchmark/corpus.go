package benchmark

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/spectest"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// Case is one benchmarkable V1↔V2 mapping pair: the V1 source, the
// translated V2 source, the input, and pre-compiled runners for both
// sides. Cases reach this form only when translation succeeded, both
// sides compile, both sides run without error, and their outputs are
// spectest.DeepEqual.
type Case struct {
	// Name uniquely identifies this case within the corpus: "<rel
	// path>/<test name>[/<sub-case>]". Used as the sub-benchmark name.
	Name string
	// V1Source is the mapping text as it appears in the V1 corpus.
	V1Source string
	// V2Source is the translator.Migrate output.
	V2Source string
	// Input is the already-decoded test input (spectest.DecodeValue form).
	Input any
	// InputMetadata is the already-decoded input metadata (may be empty).
	InputMetadata map[string]any
	// Expected is the output both V1 and V2 produce. Stored for sanity
	// checks during benchmark iterations; the harness doesn't re-check
	// each loop because that would distort the timing.
	Expected any
	// V1 and V2 are the compiled runners, ready to call Exec() with the
	// test input.
	V1 *v1Runner
	V2 *v2Runner
}

// SkipReason explains why a case was excluded from the benchmark set.
type SkipReason string

// SkipReason constants. Each one corresponds to a distinct gating
// failure in Collection.admit. The set is stable enough to group and
// count when emitting the analysis report.
const (
	SkipV1ParseFail        SkipReason = "v1-parse-fail"
	SkipV1ExecFail         SkipReason = "v1-exec-fail"
	SkipTranslateFail      SkipReason = "translate-fail"
	SkipV2CompileFail      SkipReason = "v2-compile-fail"
	SkipV2ExecFail         SkipReason = "v2-exec-fail"
	SkipOutputMismatch     SkipReason = "output-mismatch"
	SkipExpectsError       SkipReason = "expects-error"
	SkipNoInput            SkipReason = "no-input"
	SkipInputDecode        SkipReason = "input-decode-fail"
	SkipExpectedDelete     SkipReason = "expects-delete"
	SkipMultiCaseUnwound   SkipReason = "multi-case"
	SkipExplicitlyMarked   SkipReason = "skip-marker"
	SkipEmptyMapping       SkipReason = "empty-mapping"
	SkipOutputDecodeFail   SkipReason = "output-decode-fail"
	SkipInvalidTestCase    SkipReason = "invalid-test-case"
	SkipInputMetaDecode    SkipReason = "input-metadata-decode-fail"
	SkipNoOutputCheckBench SkipReason = "no-output-check"
)

// SkipRecord records why a test case was rejected. Collected alongside
// the accepted Cases so the analysis report can explain the coverage.
type SkipRecord struct {
	Name   string
	Reason SkipReason
	Detail string
}

// Collection is the result of scanning the corpus: the accepted cases
// plus the rejected ones bucketed by reason.
type Collection struct {
	Cases []Case
	Skips []SkipRecord
}

// CollectDefault scans ../v1spec/tests relative to this source file.
func CollectDefault() (*Collection, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "v1spec", "tests")
	return Collect(root)
}

// Collect walks root, loads every yaml file, and returns the
// benchmarkable subset of test cases along with skip records for the
// rest. The scan is deterministic (files sorted, cases in file order).
func Collect(root string) (*Collection, error) {
	files, err := discoverFiles(root)
	if err != nil {
		return nil, err
	}
	out := &Collection{}
	for _, path := range files {
		tf, err := spectest.LoadFile(path)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(root, path)
		for i := range tf.Tests {
			tc := &tf.Tests[i]
			if len(tc.Cases) > 0 {
				for j := range tc.Cases {
					name := fmt.Sprintf("%s/%s/%s", rel, tc.Name, tc.Cases[j].Name)
					out.admit(name, tc, &tc.Cases[j], tf.Files, path)
				}
				continue
			}
			name := fmt.Sprintf("%s/%s", rel, tc.Name)
			out.admit(name, tc, nil, tf.Files, path)
		}
	}
	return out, nil
}

// admit attempts to add a test case to the collection. If any gate
// fails (rejected by V1, rejected by V2, outputs differ, …) the case
// lands on Skips with an explanatory reason instead. The gating runs
// translation + a sanity Exec on each side so the benchmark itself
// only contains known-equivalent pairs.
func (c *Collection) admit(name string, tc *spectest.TestCase, sub *spectest.Case, fileFiles map[string]string, yamlPath string) {
	// Pull out the fields that can live on either the parent TestCase
	// or a child Case. The Case form is read if present; otherwise the
	// parent's fields govern.
	mapping := tc.Mapping
	input := tc.Input
	inputMeta := tc.InputMetadata
	hasError := tc.HasError
	errStr := tc.Error
	compileErr := tc.CompileError
	deleted := tc.Deleted
	noOutput := tc.NoOutputCheck
	if sub != nil {
		input = sub.Input
		inputMeta = sub.InputMetadata
		hasError = sub.HasError
		errStr = sub.Error
		deleted = sub.Deleted
		noOutput = sub.NoOutputCheck
	}

	// Skip-marker check runs first because many "skip: …" cases have no
	// mapping by design — the author stripped the body when the case was
	// known-incompatible. Attributing those to SkipEmptyMapping would
	// understate the real reason for skipping.
	if isSkipped(yamlPath, tc.Name) {
		c.skip(name, SkipExplicitlyMarked, "v1spec skip marker")
		return
	}
	// Filter out cases that don't represent a successful mapping run.
	if strings.TrimSpace(mapping) == "" {
		c.skip(name, SkipEmptyMapping, "")
		return
	}
	if compileErr != "" {
		c.skip(name, SkipExpectsError, "compile error expected")
		return
	}
	if hasError || errStr != "" {
		c.skip(name, SkipExpectsError, "runtime error expected")
		return
	}
	if deleted {
		c.skip(name, SkipExpectedDelete, "root deletion expected")
		return
	}
	if noOutput {
		c.skip(name, SkipNoOutputCheckBench, "")
		return
	}

	decodedInput, err := spectest.DecodeValue(input)
	if err != nil {
		c.skip(name, SkipInputDecode, err.Error())
		return
	}
	decodedMeta := map[string]any{}
	if inputMeta != nil {
		raw, err := spectest.DecodeValue(inputMeta)
		if err != nil {
			c.skip(name, SkipInputMetaDecode, err.Error())
			return
		}
		if m, ok := raw.(map[string]any); ok {
			decodedMeta = m
		}
	}

	// 1. Compile V1.
	files := mergeFileMaps(fileFiles, tc.Files)
	v1, err := newV1Runner(mapping, files)
	if err != nil {
		c.skip(name, SkipV1ParseFail, err.Error())
		return
	}

	// 2. Run V1 once to capture expected output. If V1 errors, this
	// case isn't benchmarkable — V2 equivalence is meaningless.
	v1Out, err := v1.Exec(decodedInput, decodedMeta)
	if err != nil {
		c.skip(name, SkipV1ExecFail, err.Error())
		return
	}

	// 3. Translate V1 -> V2.
	rep, err := translator.Migrate(mapping, translator.Options{MinCoverage: 0, Files: files})
	if err != nil {
		c.skip(name, SkipTranslateFail, err.Error())
		return
	}

	// 4. Compile V2.
	v2, err := newV2Runner(rep.V2Mapping)
	if err != nil {
		c.skip(name, SkipV2CompileFail, err.Error())
		return
	}

	// 5. Run V2 once and compare.
	v2Out, err := v2.Exec(decodedInput, decodedMeta)
	if err != nil {
		c.skip(name, SkipV2ExecFail, err.Error())
		return
	}
	if ok, diff := spectest.DeepEqual(v1Out, v2Out); !ok {
		c.skip(name, SkipOutputMismatch, diff)
		return
	}

	c.Cases = append(c.Cases, Case{
		Name:          name,
		V1Source:      mapping,
		V2Source:      rep.V2Mapping,
		Input:         decodedInput,
		InputMetadata: decodedMeta,
		Expected:      v1Out,
		V1:            v1,
		V2:            v2,
	})
}

func (c *Collection) skip(name string, reason SkipReason, detail string) {
	c.Skips = append(c.Skips, SkipRecord{Name: name, Reason: reason, Detail: detail})
}

// SkipCounts returns a count of skip records grouped by reason.
func (c *Collection) SkipCounts() map[SkipReason]int {
	counts := map[SkipReason]int{}
	for _, s := range c.Skips {
		counts[s.Reason]++
	}
	return counts
}

// -----------------------------------------------------------------------
// Corpus-file discovery helpers (shared with corpus_test.go patterns;
// duplicated here so the benchmark package has no test-package
// dependency).
// -----------------------------------------------------------------------

func discoverFiles(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".yaml") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

var skipCache = map[string]map[string]bool{}

// isSkipped consults the corpus YAML for a `skip: …` line under the
// named test case. Mirrors the v1spec `skip:` extension to the shared
// spectest schema (which has no field for it in-struct).
func isSkipped(path, name string) bool {
	skips, ok := skipCache[path]
	if !ok {
		skips = loadSkips(path)
		skipCache[path] = skips
	}
	return skips[name]
}

func loadSkips(path string) map[string]bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	lines := strings.Split(string(data), "\n")
	current := ""
	for _, l := range lines {
		trim := strings.TrimSpace(l)
		if strings.HasPrefix(trim, "- name:") {
			current = strings.TrimSpace(strings.TrimPrefix(trim, "- name:"))
			current = strings.Trim(current, `"`)
			continue
		}
		if current != "" && strings.HasPrefix(trim, "skip:") {
			out[current] = true
			current = ""
		}
	}
	return out
}

// mergeFileMaps merges file-scoped and case-scoped import maps, with
// case-level entries winning on collision.
func mergeFileMaps(fileLevel, caseLevel map[string]string) map[string]string {
	if len(fileLevel) == 0 && len(caseLevel) == 0 {
		return nil
	}
	out := make(map[string]string, len(fileLevel)+len(caseLevel))
	for k, v := range fileLevel {
		out[k] = v
	}
	for k, v := range caseLevel {
		out[k] = v
	}
	return out
}
