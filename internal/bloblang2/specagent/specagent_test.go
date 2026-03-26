package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- encodeNaturalJSON ---

func TestEncodeNaturalJSON(t *testing.T) {
	tests := []struct {
		name string
		in   any
		ok   bool
	}{
		{"nil", nil, true},
		{"bool", true, true},
		{"string", "hello", true},
		{"int64", int64(42), true},
		{"int32", int32(7), true},
		{"uint32", uint32(100), true},
		{"uint64", uint64(999), true},
		{"float64", float64(3.14), true},
		{"float32", float32(1.5), true},
		{"array", []any{int64(1), "two"}, true},
		{"object", map[string]any{"k": int64(1)}, true},
		{"nested", map[string]any{"a": []any{int64(1), float64(2.0)}}, true},

		// Rejected types.
		{"bytes", []byte("hello"), false},
		{"timestamp", time.Now(), false},
		{"NaN", float64NaN(), false},
		{"Inf", float64Inf(), false},
		{"nested bytes", map[string]any{"data": []byte{1}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := encodeNaturalJSON(tt.in)
			if ok != tt.ok {
				t.Fatalf("encodeNaturalJSON ok = %v, want %v", ok, tt.ok)
			}
		})
	}
}

func TestEncodeNaturalJSONCoercesIntTypes(t *testing.T) {
	// int32, uint32 should become int64.
	v, ok := encodeNaturalJSON(int32(42))
	if !ok {
		t.Fatal("expected ok")
	}
	if _, isInt64 := v.(int64); !isInt64 {
		t.Fatalf("expected int64, got %T", v)
	}
}

// --- coerceToNaturalJSON ---

func TestCoerceToNaturalJSON(t *testing.T) {
	input := map[string]any{
		"int":    int64(42),
		"float":  float64(3.14),
		"int32":  int32(7),
		"uint32": uint32(100),
		"str":    "hello",
		"nested": []any{int64(1), float32(2.5)},
	}
	result := coerceToNaturalJSON(input)
	m := result.(map[string]any)

	for key, val := range m {
		if key == "str" {
			if _, ok := val.(string); !ok {
				t.Errorf("str: expected string, got %T", val)
			}
			continue
		}
		if key == "nested" {
			arr := val.([]any)
			for i, item := range arr {
				if _, ok := item.(float64); !ok {
					t.Errorf("nested[%d]: expected float64, got %T", i, item)
				}
			}
			continue
		}
		if _, ok := val.(float64); !ok {
			t.Errorf("%s: expected float64, got %T (%v)", key, val, val)
		}
	}
}

// --- naturalJSONEqual ---

func TestNaturalJSONEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     any
		wantPass bool
	}{
		{"nil nil", nil, nil, true},
		{"nil vs value", nil, float64(1), false},
		{"numbers equal", float64(42), float64(42), true},
		{"numbers differ", float64(42), float64(43), false},
		{"strings equal", "hello", "hello", true},
		{"strings differ", "hello", "world", false},
		{"bools equal", true, true, true},
		{"bools differ", true, false, false},
		{"type mismatch", float64(1), "1", false},

		{"arrays equal", []any{float64(1), "two"}, []any{float64(1), "two"}, true},
		{"arrays differ length", []any{float64(1)}, []any{float64(1), float64(2)}, false},
		{"arrays differ value", []any{float64(1)}, []any{float64(2)}, false},

		{"objects equal",
			map[string]any{"a": float64(1), "b": "two"},
			map[string]any{"a": float64(1), "b": "two"}, true},
		{"objects missing key",
			map[string]any{"a": float64(1), "b": float64(2)},
			map[string]any{"a": float64(1)}, false},
		{"objects extra key",
			map[string]any{"a": float64(1)},
			map[string]any{"a": float64(1), "b": float64(2)}, false},
		{"objects differ value",
			map[string]any{"a": float64(1)},
			map[string]any{"a": float64(2)}, false},

		{"nested match",
			map[string]any{"arr": []any{map[string]any{"x": float64(1)}}},
			map[string]any{"arr": []any{map[string]any{"x": float64(1)}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, diff := naturalJSONEqual(tt.a, tt.b)
			if ok != tt.wantPass {
				t.Fatalf("naturalJSONEqual = %v (want %v), diff: %s", ok, tt.wantPass, diff)
			}
		})
	}
}

