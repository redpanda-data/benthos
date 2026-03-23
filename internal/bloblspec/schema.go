package bloblspec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// TestFile represents one YAML test file.
type TestFile struct {
	Description string            `yaml:"description"`
	Files       map[string]string `yaml:"files"`
	Tests       []TestCase        `yaml:"tests"`
}

// TestCase is a single test within a file.
type TestCase struct {
	Name            string            `yaml:"name"`
	Mapping         string            `yaml:"mapping"`
	Input           any               `yaml:"input"`
	InputMetadata   any               `yaml:"input_metadata"`
	Output          any               `yaml:"output"`
	OutputMetadata  any               `yaml:"output_metadata"`
	Error           string            `yaml:"error"`
	CompileError    string            `yaml:"compile_error"`
	Deleted         bool              `yaml:"deleted"`
	NoOutputCheck   bool              `yaml:"no_output_check"`
	NoMetadataCheck bool              `yaml:"no_metadata_check"`
	OutputType      string            `yaml:"output_type"`
	Files           map[string]string `yaml:"files"`
	HasOutput       bool              `yaml:"-"` // set by custom unmarshaling; true when output field is present
	HasError        bool              `yaml:"-"` // set by custom unmarshaling; true when error field is present
}

// UnmarshalYAML implements custom unmarshaling to detect when the output
// field is explicitly set (including to null).
func (tc *TestCase) UnmarshalYAML(value *yaml.Node) error {
	// Use an alias type to avoid infinite recursion.
	type rawTestCase TestCase
	var raw rawTestCase
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*tc = TestCase(raw)

	// Check if "output" key is present in the YAML mapping.
	if value.Kind == yaml.MappingNode {
		for i := 0; i < len(value.Content)-1; i += 2 {
			switch value.Content[i].Value {
			case "output":
				tc.HasOutput = true
			case "error":
				tc.HasError = true
			}
		}
	}
	return nil
}

// LoadFile reads and unmarshals a single YAML test file.
func LoadFile(path string) (*TestFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading test file %s: %w", path, err)
	}
	var tf TestFile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parsing test file %s: %w", path, err)
	}
	return &tf, nil
}

// DiscoverFiles recursively finds all .yaml files under dir, returning
// paths sorted lexicographically for deterministic ordering.
func DiscoverFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".yaml") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discovering test files in %s: %w", dir, err)
	}
	sort.Strings(files)
	return files, nil
}
