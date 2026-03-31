package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/spectest"
)

// generateUUID returns a random UUID v4 string.
func generateUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// envelope wraps a value and its metadata for JSON serialization.
type envelope struct {
	Value    any            `json:"value"`
	Metadata map[string]any `json:"metadata"`
}

// manifestEntry records one eligible test for scoring.
type manifestEntry struct {
	ID              string
	Category        string
	Name            string
	NoMetadataCheck bool
	Expected        envelope
}

// eligibleTest holds all decoded data for a single test, used to build both
// exam modes. Each exam builder picks the subset it needs.
type eligibleTest struct {
	manifestEntry

	Mapping   string
	Input     any
	InputMeta map[string]any
}

// loadEligibleTests walks YAML test files and returns decoded, JSON-encodable
// tests suitable for building either exam mode.
func loadEligibleTests(testsDir string, categories map[string]struct{}) ([]eligibleTest, error) {
	yamlFiles, err := spectest.DiscoverFiles(testsDir)
	if err != nil {
		return nil, err
	}

	var tests []eligibleTest

	for _, path := range yamlFiles {
		rel, err := filepath.Rel(testsDir, path)
		if err != nil {
			rel = path
		}
		category := filepath.Dir(rel)
		baseName := strings.TrimSuffix(filepath.Base(rel), ".yaml")

		if categories != nil {
			if _, ok := categories[category]; !ok {
				continue
			}
		}

		tf, err := spectest.LoadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", rel, err)
			continue
		}

		for i := range tf.Tests {
			tc := &tf.Tests[i]

			if !tc.HasOutput || tc.NoOutputCheck {
				continue
			}
			if tc.CompileError != "" || tc.Error != "" || tc.HasError || tc.Deleted {
				continue
			}
			if len(tf.Files) > 0 || len(tc.Files) > 0 {
				continue
			}

			input, err := spectest.DecodeValue(tc.Input)
			if err != nil {
				continue
			}
			inputMeta, err := decodeMetaMap(tc.InputMetadata)
			if err != nil {
				continue
			}
			output, err := spectest.DecodeValue(tc.Output)
			if err != nil {
				continue
			}
			outputMeta, err := decodeMetaMap(tc.OutputMetadata)
			if err != nil {
				continue
			}

			encodedInput, ok := encodeNaturalJSON(input)
			if !ok {
				continue
			}
			encodedInputMeta, ok := encodeNaturalMeta(inputMeta)
			if !ok {
				continue
			}
			encodedOutput, ok := encodeNaturalJSON(output)
			if !ok {
				continue
			}
			encodedOutputMeta, ok := encodeNaturalMeta(outputMeta)
			if !ok {
				continue
			}

			// JSON round-trip all values so types are normalized to
			// JSON-native forms (e.g. int64 → float64). This ensures
			// consistent comparisons regardless of whether the actual
			// value came from JSON unmarshal or from the interpreter.
			normInput, normInputMeta, err := jsonRoundTripInput(encodedInput, encodedInputMeta)
			if err != nil {
				continue
			}
			normOutput, normOutputMeta, err := jsonRoundTripInput(encodedOutput, encodedOutputMeta)
			if err != nil {
				continue
			}

			id := fmt.Sprintf("%s/%s_%03d", category, baseName, i)
			tests = append(tests, eligibleTest{
				manifestEntry: manifestEntry{
					ID:              id,
					Category:        category,
					Name:            tc.Name,
					NoMetadataCheck: tc.NoMetadataCheck,
					Expected: envelope{
						Value:    normOutput,
						Metadata: normOutputMeta,
					},
				},
				Mapping:   tc.Mapping,
				Input:     normInput,
				InputMeta: normInputMeta,
			})
		}
	}

	return tests, nil
}

func loadSpecDocs(specDir string) (map[string][]byte, error) {
	dirEntries, err := os.ReadDir(specDir)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, d := range dirEntries {
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(specDir, d.Name()))
		if err != nil {
			return nil, err
		}
		files[filepath.Join("spec", d.Name())] = data
	}
	return files, nil
}

// jsonRoundTripInput marshals value and metadata to JSON and back so that
// number types are normalized (int64 → float64, etc.), matching what a mapping
// would receive from a real JSON document.
func jsonRoundTripInput(value any, meta map[string]any) (any, map[string]any, error) {
	data, err := json.Marshal(envelope{Value: value, Metadata: meta})
	if err != nil {
		return nil, nil, err
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, nil, err
	}
	if env.Metadata == nil {
		env.Metadata = map[string]any{}
	}
	return env.Value, env.Metadata, nil
}

// indentLines prefixes each line of s (after the first) with prefix,
// suitable for indenting multi-line strings in log output.
func indentLines(s, prefix string) string {
	return strings.ReplaceAll(s, "\n", "\n"+prefix)
}

