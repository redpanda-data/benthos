package spectest

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// ResultKind classifies the outcome of a test case.
type ResultKind int

const (
	// KindPass indicates the test passed.
	KindPass ResultKind = iota
	// KindFail indicates the test produced incorrect results.
	KindFail
	// KindLoadError indicates the test file could not be loaded.
	KindLoadError
	// KindInvalidTest indicates the test specification is malformed.
	KindInvalidTest
)

// Result represents the outcome of a single test case execution.
type Result struct {
	File string     // path to the YAML test file
	Test string     // test case name
	Case string     // case name within a multi-case test (empty for single-case tests)
	Kind ResultKind // classification of the outcome
	Err  error      // nil if the test passed
}

// Passed returns true if this test passed.
func (r Result) Passed() bool { return r.Err == nil }

// String returns a human-readable summary of this result.
func (r Result) String() string {
	name := r.Test
	if r.Case != "" {
		name += "/" + r.Case
	}
	if r.Err == nil {
		return fmt.Sprintf("PASS %s / %s", r.File, name)
	}
	return fmt.Sprintf("FAIL %s / %s: %v", r.File, name, r.Err)
}

// Run discovers and executes all spec tests in dir using the given
// interpreter. Returns a result for every test case. The error return
// is reserved for infrastructure failures (directory not found, etc.) —
// individual test failures are reported in the results slice.
func Run(dir string, interp Interpreter) ([]Result, error) {
	files, err := DiscoverFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("discovering test files: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no test files found in %s", dir)
	}

	var results []Result
	for _, path := range files {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			rel = path
		}
		tf, err := LoadFile(path)
		if err != nil {
			results = append(results, Result{
				File: rel,
				Test: "(load)",
				Kind: KindLoadError,
				Err:  fmt.Errorf("loading test file: %w", err),
			})
			continue
		}
		results = append(results, RunFile(tf, rel, interp)...)
	}
	return results, nil
}

// RunFile executes all tests from a single parsed TestFile and returns
// a result for each test case.
func RunFile(file *TestFile, filePath string, interp Interpreter) []Result {
	results := make([]Result, 0, len(file.Tests))
	for i := range file.Tests {
		tc := &file.Tests[i]
		if len(tc.Cases) > 0 {
			results = append(results, runMultiCaseTest(file, tc, filePath, interp)...)
		} else {
			kind, err := runTestCase(file, tc, interp)
			results = append(results, Result{
				File: filePath,
				Test: tc.Name,
				Kind: kind,
				Err:  err,
			})
		}
	}
	return results
}

// RunT is a convenience that runs all spec tests and reports failures
// through testing.T with proper subtest hierarchy.
func RunT(t *testing.T, dir string, interp Interpreter) {
	t.Helper()

	files, err := DiscoverFiles(dir)
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
			tf, err := LoadFile(path)
			if err != nil {
				t.Fatalf("loading test file: %v", err)
			}
			for i := range tf.Tests {
				tc := &tf.Tests[i]
				if len(tc.Cases) > 0 {
					t.Run(tc.Name, func(t *testing.T) {
						for _, r := range runMultiCaseTest(tf, tc, rel, interp) {
							t.Run(r.Case, func(t *testing.T) {
								if r.Err != nil {
									t.Fatal(r.Err)
								}
							})
						}
					})
				} else {
					kind, err := runTestCase(tf, tc, interp)
					r := Result{File: rel, Test: tc.Name, Kind: kind, Err: err}
					t.Run(r.Test, func(t *testing.T) {
						if r.Err != nil {
							t.Fatal(r.Err)
						}
					})
				}
			}
		})
	}
}

// runTestCase executes a single test case and returns its result kind and
// an error if it failed.
func runTestCase(file *TestFile, tc *TestCase, interp Interpreter) (ResultKind, error) {
	// 0. Validate that exactly one expectation is set.
	if err := validateExpectations(tc); err != nil {
		return KindInvalidTest, err
	}

	// 1. Merge files: file-level + test-level (test wins).
	mergedFiles := mergeFiles(file.Files, tc.Files)

	// 2. Decode inputs.
	input, err := DecodeValue(tc.Input)
	if err != nil {
		return KindInvalidTest, fmt.Errorf("invalid test: decoding input: %w", err)
	}

	inputMeta, err := decodeMetadata(tc.InputMetadata)
	if err != nil {
		return KindInvalidTest, fmt.Errorf("invalid test: decoding input_metadata: %w", err)
	}

	// 3. Compile.
	mapping, compileErr := interp.Compile(tc.Mapping, mergedFiles)
	if tc.CompileError != "" {
		return KindFail, checkCompileError(compileErr, tc.CompileError)
	}
	if compileErr != nil {
		return KindFail, fmt.Errorf("unexpected compile error: %w", compileErr)
	}

	// 4. Execute.
	output, outputMeta, deleted, execErr := mapping.Exec(input, inputMeta)
	if tc.Error != "" || tc.HasError {
		return KindFail, checkRuntimeError(execErr, tc.Error)
	}
	if tc.Deleted {
		if execErr != nil {
			return KindFail, fmt.Errorf("unexpected error (expected deleted): %w", execErr)
		}
		if !deleted {
			return KindFail, errors.New("expected message to be deleted, but it was not")
		}
		return KindPass, nil
	}
	if execErr != nil {
		return KindFail, fmt.Errorf("unexpected runtime error: %w", execErr)
	}
	if deleted {
		return KindFail, errors.New("message was unexpectedly deleted")
	}

	// 5. Compare output.
	if err := checkOutput(tc, output); err != nil {
		return KindFail, err
	}

	// 6. Compare output metadata.
	if err := checkMetadata(tc, outputMeta); err != nil {
		return KindFail, err
	}
	return KindPass, nil
}

