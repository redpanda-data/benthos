// Copyright 2026 Redpanda Data, Inc.

package pure_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func TestBloblangV2Enumerated(t *testing.T) {
	got := runBloblangV2(t, `output = input.enumerated()`, []any{"a", "b"})
	assert.Equal(t, []any{
		map[string]any{"index": int64(0), "value": "a"},
		map[string]any{"index": int64(1), "value": "b"},
	}, got)
}

func TestBloblangV2FindAll(t *testing.T) {
	got := runBloblangV2(t, `output = input.find_all("bar")`, []any{"foo", "bar", "baz", "bar"})
	assert.Equal(t, []any{int64(1), int64(3)}, got)
}

func TestBloblangV2FindAllNumericLooseEquality(t *testing.T) {
	got := runBloblangV2(t, `output = input.find_all(20)`, []any{10.3, 20.0, "huh", int64(20)})
	assert.Equal(t, []any{int64(1), int64(3)}, got)
}

func TestBloblangV2FindAllEmpty(t *testing.T) {
	got := runBloblangV2(t, `output = input.find_all("nope")`, []any{"a", "b"})
	assert.Equal(t, []any{}, got)
}

func TestBloblangV2Index(t *testing.T) {
	got := runBloblangV2(t, `output = input.index(0)`, []any{"first", "second"})
	assert.Equal(t, "first", got)

	got = runBloblangV2(t, `output = input.index(-1)`, []any{"first", "second"})
	assert.Equal(t, "second", got)
}

func TestBloblangV2IndexOutOfBounds(t *testing.T) {
	exec, err := bloblangv2.GlobalEnvironment().Parse(`output = input.index(10)`)
	require.NoError(t, err)
	_, err = exec.Query([]any{"a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
}

func TestBloblangV2NotEmpty(t *testing.T) {
	got := runBloblangV2(t, `output = input.not_empty()`, "hello")
	assert.Equal(t, "hello", got)

	got = runBloblangV2(t, `output = input.not_empty()`, []any{"a"})
	assert.Equal(t, []any{"a"}, got)
}

func TestBloblangV2NotEmptyErrors(t *testing.T) {
	cases := []struct {
		input any
		want  string
	}{
		{"", "string value is empty"},
		{[]any{}, "array value is empty"},
		{map[string]any{}, "object value is empty"},
	}
	for _, tc := range cases {
		exec, err := bloblangv2.GlobalEnvironment().Parse(`output = input.not_empty()`)
		require.NoError(t, err)
		_, err = exec.Query(tc.input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), tc.want)
	}
}

func TestBloblangV2Collapse(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.collapse()`,
		map[string]any{"foo": []any{
			map[string]any{"bar": "1"},
			map[string]any{"bar": map[string]any{}},
			map[string]any{"bar": "2"},
		}},
	)
	assert.Equal(t, map[string]any{
		"foo.0.bar": "1",
		"foo.2.bar": "2",
	}, got)
}

func TestBloblangV2CollapseIncludeEmpty(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.collapse(include_empty: true)`,
		map[string]any{"foo": map[string]any{"bar": map[string]any{}}},
	).(map[string]any)
	// gabs represents preserved empty containers as struct{} sentinels.
	_, ok := got["foo.bar"]
	assert.True(t, ok, "expected foo.bar key to be preserved with include_empty: %v", got)
}

func TestBloblangV2KeyValues(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.key_values().sort_by(p -> p.key)`,
		map[string]any{"bar": int64(1), "baz": int64(2)},
	).([]any)
	assert.Equal(t, []any{
		map[string]any{"key": "bar", "value": int64(1)},
		map[string]any{"key": "baz", "value": int64(2)},
	}, got)
}

func TestBloblangV2FindBy(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.find_by(v -> v != "bar")`,
		[]any{"bar", "foo", "baz"},
	)
	assert.Equal(t, int64(1), got)
}

func TestBloblangV2FindByObjectPredicate(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.find_by(u -> u.age >= 18)`,
		[]any{
			map[string]any{"name": "Alice", "age": int64(15)},
			map[string]any{"name": "Bob", "age": int64(22)},
			map[string]any{"name": "Carol", "age": int64(19)},
		},
	)
	assert.Equal(t, int64(1), got)
}

func TestBloblangV2FindByNoMatch(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.find_by(v -> v == "missing")`,
		[]any{"a", "b", "c"},
	)
	assert.Equal(t, int64(-1), got)
}

func TestBloblangV2FindAllBy(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.find_all_by(log -> log.level == "error")`,
		[]any{
			map[string]any{"level": "info"},
			map[string]any{"level": "error"},
			map[string]any{"level": "warn"},
			map[string]any{"level": "error"},
		},
	)
	assert.Equal(t, []any{int64(1), int64(3)}, got)
}

func TestBloblangV2MapEachKey(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.map_each_key(k -> k.uppercase())`,
		map[string]any{"keya": "hello", "keyb": "world"},
	)
	assert.Equal(t, map[string]any{"KEYA": "hello", "KEYB": "world"}, got)
}

func TestBloblangV2MapEachKeyMustReturnString(t *testing.T) {
	exec, err := bloblangv2.GlobalEnvironment().Parse(`output = input.map_each_key(k -> 42)`)
	require.NoError(t, err)
	_, err = exec.Query(map[string]any{"a": "v"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "string")
}
