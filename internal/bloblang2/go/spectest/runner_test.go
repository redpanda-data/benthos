package spectest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// mockInterpreter implements Interpreter for testing the runner itself.
type mockInterpreter struct {
	compileFunc func(mapping string, files map[string]string) (Mapping, error)
}

func (m *mockInterpreter) Compile(mapping string, files map[string]string) (Mapping, error) {
	return m.compileFunc(mapping, files)
}

// mockMapping implements Mapping for testing.
type mockMapping struct {
	execFunc func(input any, metadata map[string]any) (any, map[string]any, bool, error)
}

func (m *mockMapping) Exec(input any, metadata map[string]any) (any, map[string]any, bool, error) {
	return m.execFunc(input, metadata)
}

func requirePass(t *testing.T, results []Result) {
	t.Helper()
	for _, r := range results {
		if r.Err != nil {
			t.Fatalf("expected all tests to pass, but %q failed: %v", r.Test, r.Err)
		}
	}
}

func requireFail(t *testing.T, results []Result, testName string) {
	t.Helper()
	for _, r := range results {
		if r.Test == testName {
			if r.Err == nil {
				t.Fatalf("expected test %q to fail, but it passed", testName)
			}
			return
		}
	}
	t.Fatalf("test %q not found in results", testName)
}

func TestRunFile_Success(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "basic output",
				Mapping: "output.x = 42",
				Output:  map[string]any{"x": int(42)},
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return map[string]any{"x": int64(42)}, map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requirePass(t, results)
}

func TestRunFile_CompileError(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:         "expect compile error",
				Mapping:      "bad syntax",
				CompileError: "syntax",
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return nil, &CompileError{Message: "syntax error at line 1"}
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requirePass(t, results)
}

func TestRunFile_CompileErrorWrongKind(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:         "expect compile error but get runtime",
				Mapping:      "bad",
				CompileError: "syntax",
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			// Return a plain error, not *CompileError.
			return nil, fmt.Errorf("syntax issue")
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requireFail(t, results, "expect compile error but get runtime")
}

func TestRunFile_RuntimeError(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "expect runtime error",
				Mapping: "output = 5 / 0",
				Error:   "division by zero",
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return nil, nil, false, fmt.Errorf("division by zero")
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requirePass(t, results)
}

func TestRunFile_RuntimeErrorWrongKind(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "expect runtime error but get compile",
				Mapping: "output = 5 / 0",
				Error:   "overflow",
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					// Return a *CompileError when runtime error was expected.
					return nil, nil, false, &CompileError{Message: "overflow detected"}
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requireFail(t, results, "expect runtime error but get compile")
}

func TestRunFile_Deleted(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "expect deletion",
				Mapping: "output = deleted()",
				Deleted: true,
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return nil, nil, true, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requirePass(t, results)
}

func TestRunFile_FileMerge(t *testing.T) {
	tf := &TestFile{
		Files: map[string]string{
			"lib.blobl": "map double(x) { x * 2 }",
		},
		Tests: []TestCase{
			{
				Name:    "file-level files available",
				Mapping: `import "lib.blobl" as l`,
				Output:  int64(42),
			},
			{
				Name:    "test-level override",
				Mapping: `import "lib.blobl" as l`,
				Files: map[string]string{
					"lib.blobl": "map triple(x) { x * 3 }",
				},
				Output: int64(42),
			},
		},
	}

	var capturedFiles []map[string]string
	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			cpy := make(map[string]string, len(files))
			for k, v := range files {
				cpy[k] = v
			}
			capturedFiles = append(capturedFiles, cpy)
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return int64(42), map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requirePass(t, results)

	if len(capturedFiles) != 2 {
		t.Fatalf("expected 2 compile calls, got %d", len(capturedFiles))
	}
	if capturedFiles[0]["lib.blobl"] != "map double(x) { x * 2 }" {
		t.Fatalf("first test should see file-level lib.blobl, got: %q", capturedFiles[0]["lib.blobl"])
	}
	if capturedFiles[1]["lib.blobl"] != "map triple(x) { x * 3 }" {
		t.Fatalf("second test should see test-level override, got: %q", capturedFiles[1]["lib.blobl"])
	}
}

