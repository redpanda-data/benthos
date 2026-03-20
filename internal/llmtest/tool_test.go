package llmtest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptToolOnFolder(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"hello":"world"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("nested content"), 0o644))

	tools := OptToolOnFolder(dir)
	require.Len(t, tools, 2)

	// Find tools by name.
	var listTool, readTool Tool
	for _, tool := range tools {
		switch tool.Name {
		case "list_files":
			listTool = tool
		case "read_file":
			readTool = tool
		}
	}
	require.NotEmpty(t, listTool.Name, "list_files tool not found")
	require.NotEmpty(t, readTool.Name, "read_file tool not found")

	// list_files
	result, err := listTool.Execute(nil)
	require.NoError(t, err)
	assert.Contains(t, result, "a.json")
	assert.Contains(t, result, filepath.Join("sub", "b.txt"))

	// read_file — valid path
	result, err = readTool.Execute(map[string]any{"path": "a.json"})
	require.NoError(t, err)
	assert.Equal(t, `{"hello":"world"}`, result)

	// read_file — nested path
	result, err = readTool.Execute(map[string]any{"path": filepath.Join("sub", "b.txt")})
	require.NoError(t, err)
	assert.Equal(t, "nested content", result)

	// read_file — path traversal blocked
	_, err = readTool.Execute(map[string]any{"path": "../../../etc/passwd"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "outside the allowed directory")

	// read_file — missing path arg
	_, err = readTool.Execute(map[string]any{})
	assert.Error(t, err)
}

func TestOptToolOnFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.json")
	require.NoError(t, os.WriteFile(f, []byte(`{"key":"value"}`), 0o644))

	tools := OptToolOnFile(f)
	require.Len(t, tools, 1)

	result, err := tools[0].Execute(nil)
	require.NoError(t, err)
	assert.Equal(t, `{"key":"value"}`, result)
}

func TestOptToolOnFile_NotFound(t *testing.T) {
	tools := OptToolOnFile("/nonexistent/file.json")
	require.Len(t, tools, 1)

	_, err := tools[0].Execute(nil)
	assert.Error(t, err)
}