func marshalEnvelope(e envelope) ([]byte, error) {
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func decodeMetaMap(raw any) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}
	decoded, err := spectest.DecodeValue(raw)
	if err != nil {
		return nil, err
	}
	m, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("metadata must be an object, got %T", decoded)
	}
	return m, nil
}

func parseCategories(s string) map[string]struct{} {
	if s == "" {
		return nil
	}
	cats := map[string]struct{}{}
	for _, c := range strings.Split(s, ",") {
		c = strings.TrimSpace(c)
		if c != "" {
			cats[c] = struct{}{}
		}
	}
	if len(cats) == 0 {
		return nil
	}
	return cats
}

// --- Natural JSON helpers ---

func encodeNaturalJSON(v any) (any, bool) {
	switch val := v.(type) {
	case nil:
		return nil, true
	case bool:
		return val, true
	case string:
		return val, true
	case int64:
		return val, true
	case int32:
		return int64(val), true
	case uint32:
		return int64(val), true
	case uint64:
		if val <= math.MaxInt64 {
			return int64(val), true
		}
		return float64(val), true
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return nil, false
		}
		return val, true
	case float32:
		f := float64(val)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, false
		}
		return f, true
	case []byte:
		return nil, false
	case time.Time:
		return nil, false
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			enc, ok := encodeNaturalJSON(item)
			if !ok {
				return nil, false
			}
			out[i] = enc
		}
		return out, true
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			enc, ok := encodeNaturalJSON(item)
			if !ok {
				return nil, false
			}
			out[k] = enc
		}
		return out, true
	default:
		return nil, false
	}
}

func encodeNaturalMeta(m map[string]any) (map[string]any, bool) {
	if len(m) == 0 {
		return map[string]any{}, true
	}
	encoded, ok := encodeNaturalJSON(m)
	if !ok {
		return nil, false
	}
	em, _ := encoded.(map[string]any)
	return em, em != nil
}

func coerceToNaturalJSON(v any) any {
	switch val := v.(type) {
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	case float32:
		return float64(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = coerceToNaturalJSON(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = coerceToNaturalJSON(item)
		}
		return out
	default:
		return val
	}
}

func naturalJSONEqual(expected, actual any) (bool, string) {
	return naturalEqual(expected, actual, "root")
}

func naturalEqual(expected, actual any, path string) (bool, string) {
	if expected == nil && actual == nil {
		return true, ""
	}
	if expected == nil || actual == nil {
		return false, fmt.Sprintf("%s: expected %v (%T), got %v (%T)", path, expected, expected, actual, actual)
	}

	switch ev := expected.(type) {
	case float64:
		av, ok := actual.(float64)
		if !ok {
			return false, fmt.Sprintf("%s: expected number (%v), got %T (%v)", path, ev, actual, actual)
		}
		if ev != av {
			return false, fmt.Sprintf("%s: expected %v, got %v", path, ev, av)
		}
		return true, ""
	case string:
		av, ok := actual.(string)
		if !ok {
			return false, fmt.Sprintf("%s: expected string (%q), got %T (%v)", path, ev, actual, actual)
		}
		if ev != av {
			return false, fmt.Sprintf("%s: expected %q, got %q", path, ev, av)
		}
		return true, ""
	case bool:
		av, ok := actual.(bool)
		if !ok {
			return false, fmt.Sprintf("%s: expected bool (%v), got %T (%v)", path, ev, actual, actual)
		}
		if ev != av {
			return false, fmt.Sprintf("%s: expected %v, got %v", path, ev, av)
		}
		return true, ""
	case []any:
		av, ok := actual.([]any)
		if !ok {
			return false, fmt.Sprintf("%s: expected array, got %T", path, actual)
		}
		if len(ev) != len(av) {
			return false, fmt.Sprintf("%s: array length: expected %d, got %d", path, len(ev), len(av))
		}
		for i := range ev {
			if ok, diff := naturalEqual(ev[i], av[i], fmt.Sprintf("%s[%d]", path, i)); !ok {
				return false, diff
			}
		}
		return true, ""
	case map[string]any:
		av, ok := actual.(map[string]any)
		if !ok {
			return false, fmt.Sprintf("%s: expected object, got %T", path, actual)
		}
		var diffs []string
		for k := range ev {
			if _, ok := av[k]; !ok {
				diffs = append(diffs, fmt.Sprintf("%s: missing key %q", path, k))
			}
		}
		for k := range av {
			if _, ok := ev[k]; !ok {
				diffs = append(diffs, fmt.Sprintf("%s: unexpected key %q", path, k))
			}
		}
		if len(diffs) > 0 {
			sort.Strings(diffs)
			return false, strings.Join(diffs, "\n")
		}
		for k := range ev {
			if ok, diff := naturalEqual(ev[k], av[k], path+"."+k); !ok {
				return false, diff
			}
		}
		return true, ""
	default:
		return false, fmt.Sprintf("%s: unexpected type %T", path, expected)
	}
}
