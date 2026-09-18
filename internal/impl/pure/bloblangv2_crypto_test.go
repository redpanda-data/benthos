// Copyright 2026 Redpanda Data, Inc.

package pure_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

// 16 bytes for AES-128, 16 bytes for IV (block size).
const (
	aesTestKey = "0123456789abcdef"
	aesTestIV  = "fedcba9876543210"
)

func TestBloblangV2AESCTRRoundTrip(t *testing.T) {
	enc := runBloblangV2(t,
		`output = input.encrypt_aes("ctr", "`+aesTestKey+`", "`+aesTestIV+`")`,
		"hello world",
	).([]byte)

	dec := runBloblangV2(t,
		`output = input.decrypt_aes("ctr", "`+aesTestKey+`", "`+aesTestIV+`").string()`,
		enc,
	)
	assert.Equal(t, "hello world", dec)
}

func TestBloblangV2AESGCMRoundTrip(t *testing.T) {
	// GCM uses a 12-byte nonce.
	const nonce = "0123456789ab"
	enc := runBloblangV2(t,
		`output = input.encrypt_aes("gcm", "`+aesTestKey+`", "`+nonce+`")`,
		"secret payload",
	).([]byte)

	dec := runBloblangV2(t,
		`output = input.decrypt_aes("gcm", "`+aesTestKey+`", "`+nonce+`").string()`,
		enc,
	)
	assert.Equal(t, "secret payload", dec)
}

func TestBloblangV2AESCBCRoundTrip(t *testing.T) {
	// CBC requires 16-byte aligned plaintext.
	const plain = "0123456789abcdef0123456789abcdef"
	enc := runBloblangV2(t,
		`output = input.encrypt_aes("cbc", "`+aesTestKey+`", "`+aesTestIV+`")`,
		plain,
	).([]byte)

	dec := runBloblangV2(t,
		`output = input.decrypt_aes("cbc", "`+aesTestKey+`", "`+aesTestIV+`").string()`,
		enc,
	)
	assert.Equal(t, plain, dec)
}

func TestBloblangV2AESUnknownScheme(t *testing.T) {
	// Static literal args mean V2 runs the constructor at parse time, so the
	// scheme validation surfaces as a parse error.
	_, err := bloblangv2.GlobalEnvironment().Parse(
		`output = input.encrypt_aes("does_not_exist", "` + aesTestKey + `", "` + aesTestIV + `")`,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unrecognized encryption scheme")
}
