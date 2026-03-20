package llmtest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Tool is a portable tool specification produced by opt funcs (e.g.
// OptToolOnFolder). It contains everything a provider needs to register
// the tool and execute calls against it.
type Tool struct {
	// Name of the tool (e.g. "read_file", "list_files").
	Name string
	// Description for the model.
	Description string
	// Parameters is a JSON Schema object describing the tool's arguments.
	Parameters map[string]any
	// Execute runs the tool with the given arguments and returns the result
	// string. Errors are returned as tool-level errors to the model, not fatal
	// to the evaluation.
	Execute func(args map[string]any) (string, error)
}

// OptToolOnFolder returns tools that give the judge read-only access to a
// directory: list_files (recursive listing) and read_file (read by relative
// path). Path traversal is prevented by resolving paths and confirming they
// remain within the root.
func OptToolOnFolder(dir string) []Tool {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	return []Tool{
		{
			Name:        "list_files",
			Description: "List all files recursively within the provided directory. Returns relative file paths, one per line.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(_ map[string]any) (string, error) {
				return listFiles(absDir)
			},
		},
		{
			Name:        "read_file",
			Description: "Read the contents of a file by its relative path within the provided directory.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path to the file within the directory.",
					},
				},
				"required": []string{"path"},
			},
			Execute: func(args map[string]any) (string, error) {
				path, _ := args["path"].(string)
				if path == "" {
					return "", errors.New("path argument is required")
				}
				return readFileInDir(absDir, path)
			},
		},
	}
}

// OptToolOnFile returns a single read_file tool for a specific file.
func OptToolOnFile(path string) []Tool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	return []Tool{
		{
			Name:        "read_file",
			Description: fmt.Sprintf("Read the contents of the file %q.", filepath.Base(absPath)),
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(_ map[string]any) (string, error) {
				b, err := os.ReadFile(absPath)
				if err != nil {
					return "", fmt.Errorf("failed to read file: %w", err)
				}
				return string(b), nil
			},
		},
	}
}

func listFiles(root string) (string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to list files: %w", err)
	}
	return strings.Join(files, "\n"), nil
}

func readFileInDir(root, relPath string) (string, error) {
	// Resolve the requested path and ensure it stays within the root.
	requested := filepath.Join(root, relPath)
	absRequested, err := filepath.Abs(requested)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Ensure the resolved path is within the root directory.
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("invalid root: %w", err)
	}
	if !strings.HasPrefix(absRequested, absRoot+string(filepath.Separator)) && absRequested != absRoot {
		return "", fmt.Errorf("path %q is outside the allowed directory", relPath)
	}

	b, err := os.ReadFile(absRequested)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(b), nil
}
