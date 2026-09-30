// Copyright 2026 Redpanda Data, Inc.

package io_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"

	_ "github.com/redpanda-data/benthos/v4/public/components/io"
)

func TestBloblangV2KsuidShape(t *testing.T) {
	got := runBloblangV2(t, `output = ksuid()`, nil).(string)
	// KSUIDs are 27 base62 characters.
	if len(got) != 27 {
		t.Fatalf("ksuid length=%d, want 27 (got %q)", len(got), got)
	}
}

func TestBloblangV2KsuidIsRandom(t *testing.T) {
	a := runBloblangV2(t, `output = ksuid()`, nil).(string)
	b := runBloblangV2(t, `output = ksuid()`, nil).(string)
	assert.NotEqual(t, a, b)
}

func TestBloblangV2NanoidDefault(t *testing.T) {
	got := runBloblangV2(t, `output = nanoid()`, nil).(string)
	// Default Nano ID length is 21.
	if len(got) != 21 {
		t.Fatalf("nanoid length=%d, want 21 (got %q)", len(got), got)
	}
}

func TestBloblangV2NanoidCustomLength(t *testing.T) {
	got := runBloblangV2(t, `output = nanoid(10)`, nil).(string)
	assert.Len(t, got, 10)
}

func TestBloblangV2NanoidCustomAlphabet(t *testing.T) {
	got := runBloblangV2(t, `output = nanoid(8, "abc")`, nil).(string)
	assert.Len(t, got, 8)
	for _, r := range got {
		assert.Contains(t, "abc", string(r))
	}
}

func TestBloblangV2NanoidAlphabetWithoutLengthErrors(t *testing.T) {
	// Named args bypass V2's static-arg folding so the constructor runs at
	// query time. The validation error surfaces from Query, not Parse.
	exec, err := bloblangv2.GlobalEnvironment().Parse(`output = nanoid(alphabet: "abc")`)
	require.NoError(t, err)
	_, err = exec.Query(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "length")
}

func TestBloblangV2UUIDV7Shape(t *testing.T) {
	got := runBloblangV2(t, `output = uuid_v7()`, nil).(string)
	// Standard UUID string form is 36 characters.
	if len(got) != 36 {
		t.Fatalf("uuid_v7 length=%d, want 36 (got %q)", len(got), got)
	}
	// V7 variant char (the first nibble of the third group) is "7".
	if !strings.HasPrefix(strings.Split(got, "-")[2], "7") {
		t.Fatalf("uuid_v7 third group should start with '7': %q", got)
	}
}

func TestBloblangV2UUIDV7IsRandom(t *testing.T) {
	a := runBloblangV2(t, `output = uuid_v7()`, nil).(string)
	b := runBloblangV2(t, `output = uuid_v7()`, nil).(string)
	assert.NotEqual(t, a, b)
}

func TestBloblangV2UUIDV7WithTimestamp(t *testing.T) {
	// Pass a parsed timestamp to back-date the UUID. V2 ts_parse uses
	// strftime-style format strings, not Go's reference-time format.
	got := runBloblangV2(t,
		`output = uuid_v7("2024-01-01T00:00:00Z".ts_parse("%Y-%m-%dT%H:%M:%SZ"))`,
		nil,
	).(string)
	assert.Len(t, got, 36)
}
