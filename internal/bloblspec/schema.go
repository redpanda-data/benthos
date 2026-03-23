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
