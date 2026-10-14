// Copyright 2026 Redpanda Data, Inc.

package pure_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func TestBloblangV2ReReplace(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.re_replace("ADD ([0-9]+)", "+($1)")`,
		"foo ADD 70 ADD 1",
	)
	assert.Equal(t, "foo +(70) +(1)", got)
}

func TestBloblangV2ReFindObject(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.re_find_object("a(?P<foo>x*)b")`,
		"-axxb-ab-",
	)
	assert.Equal(t, map[string]any{"0": "axxb", "foo": "xx"}, got)
}

func TestBloblangV2ReFindAllObject(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.re_find_all_object("a(?P<foo>x*)b")`,
		"-axxb-ab-",
	)
	assert.Equal(t, []any{
		map[string]any{"0": "axxb", "foo": "xx"},
		map[string]any{"0": "ab", "foo": ""},
	}, got)
}

func TestBloblangV2ReFindAllSubmatch(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.re_find_all_submatch("a(x*)b")`,
		"-axxb-ab-",
	)
	assert.Equal(t, []any{
		[]any{"axxb", "xx"},
		[]any{"ab", ""},
	}, got)
}

func TestBloblangV2ReFindAllSubmatchEmpty(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.re_find_all_submatch("a(x*)b")`,
		"nothing matches here",
	)
	assert.Equal(t, []any{}, got)
}