// validateExpectations checks that a test case specifies exactly one
// expectation: output (or no_output_check), compile_error, error, or deleted.
// Also validates that output_type is only used with no_output_check.
func validateExpectations(tc *TestCase) error {
	count := 0
	if tc.CompileError != "" {
		count++
	}
	if tc.Error != "" || tc.HasError {
		count++
	}
	if tc.Deleted {
		count++
	}
	if tc.HasOutput || tc.Output != nil || tc.NoOutputCheck {
		count++
	}

	if count == 0 {
		return errors.New("invalid test: no expectation set (need output, compile_error, error, or deleted)")
	}
	if count > 1 {
		return fmt.Errorf("invalid test: multiple expectations set (compile_error=%q, error=%q, deleted=%v, has_output=%v)",
			tc.CompileError, tc.Error, tc.Deleted, tc.Output != nil || tc.NoOutputCheck)
	}

	if tc.OutputType != "" && !tc.NoOutputCheck {
		return errors.New("invalid test: output_type requires no_output_check to be true")
	}

	return nil
}

func mergeFiles(fileLevel, testLevel map[string]string) map[string]string {
	if len(fileLevel) == 0 && len(testLevel) == 0 {
		return nil
	}
	merged := make(map[string]string, len(fileLevel)+len(testLevel))
	for k, v := range fileLevel {
		merged[k] = v
	}
	for k, v := range testLevel {
		merged[k] = v
	}
	return merged
}

func decodeMetadata(raw any) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}
	decoded, err := DecodeValue(raw)
	if err != nil {
		return nil, err
	}
	meta, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("input_metadata must be an object, got %T", decoded)
	}
	return meta, nil
}

func checkCompileError(err error, expectedSubstring string) error {
	if err == nil {
		return fmt.Errorf("expected compile error containing %q, but compilation succeeded", expectedSubstring)
	}
	var ce *CompileError
	if !errors.As(err, &ce) {
		return fmt.Errorf("expected a *CompileError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), expectedSubstring) {
		return fmt.Errorf("compile error %q does not contain expected substring %q", err.Error(), expectedSubstring)
	}
	return nil
}

func checkRuntimeError(err error, expectedSubstring string) error {
	if err == nil {
		return fmt.Errorf("expected runtime error containing %q, but execution succeeded", expectedSubstring)
	}
	var ce *CompileError
	if errors.As(err, &ce) {
		return fmt.Errorf("expected a runtime error, got *CompileError: %v", err)
	}
	if !strings.Contains(err.Error(), expectedSubstring) {
		return fmt.Errorf("runtime error %q does not contain expected substring %q", err.Error(), expectedSubstring)
	}
	return nil
}

func checkOutputFields(output any, outputType string, noOutputCheck bool, actual any) error {
	if noOutputCheck {
		if outputType != "" {
			ok, diff := CheckOutputType(outputType, actual)
			if !ok {
				return fmt.Errorf("output type mismatch: %s", diff)
			}
		}
		return nil
	}

	expected, err := DecodeValue(output)
	if err != nil {
		return fmt.Errorf("invalid test: decoding expected output: %w", err)
	}

	ok, diff := DeepEqual(expected, actual)
	if !ok {
		return fmt.Errorf("output mismatch:\n%s", diff)
	}
	return nil
}

func checkOutput(tc *TestCase, actual any) error {
	return checkOutputFields(tc.Output, tc.OutputType, tc.NoOutputCheck, actual)
}

func checkMetadataFields(outputMetadata any, noMetadataCheck bool, actual map[string]any) error {
	if noMetadataCheck {
		return nil
	}

	var expected map[string]any
	if outputMetadata != nil {
		decoded, err := DecodeValue(outputMetadata)
		if err != nil {
			return fmt.Errorf("invalid test: decoding expected output_metadata: %w", err)
		}
		var ok bool
		expected, ok = decoded.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid test: output_metadata must be an object, got %T", decoded)
		}
	} else {
		expected = map[string]any{}
	}

	if actual == nil {
		actual = map[string]any{}
	}

	ok, diff := DeepEqual(any(expected), any(actual))
	if !ok {
		return fmt.Errorf("output metadata mismatch:\n%s", diff)
	}
	return nil
}