func TestRunFile_OutputMetadataDefault(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "metadata defaults to empty",
				Mapping: "output.x = 1",
				Output:  map[string]any{"x": int(1)},
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return map[string]any{"x": int64(1)}, map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requirePass(t, results)
}

func TestRunFile_MetadataLeakDetected(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "leaked metadata caught",
				Mapping: "output.x = 1",
				Output:  map[string]any{"x": int(1)},
				// No OutputMetadata — defaults to {}, so leaked metadata is caught.
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return map[string]any{"x": int64(1)}, map[string]any{"leaked": "value"}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requireFail(t, results, "leaked metadata caught")
}

func TestRunFile_NoMetadataCheck(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:            "skip metadata check",
				Mapping:         "output.x = 1",
				Output:          map[string]any{"x": int(1)},
				NoMetadataCheck: true,
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return map[string]any{"x": int64(1)}, map[string]any{"leaked": "value"}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requirePass(t, results)
}

func TestRunFile_NoOutputCheck(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:            "skip output check with type",
				Mapping:         "output = uuid_v4()",
				NoOutputCheck:   true,
				NoMetadataCheck: true,
				OutputType:      "string",
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return "some-uuid-value", map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requirePass(t, results)
}

func TestRunFile_InputDecoding(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:            "typed input decoded",
				Mapping:         "output = input",
				Input:           map[string]any{"val": map[string]any{"_type": "float32", "value": "1.5"}},
				Output:          map[string]any{"val": map[string]any{"_type": "float32", "value": "1.5"}},
				NoMetadataCheck: true,
			},
		},
	}

	var capturedInput any
	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					capturedInput = input
					return map[string]any{"val": float32(1.5)}, map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requirePass(t, results)

	inputMap, ok := capturedInput.(map[string]any)
	if !ok {
		t.Fatalf("expected input to be map, got %T", capturedInput)
	}
	val, ok := inputMap["val"].(float32)
	if !ok {
		t.Fatalf("expected input.val to be float32, got %T", inputMap["val"])
	}
	if val != 1.5 {
		t.Fatalf("expected input.val = 1.5, got %v", val)
	}
}

func TestRunFile_OutputMismatchFails(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:            "wrong output",
				Mapping:         "output.x = 1",
				Output:          map[string]any{"x": int(99)},
				NoMetadataCheck: true,
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return map[string]any{"x": int64(1)}, map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requireFail(t, results, "wrong output")
}

func TestRunFile_CompileErrorButSucceeds(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:         "expect compile error but succeeds",
				Mapping:      "output = 1",
				CompileError: "syntax",
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return int64(1), map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requireFail(t, results, "expect compile error but succeeds")
}

func TestRunFile_RuntimeErrorButSucceeds(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "expect error but succeeds",
				Mapping: "output = 1",
				Error:   "overflow",
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return int64(1), map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requireFail(t, results, "expect error but succeeds")
}

func TestRunFile_CompileErrorWrongSubstring(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:         "wrong substring",
				Mapping:      "bad",
				CompileError: "overflow",
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return nil, &CompileError{Message: "syntax error"}
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requireFail(t, results, "wrong substring")
}

func TestRunFile_RuntimeErrorWrongSubstring(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "wrong substring",
				Mapping: "output = 5 / 0",
				Error:   "overflow",
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return nil, nil, false, fmt.Errorf("division by zero")
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requireFail(t, results, "wrong substring")
}

func TestRunFile_UnexpectedDeletion(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "not expecting deletion",
				Mapping: "output = input",
				Output:  map[string]any{"x": int(1)},
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return nil, nil, true, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requireFail(t, results, "not expecting deletion")
}