// --- JSON roundtrip ---

func TestJSONRoundtrip(t *testing.T) {
	// Verify that values survive encode → json.Marshal → json.Unmarshal
	// and can be compared with naturalJSONEqual.
	original := map[string]any{
		"int":   int64(42),
		"float": float64(3.14),
		"str":   "hello",
		"bool":  true,
		"null":  nil,
		"array": []any{int64(1), float64(2.5), "three"},
		"obj":   map[string]any{"nested": int64(7)},
	}
	encoded, ok := encodeNaturalJSON(original)
	if !ok {
		t.Fatal("encodeNaturalJSON failed")
	}

	data, err := json.Marshal(encoded)
	if err != nil {
		t.Fatal(err)
	}

	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	// After unmarshal, all numbers are float64. Coerce original too.
	coerced := coerceToNaturalJSON(encoded)

	pass, diff := naturalJSONEqual(coerced, decoded)
	if !pass {
		t.Fatalf("roundtrip mismatch: %s", diff)
	}
}

// --- manifest roundtrip ---

func TestManifestRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	original := &manifest{
		Tests: []manifestEntry{
			{
				ID:       "test/foo_000",
				Category: "test",
				Name:     "int value",
				Expected: envelope{
					Value:    int64(42),
					Metadata: map[string]any{},
				},
			},
			{
				ID:       "test/foo_001",
				Category: "test",
				Name:     "float value",
				Expected: envelope{
					Value:    float64(3.14),
					Metadata: map[string]any{},
				},
			},
			{
				ID:       "test/foo_002",
				Category: "test",
				Name:     "complex object",
				Expected: envelope{
					Value: map[string]any{
						"name":  "Alice",
						"age":   int64(30),
						"score": float64(9.5),
					},
					Metadata: map[string]any{"source": "test"},
				},
			},
			{
				ID:       "test/foo_003",
				Category: "test",
				Name:     "null value",
				Expected: envelope{Value: nil, Metadata: map[string]any{}},
			},
		},
	}

	if err := writeManifest(path, original); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Tests) != len(original.Tests) {
		t.Fatalf("test count: got %d, want %d", len(loaded.Tests), len(original.Tests))
	}

	// After json roundtrip all numbers become float64.
	// Coerce originals and compare.
	for i, entry := range loaded.Tests {
		orig := original.Tests[i]
		expected := coerceToNaturalJSON(orig.Expected.Value)
		ok, diff := naturalJSONEqual(expected, entry.Expected.Value)
		if !ok {
			t.Errorf("test %d (%s): %s", i, entry.Name, diff)
		}
	}
}

// --- writeJSONFile / readJSONFile roundtrip ---

func TestWriteReadJSONFileRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	original := envelope{
		Value: map[string]any{
			"items": []any{int64(1), int64(2), int64(3)},
			"total": float64(6.0),
		},
		Metadata: map[string]any{"key": "value"},
	}

	if err := writeJSONFile(path, original); err != nil {
		t.Fatal(err)
	}
	raw, err := readJSONFile(path)
	if err != nil {
		t.Fatal(err)
	}

	env := raw.(map[string]any)
	value := env["value"].(map[string]any)

	// After json roundtrip all numbers are float64.
	items := value["items"].([]any)
	for i, item := range items {
		f, ok := item.(float64)
		if !ok {
			t.Errorf("items[%d]: expected float64, got %T", i, item)
		}
		if f != float64(i+1) {
			t.Errorf("items[%d]: expected %v, got %v", i, float64(i+1), f)
		}
	}

	total := value["total"].(float64)
	if total != 6.0 {
		t.Errorf("total: expected 6.0, got %v", total)
	}
}