func checkMetadata(tc *TestCase, actual map[string]any) error {
	return checkMetadataFields(tc.OutputMetadata, tc.NoMetadataCheck, actual)
}

// runMultiCaseTest executes a test that has multiple cases sharing one
// compiled mapping.
func runMultiCaseTest(file *TestFile, tc *TestCase, filePath string, interp Interpreter) []Result {
	if err := validateMultiCase(tc); err != nil {
		return []Result{{
			File: filePath, Test: tc.Name,
			Kind: KindInvalidTest, Err: err,
		}}
	}

	mergedFiles := mergeFiles(file.Files, tc.Files)

	mapping, compileErr := interp.Compile(tc.Mapping, mergedFiles)
	if compileErr != nil {
		return []Result{{
			File: filePath, Test: tc.Name,
			Kind: KindFail,
			Err:  fmt.Errorf("unexpected compile error: %w", compileErr),
		}}
	}

	results := make([]Result, 0, len(tc.Cases))
	for i := range tc.Cases {
		c := &tc.Cases[i]
		kind, err := runCase(mapping, c)
		results = append(results, Result{
			File: filePath,
			Test: tc.Name,
			Case: c.Name,
			Kind: kind,
			Err:  err,
		})
	}
	return results
}

// runCase executes a single case against an already-compiled mapping.
func runCase(mapping Mapping, c *Case) (ResultKind, error) {
	if err := validateCaseExpectations(c); err != nil {
		return KindInvalidTest, err
	}

	input, err := DecodeValue(c.Input)
	if err != nil {
		return KindInvalidTest, fmt.Errorf("invalid case: decoding input: %w", err)
	}

	inputMeta, err := decodeMetadata(c.InputMetadata)
	if err != nil {
		return KindInvalidTest, fmt.Errorf("invalid case: decoding input_metadata: %w", err)
	}

	output, outputMeta, deleted, execErr := mapping.Exec(input, inputMeta)

	if c.Error != "" || c.HasError {
		return KindFail, checkRuntimeError(execErr, c.Error)
	}
	if c.Deleted {
		if execErr != nil {
			return KindFail, fmt.Errorf("unexpected error (expected deleted): %w", execErr)
		}
		if !deleted {
			return KindFail, errors.New("expected message to be deleted, but it was not")
		}
		return KindPass, nil
	}
	if execErr != nil {
		return KindFail, fmt.Errorf("unexpected runtime error: %w", execErr)
	}
	if deleted {
		return KindFail, errors.New("message was unexpectedly deleted")
	}

	if err := checkOutputFields(c.Output, c.OutputType, c.NoOutputCheck, output); err != nil {
		return KindFail, err
	}
	if err := checkMetadataFields(c.OutputMetadata, c.NoMetadataCheck, outputMeta); err != nil {
		return KindFail, err
	}
	return KindPass, nil
}

// validateMultiCase checks that a multi-case test is well-formed.
func validateMultiCase(tc *TestCase) error {
	if len(tc.Cases) == 0 {
		return errors.New("invalid test: cases array is empty")
	}

	// Cases must not coexist with inline execution fields.
	if tc.HasOutput || tc.Output != nil || tc.NoOutputCheck ||
		tc.Error != "" || tc.HasError || tc.Deleted ||
		tc.Input != nil || tc.InputMetadata != nil || tc.OutputMetadata != nil {
		return errors.New("invalid test: cannot mix inline input/output fields with cases")
	}

	if tc.CompileError != "" {
		return errors.New("invalid test: compile_error cannot be combined with cases")
	}

	for i := range tc.Cases {
		if tc.Cases[i].Name == "" {
			return fmt.Errorf("invalid test: case at index %d has no name", i)
		}
	}
	return nil
}

// validateCaseExpectations checks that a case specifies exactly one expectation.
func validateCaseExpectations(c *Case) error {
	count := 0
	if c.Error != "" || c.HasError {
		count++
	}
	if c.Deleted {
		count++
	}
	if c.HasOutput || c.Output != nil || c.NoOutputCheck {
		count++
	}

	if count == 0 {
		return errors.New("invalid case: no expectation set (need output, error, or deleted)")
	}
	if count > 1 {
		return fmt.Errorf("invalid case: multiple expectations set (error=%q, deleted=%v, has_output=%v)",
			c.Error, c.Deleted, c.Output != nil || c.NoOutputCheck)
	}

	if c.OutputType != "" && !c.NoOutputCheck {
		return errors.New("invalid case: output_type requires no_output_check to be true")
	}
	return nil
}
