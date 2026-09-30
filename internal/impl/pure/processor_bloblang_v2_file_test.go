// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"
)

func parseBloblangV2FileProc(t *testing.T, path string) (*bloblangV2FileProc, error) {
	t.Helper()
	conf, err := bloblangV2FileProcConfig().ParseYAML(path, nil)
	require.NoError(t, err)
	return newBloblangV2FileFromParsed(conf, service.MockResources())
}

func TestBloblangV2FileProcessorReadsAndExecutes(t *testing.T) {
	dir := t.TempDir()
	mappingPath := filepath.Join(dir, "mapping.blobl")
	require.NoError(t, os.WriteFile(mappingPath, []byte(
		`output.upper_name = input.name.uppercase()`+"\n",
	), 0o644))

	proc, err := parseBloblangV2FileProc(t, mappingPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = proc.Close(t.Context()) })

	msg := service.NewMessage(nil)
	msg.SetStructured(map[string]any{"name": "blob"})

	batches, err := proc.ProcessBatch(t.Context(), service.MessageBatch{msg})
	require.NoError(t, err)
	require.Len(t, batches, 1)
	require.Len(t, batches[0], 1)

	got, err := batches[0][0].AsStructured()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"upper_name": "BLOB"}, got)
}

func TestBloblangV2FileProcessorPreservesPath(t *testing.T) {
	dir := t.TempDir()
	mappingPath := filepath.Join(dir, "mapping.blobl")
	require.NoError(t, os.WriteFile(mappingPath, []byte(`output = input`), 0o644))

	proc, err := parseBloblangV2FileProc(t, mappingPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = proc.Close(t.Context()) })

	assert.Equal(t, mappingPath, proc.path)
}

func TestBloblangV2FileProcessorMissingFile(t *testing.T) {
	_, err := parseBloblangV2FileProc(t, "/does/not/exist.blobl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bloblang_v2_file")
	assert.Contains(t, err.Error(), "opening")
}

func TestBloblangV2FileProcessorEmptyPath(t *testing.T) {
	_, err := parseBloblangV2FileProc(t, `""`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

func TestBloblangV2FileProcessorInvalidMapping(t *testing.T) {
	dir := t.TempDir()
	mappingPath := filepath.Join(dir, "broken.blobl")
	require.NoError(t, os.WriteFile(mappingPath, []byte(`@@@ not bloblang @@@`), 0o644))

	_, err := parseBloblangV2FileProc(t, mappingPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing")
}

// TestBloblangV2FileProcessorLintsClean exercises the schema-level
// linter against a stream config containing the new processor,
// ensuring the registration is wired correctly into the global env
// and the field shape matches what users will write.
func TestBloblangV2FileProcessorLintsClean(t *testing.T) {
	dir := t.TempDir()
	mappingPath := filepath.Join(dir, "mapping.blobl")
	require.NoError(t, os.WriteFile(mappingPath, []byte(`output = input`), 0o644))

	yamlConfig := []byte(`
input:
  generate:
    count: 1
    interval: ""
    mapping: 'root = {"id": "abc"}'
pipeline:
  processors:
    - bloblang_v2_file: ` + mappingPath + `
output:
  drop: {}
`)

	schema := service.GlobalEnvironment().FullConfigSchema("", "")
	linter := schema.NewStreamConfigLinter()
	lints, err := linter.LintYAML(yamlConfig)
	require.NoError(t, err)
	for _, l := range lints {
		t.Errorf("unexpected lint: %+v", l)
	}
}

func TestBloblangV2FileProcessorMetadataOverwrite(t *testing.T) {
	dir := t.TempDir()
	mappingPath := filepath.Join(dir, "meta.blobl")
	require.NoError(t, os.WriteFile(mappingPath, []byte(`
output = input
output@.stamp = "added"
`), 0o644))

	proc, err := parseBloblangV2FileProc(t, mappingPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = proc.Close(t.Context()) })

	msg := service.NewMessage(nil)
	msg.SetStructured(map[string]any{"id": "x"})
	msg.MetaSetMut("original", "should-be-dropped")

	batches, err := proc.ProcessBatch(t.Context(), service.MessageBatch{msg})
	require.NoError(t, err)
	require.Len(t, batches, 1)
	require.Len(t, batches[0], 1)

	stamp, ok := batches[0][0].MetaGet("stamp")
	require.True(t, ok)
	assert.Equal(t, "added", stamp)

	_, exists := batches[0][0].MetaGet("original")
	assert.False(t, exists, "V2 metadata semantics should drop input metadata not assigned in output@")
}
