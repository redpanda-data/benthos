// Copyright 2026 Redpanda Data, Inc.

package pure_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func TestBloblangV2Array(t *testing.T) {
	got := runBloblangV2(t, `output = input.array()`, "hello")
	assert.Equal(t, []any{"hello"}, got)

	got = runBloblangV2(t, `output = input.array()`, []any{"already"})
	assert.Equal(t, []any{"already"}, got)
}

func TestBloblangV2Exists(t *testing.T) {
	doc := map[string]any{"foo": map[string]any{"bar": map[string]any{"baz": "yep"}}}
	got := runBloblangV2(t, `output = input.exists("foo.bar.baz")`, doc)
	assert.Equal(t, true, got)

	got = runBloblangV2(t, `output = input.exists("foo.bar.qux")`, doc)
	assert.Equal(t, false, got)
}

func TestBloblangV2ExistsTrueForNullValue(t *testing.T) {
	doc := map[string]any{"data": map[string]any{"optional": nil}}
	got := runBloblangV2(t, `output = input.exists("data.optional")`, doc)
	assert.Equal(t, true, got)
}

func TestBloblangV2Get(t *testing.T) {
	doc := map[string]any{"foo": map[string]any{"bar": "from bar"}}
	got := runBloblangV2(t, `output = input.get("foo.bar")`, doc)
	assert.Equal(t, "from bar", got)

	got = runBloblangV2(t, `output = input.get("foo.missing")`, doc)
	assert.Nil(t, got)
}

func TestBloblangV2ExplodeOnArray(t *testing.T) {
	doc := map[string]any{"id": int64(1), "value": []any{"foo", "bar", "baz"}}
	got := runBloblangV2(t, `output = input.explode("value")`, doc)
	assert.Equal(t, []any{
		map[string]any{"id": int64(1), "value": "foo"},
		map[string]any{"id": int64(1), "value": "bar"},
		map[string]any{"id": int64(1), "value": "baz"},
	}, got)
}

func TestBloblangV2ExplodeOnObject(t *testing.T) {
	doc := map[string]any{
		"id":    int64(1),
		"value": map[string]any{"foo": int64(2), "bar": int64(3)},
	}
	got := runBloblangV2(t, `output = input.explode("value")`, doc)
	expected := map[string]any{
		"foo": map[string]any{"id": int64(1), "value": int64(2)},
		"bar": map[string]any{"id": int64(1), "value": int64(3)},
	}
	assert.Equal(t, expected, got)
}

func TestBloblangV2Assign(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.assign({"likes": "foos", "second_name": "barer"})`,
		map[string]any{"first_name": "fooer", "likes": "bars"},
	)
	assert.Equal(t, map[string]any{
		"first_name":  "fooer",
		"likes":       "foos",
		"second_name": "barer",
	}, got)
}

func TestBloblangV2AssignArray(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.assign(["c", "d"])`,
		[]any{"a", "b"},
	)
	assert.Equal(t, []any{"a", "b", "c", "d"}, got)
}
