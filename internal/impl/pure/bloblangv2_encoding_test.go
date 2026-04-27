// Copyright 2026 Redpanda Data, Inc.

package pure_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func TestBloblangV2HashSHA256(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.hash("sha256").encode("hex")`,
		"hello world",
	)
	assert.Equal(t, "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9", got)
}

func TestBloblangV2HashHMACSHA1(t *testing.T) {
	got := runBloblangV2(t,
		`output = input.hash("hmac_sha1", "static-key").encode("hex")`,
		"hello world",
	)
	assert.Equal(t, "d87e5f068fa08fe90bb95bc7c8344cb809179d76", got)
}

func TestBloblangV2HashHMACRequiresKey(t *testing.T) {
	// Static args trigger parse-time construction; HMAC missing a key
	// surfaces as a parse error.
	_, err := bloblangv2.GlobalEnvironment().Parse(`output = input.hash("hmac_sha256")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key")
}

func TestBloblangV2HashUnknownAlgorithm(t *testing.T) {
	// V2 caches the constructor for static literal args at parse time, so an
	// unknown algorithm surfaces as a parse error rather than a runtime one.
	_, err := bloblangv2.GlobalEnvironment().Parse(`output = input.hash("does_not_exist")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unrecognized hash type")
}

func TestBloblangV2UUIDV5Deterministic(t *testing.T) {
	// Same name + namespace must produce the same UUID twice in a row.
	exec, err := bloblangv2.GlobalEnvironment().Parse(`output = input.uuid_v5("dns")`)
	require.NoError(t, err)
	a, err := exec.Query("example.com")
	require.NoError(t, err)
	b, err := exec.Query("example.com")
	require.NoError(t, err)
	assert.Equal(t, a, b)

	// Different namespace must change the result.
	exec2, err := bloblangv2.GlobalEnvironment().Parse(`output = input.uuid_v5("url")`)
	require.NoError(t, err)
	c, err := exec2.Query("example.com")
	require.NoError(t, err)
	assert.NotEqual(t, a, c)
}

func TestBloblangV2UUIDV5DefaultNamespace(t *testing.T) {
	// With the default empty namespace the result is the nil-namespace UUID.
	got := runBloblangV2(t, `output = input.uuid_v5()`, "example")
	assert.Equal(t, "feb54431-301b-52bb-a6dd-e1e93e81bb9e", got)
}