// --- evaluate predict_output ---

func TestEvaluatePredictOutput(t *testing.T) {
	dir := t.TempDir()
	crDir := filepath.Join(dir, "predict_output")
	testsDir := filepath.Join(crDir, "tests", "cat")
	os.MkdirAll(testsDir, 0o755)

	mf := &manifest{
		Tests: []manifestEntry{
			{
				ID:       "cat/test_000",
				Category: "cat",
				Name:     "correct answer",
				Expected: envelope{
					Value:    map[string]any{"v": "Alice"},
					Metadata: map[string]any{},
				},
			},
			{
				ID:       "cat/test_001",
				Category: "cat",
				Name:     "wrong answer",
				Expected: envelope{
					Value:    float64(42),
					Metadata: map[string]any{},
				},
			},
			{
				ID:       "cat/test_002",
				Category: "cat",
				Name:     "missing file",
				Expected: envelope{
					Value:    "anything",
					Metadata: map[string]any{},
				},
			},
		},
	}

	// Roundtrip through manifest to get float64 numbers.
	mfPath := filepath.Join(dir, "manifest.json")
	writeManifest(mfPath, mf)
	mf, _ = loadManifest(mfPath)

	// Correct output.
	writeJSONFile(filepath.Join(testsDir, "test_000.output.json"), envelope{
		Value:    map[string]any{"v": "Alice"},
		Metadata: map[string]any{},
	})

	// Wrong output.
	writeJSONFile(filepath.Join(testsDir, "test_001.output.json"), envelope{
		Value:    float64(99),
		Metadata: map[string]any{},
	})

	// test_002: no file.

	results := evaluatePredictOutput(crDir, mf)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("test_000 should pass: %s", results[0].Error)
	}
	if results[1].Passed {
		t.Error("test_001 should fail (wrong value)")
	}
	if results[2].Passed {
		t.Error("test_002 should fail (missing file)")
	}
}

// --- evaluate predict_mapping ---

func TestEvaluatePredictMapping(t *testing.T) {
	dir := t.TempDir()
	crDir := filepath.Join(dir, "predict_mapping")
	testsDir := filepath.Join(crDir, "tests", "cat")
	os.MkdirAll(testsDir, 0o755)

	mf := &manifest{
		Tests: []manifestEntry{
			{
				ID:       "cat/test_000",
				Category: "cat",
				Name:     "correct mapping",
				Expected: envelope{
					Value:    map[string]any{"v": "Alice"},
					Metadata: map[string]any{},
				},
			},
			{
				ID:       "cat/test_001",
				Category: "cat",
				Name:     "wrong mapping",
				Expected: envelope{
					Value:    map[string]any{"v": "Alice"},
					Metadata: map[string]any{},
				},
			},
			{
				ID:       "cat/test_002",
				Category: "cat",
				Name:     "compile error",
				Expected: envelope{
					Value:    float64(1),
					Metadata: map[string]any{},
				},
			},
		},
	}

	mfPath := filepath.Join(dir, "manifest.json")
	writeManifest(mfPath, mf)
	mf, _ = loadManifest(mfPath)

	input := envelope{
		Value:    map[string]any{"name": "Alice"},
		Metadata: map[string]any{},
	}
	for _, id := range []string{"test_000", "test_001", "test_002"} {
		writeJSONFile(filepath.Join(testsDir, id+".input.json"), input)
	}

	os.WriteFile(filepath.Join(testsDir, "test_000.blobl2"), []byte("output.v = input.name\n"), 0o644)
	os.WriteFile(filepath.Join(testsDir, "test_001.blobl2"), []byte("output.v = \"Bob\"\n"), 0o644)
	os.WriteFile(filepath.Join(testsDir, "test_002.blobl2"), []byte("this is not valid\n"), 0o644)

	results := evaluatePredictMapping(crDir, mf)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("test_000 should pass: %s", results[0].Error)
	}
	if results[1].Passed {
		t.Error("test_001 should fail (wrong output)")
	}
	if results[2].Passed {
		t.Error("test_002 should fail (compile error)")
	}
	if !strings.Contains(results[2].Error, "compile error") {
		t.Errorf("test_002 error should mention compile, got: %s", results[2].Error)
	}
}