func TestRunFile_DeletedButGotError(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "expect deleted but got error",
				Mapping: "output = deleted()",
				Deleted: true,
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return nil, nil, false, fmt.Errorf("something broke")
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requireFail(t, results, "expect deleted but got error")
}

func TestRunFile_DeletedNotSet(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "expect deleted but not deleted",
				Mapping: "output = deleted()",
				Deleted: true,
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return map[string]any{}, map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requireFail(t, results, "expect deleted but not deleted")
}

func TestRunFile_ExplicitOutputMetadata(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:           "explicit metadata match",
				Mapping:        "output@.topic = \"events\"",
				Output:         map[string]any{},
				OutputMetadata: map[string]any{"topic": "events"},
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return map[string]any{}, map[string]any{"topic": "events"}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requirePass(t, results)
}

func TestRunFile_ExplicitOutputMetadataMismatch(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:           "metadata mismatch",
				Mapping:        "output@.topic = \"events\"",
				Output:         map[string]any{},
				OutputMetadata: map[string]any{"topic": "events"},
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return map[string]any{}, map[string]any{"topic": "wrong"}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requireFail(t, results, "metadata mismatch")
}

func TestRunFile_NullInputDefault(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:            "null input default",
				Mapping:         "output = input",
				NoOutputCheck:   true,
				NoMetadataCheck: true,
				// Input not set — defaults to nil.
			},
		},
	}

	var capturedInput any
	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					capturedInput = input
					return nil, map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requirePass(t, results)

	if capturedInput != nil {
		t.Fatalf("expected nil input, got %v (%T)", capturedInput, capturedInput)
	}
}

func TestRunFile_NoOutputCheckWithoutType(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:            "skip output entirely",
				Mapping:         "output = now()",
				NoOutputCheck:   true,
				NoMetadataCheck: true,
				// No OutputType — just skip output comparison entirely.
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return "anything at all", map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requirePass(t, results)
}

func TestRunFile_NoOutputCheckWrongType(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:            "wrong output type",
				Mapping:         "output = now()",
				NoOutputCheck:   true,
				NoMetadataCheck: true,
				OutputType:      "string",
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return int64(42), map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requireFail(t, results, "wrong output type")
}

func TestRun_WithTempDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.yaml"), []byte(`
description: "integration"
tests:
  - name: "passthrough"
    mapping: |
      output = input
    input: {"x": 1}
    output: {"x": 1}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return input, map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results, err := Run(dir, interp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	requirePass(t, results)
}

func TestRun_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := Run(dir, nil)
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
}

func TestRunFile_ValidationRejectsMultipleExpectations(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:         "both error and compile_error",
				Mapping:      "output = 1",
				Error:        "overflow",
				CompileError: "syntax",
			},
		},
	}

	results := RunFile(tf, "test.yaml", nil)
	requireFail(t, results, "both error and compile_error")
	if results[0].Kind != KindInvalidTest {
		t.Fatalf("expected KindInvalidTest, got %v", results[0].Kind)
	}
}

func TestRunFile_ValidationRejectsNoExpectation(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "no expectation",
				Mapping: "output = 1",
			},
		},
	}

	results := RunFile(tf, "test.yaml", nil)
	requireFail(t, results, "no expectation")
	if results[0].Kind != KindInvalidTest {
		t.Fatalf("expected KindInvalidTest, got %v", results[0].Kind)
	}
}

func TestRunFile_ValidationRejectsOutputTypeWithoutNoOutputCheck(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:       "output_type without no_output_check",
				Mapping:    "output = 1",
				Output:     int64(1),
				OutputType: "int64",
			},
		},
	}

	results := RunFile(tf, "test.yaml", nil)
	requireFail(t, results, "output_type without no_output_check")
	if results[0].Kind != KindInvalidTest {
		t.Fatalf("expected KindInvalidTest, got %v", results[0].Kind)
	}
}

