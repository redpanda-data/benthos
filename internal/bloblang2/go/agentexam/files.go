package agentexam

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DirFiles reads a directory tree and returns its contents as a map of
// relative paths to file contents, suitable for use as Exam.Files.
// Only regular files are included; symlinks are skipped.
func DirFiles(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("computing relative path for %s: %w", path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", root, err)
	}
	return files, nil
}