// --- copySpecDocs ---

func TestCopySpecDocsOnlyCopiesTopLevelMD(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	os.WriteFile(filepath.Join(src, "01_overview.md"), []byte("# Overview"), 0o644)
	os.WriteFile(filepath.Join(src, "README.md"), []byte("# README"), 0o644)
	os.WriteFile(filepath.Join(src, "not_md.txt"), []byte("skip me"), 0o644)
	os.MkdirAll(filepath.Join(src, "tests"), 0o755)
	os.WriteFile(filepath.Join(src, "tests", "README.md"), []byte("# Test README"), 0o644)
	os.MkdirAll(filepath.Join(src, "other_subdir"), 0o755)
	os.WriteFile(filepath.Join(src, "other_subdir", "notes.md"), []byte("# Notes"), 0o644)

	if err := copySpecDocs(src, dst); err != nil {
		t.Fatal(err)
	}

	// Top-level .md files should be copied.
	if _, err := os.Stat(filepath.Join(dst, "01_overview.md")); err != nil {
		t.Error("01_overview.md should exist")
	}
	if _, err := os.Stat(filepath.Join(dst, "README.md")); err != nil {
		t.Error("README.md should exist")
	}
	// Non-.md files should not be copied.
	if _, err := os.Stat(filepath.Join(dst, "not_md.txt")); !os.IsNotExist(err) {
		t.Error("not_md.txt should not be copied")
	}
	// No subdirectories or their contents should be copied.
	if _, err := os.Stat(filepath.Join(dst, "tests")); !os.IsNotExist(err) {
		t.Error("tests/ directory should not exist in destination")
	}
	if _, err := os.Stat(filepath.Join(dst, "other_subdir")); !os.IsNotExist(err) {
		t.Error("other_subdir/ should not exist in destination")
	}
	if _, err := os.Stat(filepath.Join(dst, "notes.md")); !os.IsNotExist(err) {
		t.Error("notes.md from subdirectory should not be copied")
	}
}

// --- loadEligibleTests against real spec tests ---

func TestLoadEligibleTestsReal(t *testing.T) {
	testsDir := filepath.Join("..", "spec", "tests")
	if _, err := os.Stat(testsDir); os.IsNotExist(err) {
		t.Skip("spec/tests not found")
	}

	entries, err := loadEligibleTests(testsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least some eligible tests")
	}

	for _, e := range entries {
		if e.ID == "" || e.Category == "" || e.Name == "" || e.mapping == "" {
			t.Errorf("entry %s has empty required field", e.ID)
		}
	}

	// Verify manifest roundtrip preserves values.
	dir := t.TempDir()
	mfPath := filepath.Join(dir, "manifest.json")
	writeManifest(mfPath, &manifest{Tests: entries})
	loaded, err := loadManifest(mfPath)
	if err != nil {
		t.Fatal(err)
	}

	for i, entry := range loaded.Tests {
		orig := entries[i]
		// Both should be the same after json roundtrip (both go through
		// encodeNaturalJSON → json.Marshal → json.Unmarshal).
		origCoerced := coerceToNaturalJSON(orig.Expected.Value)
		ok, diff := naturalJSONEqual(origCoerced, entry.Expected.Value)
		if !ok {
			t.Errorf("entry %s: manifest roundtrip mismatch: %s", entry.ID, diff)
		}
	}
}

// --- end-to-end: prepare + evaluate ---