func TestRunFile_KindLoadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("{{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := Run(dir, nil)
	if err != nil {
		t.Fatalf("unexpected infrastructure error: %v", err)
	}
	if results[0].Kind != KindLoadError {
		t.Fatalf("expected KindLoadError, got %v", results[0].Kind)
	}
}

func TestRunFile_KindPassOnSuccess(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "passes",
				Mapping: "output = 1",
				Output:  int64(1),
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return int64(1), map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	if results[0].Kind != KindPass {
		t.Fatalf("expected KindPass, got %v", results[0].Kind)
	}
}

// --- Multi-case tests ---

func TestRunFile_MultiCase_AllPassing(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "doubler",
				Mapping: "output.v = input.x * 2",
				Cases: []Case{
					{Name: "positive", Input: map[string]any{"x": int(3)}, Output: map[string]any{"v": int(6)}},
					{Name: "zero", Input: map[string]any{"x": int(0)}, Output: map[string]any{"v": int(0)}},
					{Name: "negative", Input: map[string]any{"x": int(-5)}, Output: map[string]any{"v": int(-10)}},
				},
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					m := input.(map[string]any)
					x := m["x"].(int64)
					return map[string]any{"v": x * 2}, map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	requirePass(t, results)

	// Verify case names are populated.
	for i, name := range []string{"positive", "zero", "negative"} {
		if results[i].Case != name {
			t.Fatalf("result[%d].Case = %q, want %q", i, results[i].Case, name)
		}
		if results[i].Test != "doubler" {
			t.Fatalf("result[%d].Test = %q, want %q", i, results[i].Test, "doubler")
		}
	}
}

func TestRunFile_MultiCase_OneFails(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "doubler",
				Mapping: "output.v = input.x * 2",
				Cases: []Case{
					{Name: "correct", Input: map[string]any{"x": int(3)}, Output: map[string]any{"v": int(6)}},
					{Name: "wrong", Input: map[string]any{"x": int(5)}, Output: map[string]any{"v": int(99)}},
				},
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					m := input.(map[string]any)
					x := m["x"].(int64)
					return map[string]any{"v": x * 2}, map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("expected first case to pass, got: %v", results[0].Err)
	}
	if results[1].Err == nil {
		t.Fatal("expected second case to fail")
	}
	if results[1].Case != "wrong" {
		t.Fatalf("failed case = %q, want %q", results[1].Case, "wrong")
	}
}

func TestRunFile_MultiCase_MixedExpectations(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "mixed",
				Mapping: "output = input",
				Cases: []Case{
					{Name: "output case", Input: int(42), Output: int(42)},
					{Name: "error case", Input: "bad", Error: "kaboom"},
					{Name: "deleted case", Input: nil, Deleted: true},
				},
			},
		},
	}

	callIdx := 0
	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					callIdx++
					switch callIdx {
					case 1:
						return int64(42), map[string]any{}, false, nil
					case 2:
						return nil, nil, false, fmt.Errorf("kaboom: bad input")
					case 3:
						return nil, nil, true, nil
					}
					return nil, nil, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	requirePass(t, results)
}

func TestRunFile_MultiCase_CompileError(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "bad mapping",
				Mapping: "broken",
				Cases: []Case{
					{Name: "a", Output: int(1)},
				},
			},
		},
	}

	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			return nil, &CompileError{Message: "syntax error"}
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for compile error, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Fatal("expected compile error to be reported")
	}
	if results[0].Kind != KindFail {
		t.Fatalf("expected KindFail, got %v", results[0].Kind)
	}
}