func TestPrepareAndEvaluateEndToEnd(t *testing.T) {
	specDir := filepath.Join("..", "spec")
	testsDir := filepath.Join("..", "spec", "tests")
	if _, err := os.Stat(testsDir); os.IsNotExist(err) {
		t.Skip("spec/tests not found")
	}

	outDir := t.TempDir()

	entries, err := loadEligibleTests(testsDir)
	if err != nil {
		t.Fatal(err)
	}

	poDir := filepath.Join(outDir, "predict_output")
	pmDir := filepath.Join(outDir, "predict_mapping")

	if err := generateCleanRoom(poDir, specDir, entries, true); err != nil {
		t.Fatal(err)
	}
	if err := generateCleanRoom(pmDir, specDir, entries, false); err != nil {
		t.Fatal(err)
	}
	writeManifest(filepath.Join(outDir, "manifest.json"), &manifest{Tests: entries})
	mf, err := loadManifest(filepath.Join(outDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Simulate perfect agent for predict_output: copy expected output
	// from predict_mapping clean room into predict_output.
	for _, entry := range entries {
		src := filepath.Join(pmDir, "tests", entry.ID+".output.json")
		dst := filepath.Join(poDir, "tests", entry.ID+".output.json")
		data, _ := os.ReadFile(src)
		os.WriteFile(dst, data, 0o644)
	}

	poResults := evaluatePredictOutput(poDir, mf)
	var poFails []string
	for _, r := range poResults {
		if !r.Passed {
			poFails = append(poFails, r.ID+": "+r.Error)
		}
	}
	if len(poFails) > 0 {
		t.Errorf("predict_output: %d/%d failed with perfect answers:\n  %s",
			len(poFails), len(poResults), strings.Join(poFails, "\n  "))
	}

	// Simulate perfect agent for predict_mapping: copy original
	// mappings into predict_mapping clean room.
	for _, entry := range entries {
		src := filepath.Join(poDir, "tests", entry.ID+".blobl2")
		dst := filepath.Join(pmDir, "tests", entry.ID+".blobl2")
		data, _ := os.ReadFile(src)
		os.WriteFile(dst, data, 0o644)
	}

	// For predict_mapping, a handful of original mappings are expected to
	// fail when their input goes through a JSON roundtrip (integers
	// become float64, which affects string interpolation and type
	// introspection). A real agent wouldn't hit this since it writes
	// mappings for JSON-decoded input. Assert >= 99% pass rate.
	pmResults := evaluatePredictMapping(pmDir, mf)
	var pmFails []string
	for _, r := range pmResults {
		if !r.Passed {
			pmFails = append(pmFails, r.ID+": "+r.Error)
		}
	}
	pmPassRate := float64(len(pmResults)-len(pmFails)) / float64(len(pmResults)) * 100
	if pmPassRate < 99.0 {
		t.Errorf("predict_mapping: %.1f%% pass rate (%d/%d failed), want >= 99%%:\n  %s",
			pmPassRate, len(pmFails), len(pmResults), strings.Join(pmFails, "\n  "))
	} else if len(pmFails) > 0 {
		t.Logf("predict_mapping: %d/%d expected failures (type-sensitive originals with JSON input)",
			len(pmFails), len(pmResults))
	}
}

// --- formatStats ---

func TestFormatStats(t *testing.T) {
	tests := []struct {
		pass, total int
		want        string
	}{
		{0, 0, "N/A"},
		{0, 10, "  0.0% (0/10)"},
		{10, 10, "100.0% (10/10)"},
		{3, 4, " 75.0% (3/4)"},
	}
	for _, tt := range tests {
		got := formatStats(tt.pass, tt.total)
		if got != tt.want {
			t.Errorf("formatStats(%d, %d) = %q, want %q", tt.pass, tt.total, got, tt.want)
		}
	}
}

// --- helpers ---

func float64NaN() float64 {
	var zero float64
	return zero / zero
}

func float64Inf() float64 {
	var zero float64
	return 1.0 / zero
}