func TestRunFile_MultiCase_CompilesOnce(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "shared mapping",
				Mapping: "output = input",
				Cases: []Case{
					{Name: "a", Input: int(1), Output: int(1)},
					{Name: "b", Input: int(2), Output: int(2)},
					{Name: "c", Input: int(3), Output: int(3)},
				},
			},
		},
	}

	compileCount := 0
	interp := &mockInterpreter{
		compileFunc: func(mapping string, files map[string]string) (Mapping, error) {
			compileCount++
			return &mockMapping{
				execFunc: func(input any, metadata map[string]any) (any, map[string]any, bool, error) {
					return input, map[string]any{}, false, nil
				},
			}, nil
		},
	}

	results := RunFile(tf, "test.yaml", interp)
	requirePass(t, results)
	if compileCount != 1 {
		t.Fatalf("expected 1 compile call, got %d", compileCount)
	}
}

func TestRunFile_MultiCase_ValidationRejectsMixedInline(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "mixed inline and cases",
				Mapping: "output = 1",
				Output:  int(1), // inline output
				Cases: []Case{
					{Name: "a", Output: int(1)},
				},
			},
		},
	}

	results := RunFile(tf, "test.yaml", nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Kind != KindInvalidTest {
		t.Fatalf("expected KindInvalidTest, got %v", results[0].Kind)
	}
}

func TestRunFile_MultiCase_ValidationRejectsCompileErrorWithCases(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:         "compile_error with cases",
				Mapping:      "bad",
				CompileError: "syntax",
				Cases: []Case{
					{Name: "a", Output: int(1)},
				},
			},
		},
	}

	results := RunFile(tf, "test.yaml", nil)
	if results[0].Kind != KindInvalidTest {
		t.Fatalf("expected KindInvalidTest, got %v", results[0].Kind)
	}
}

func TestRunFile_MultiCase_ValidationRejectsEmptyCases(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "empty cases",
				Mapping: "output = 1",
				Cases:   []Case{},
			},
		},
	}

	results := RunFile(tf, "test.yaml", nil)
	if results[0].Kind != KindInvalidTest {
		t.Fatalf("expected KindInvalidTest, got %v", results[0].Kind)
	}
}

func TestRunFile_MultiCase_ValidationRejectsCaseWithoutName(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:    "unnamed case",
				Mapping: "output = 1",
				Cases: []Case{
					{Name: "ok", Output: int(1)},
					{Output: int(2)}, // no name
				},
			},
		},
	}

	results := RunFile(tf, "test.yaml", nil)
	if results[0].Kind != KindInvalidTest {
		t.Fatalf("expected KindInvalidTest, got %v", results[0].Kind)
	}
}

func TestResult_StringWithCase(t *testing.T) {
	pass := Result{File: "types/int.yaml", Test: "doubler", Case: "positive"}
	expected := "PASS types/int.yaml / doubler/positive"
	if pass.String() != expected {
		t.Fatalf("got %q, want %q", pass.String(), expected)
	}

	fail := Result{File: "types/int.yaml", Test: "doubler", Case: "negative", Err: fmt.Errorf("mismatch")}
	expected = "FAIL types/int.yaml / doubler/negative: mismatch"
	if fail.String() != expected {
		t.Fatalf("got %q, want %q", fail.String(), expected)
	}
}

func TestRunFile_InvalidInputMetadata(t *testing.T) {
	tf := &TestFile{
		Tests: []TestCase{
			{
				Name:          "bad metadata type",
				Mapping:       "output = 1",
				InputMetadata: "not an object",
				Output:        int64(1),
			},
		},
	}

	results := RunFile(tf, "test.yaml", nil)
	requireFail(t, results, "bad metadata type")
	if results[0].Kind != KindInvalidTest {
		t.Fatalf("expected KindInvalidTest, got %v", results[0].Kind)
	}
}

func TestResult_String(t *testing.T) {
	pass := Result{File: "types/int.yaml", Test: "add ints"}
	if pass.String() != "PASS types/int.yaml / add ints" {
		t.Fatalf("unexpected: %s", pass.String())
	}

	fail := Result{File: "types/int.yaml", Test: "add ints", Err: fmt.Errorf("mismatch")}
	if fail.String() != "FAIL types/int.yaml / add ints: mismatch" {
		t.Fatalf("unexpected: %s", fail.String())
	}
}
