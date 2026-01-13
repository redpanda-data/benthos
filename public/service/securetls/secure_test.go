// Copyright 2025 Redpanda Data, Inc.

package securetls

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name               string
		level              SecurityLevel
		expectedMinVersion uint16
		expectedSuites     []uint16
	}{
		{
			name:               "strict security level",
			level:              SecurityLevelStrict,
			expectedMinVersion: tls.VersionTLS12,
			expectedSuites: []uint16{
				tls.TLS_AES_256_GCM_SHA384,
				tls.TLS_CHACHA20_POLY1305_SHA256,
				tls.TLS_AES_128_GCM_SHA256,
			},
		},
		{
			name:               "normal security level",
			level:              SecurityLevelNormal,
			expectedMinVersion: tls.VersionTLS12,
			expectedSuites: []uint16{
				tls.TLS_AES_256_GCM_SHA384,
				tls.TLS_CHACHA20_POLY1305_SHA256,
				tls.TLS_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			},
		},
		{
			name:               "unspecified security level defaults to normal",
			level:              SecurityLevel(""),
			expectedMinVersion: tls.VersionTLS12,
			expectedSuites: []uint16{
				tls.TLS_AES_256_GCM_SHA384,
				tls.TLS_CHACHA20_POLY1305_SHA256,
				tls.TLS_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := NewConfig(tt.level)
			require.NotNil(t, conf)

			assert.Equal(t, tt.expectedMinVersion, conf.MinVersion,
				"MinVersion should be TLS 1.2")

			assert.Equal(t, tt.expectedSuites, conf.CipherSuites,
				"CipherSuites should match expected list")

			assert.False(t, conf.InsecureSkipVerify,
				"InsecureSkipVerify should be false by default")
		})
	}
}

func TestWithInsecureSkipVerify(t *testing.T) {
	tests := []struct {
		name  string
		level SecurityLevel
	}{
		{
			name:  "strict with skip verify",
			level: SecurityLevelStrict,
		},
		{
			name:  "normal with skip verify",
			level: SecurityLevelNormal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := WithInsecureSkipVerify(tt.level)
			require.NotNil(t, conf)

			assert.True(t, conf.InsecureSkipVerify,
				"InsecureSkipVerify should be true")

			assert.Equal(t, uint16(tls.VersionTLS12), conf.MinVersion,
				"MinVersion should still be TLS 1.2")

			assert.NotEmpty(t, conf.CipherSuites,
				"CipherSuites should still be configured")
		})
	}
}

func TestCipherSuiteCompliance(t *testing.T) {
	t.Run("strict suites are TLS 1.3 only", func(t *testing.T) {
		suites := getStrictCipherSuites()
		require.NotEmpty(t, suites)

		// All strict suites should be TLS 1.3 suites
		tls13Suites := map[uint16]bool{
			tls.TLS_AES_256_GCM_SHA384:       true,
			tls.TLS_CHACHA20_POLY1305_SHA256: true,
			tls.TLS_AES_128_GCM_SHA256:       true,
		}

		for _, suite := range suites {
			assert.True(t, tls13Suites[suite],
				"Strict suite 0x%04X should be a TLS 1.3 suite", suite)
		}
	})

	t.Run("normal suites include TLS 1.2 compatibility", func(t *testing.T) {
		suites := getNormalCipherSuites()
		require.NotEmpty(t, suites)

		// Should include at least one TLS 1.2 suite for compatibility
		hasTLS12Suite := false
		for _, suite := range suites {
			if suite == tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384 ||
				suite == tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384 {
				hasTLS12Suite = true
				break
			}
		}

		assert.True(t, hasTLS12Suite,
			"Normal suites should include TLS 1.2 compatibility")
	})

	t.Run("no weak cipher suites", func(t *testing.T) {
		// Test both strict and lax
		allSuites := append(getStrictCipherSuites(), getNormalCipherSuites()...)

		// Weak suites that should NOT be present
		weakSuites := map[uint16]string{
			tls.TLS_RSA_WITH_RC4_128_SHA:             "RC4 (broken)",
			tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA:        "3DES (weak)",
			tls.TLS_RSA_WITH_AES_128_CBC_SHA:         "CBC mode without ECDHE",
			tls.TLS_RSA_WITH_AES_256_CBC_SHA:         "CBC mode without ECDHE",
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA:   "CBC mode (deprecated)",
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA:   "CBC mode (deprecated)",
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA: "CBC mode (deprecated)",
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA: "CBC mode (deprecated)",
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256:      "RSA without forward secrecy",
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384:      "RSA without forward secrecy",
		}

		for _, suite := range allSuites {
			if reason, isWeak := weakSuites[suite]; isWeak {
				t.Errorf("Weak cipher suite 0x%04X found: %s", suite, reason)
			}
		}
	})

	t.Run("all suites use AEAD or are TLS 1.3", func(t *testing.T) {
		allSuites := append(getStrictCipherSuites(), getNormalCipherSuites()...)

		// AEAD suites (GCM, CHACHA20-POLY1305) or TLS 1.3
		aeadSuites := map[uint16]bool{
			// TLS 1.3
			tls.TLS_AES_256_GCM_SHA384:       true,
			tls.TLS_CHACHA20_POLY1305_SHA256: true,
			tls.TLS_AES_128_GCM_SHA256:       true,
			// TLS 1.2 with AEAD
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:         true,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384:       true,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:         true,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256:       true,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256:   true,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256: true,
		}

		for _, suite := range allSuites {
			assert.True(t, aeadSuites[suite],
				"Suite 0x%04X should use AEAD or be TLS 1.3", suite)
		}
	})
}

func TestMinVersionCompliance(t *testing.T) {
	t.Run("all configs enforce TLS 1.2 minimum", func(t *testing.T) {
		levels := []SecurityLevel{SecurityLevelStrict, SecurityLevelNormal, SecurityLevel("")}

		for _, level := range levels {
			conf := NewConfig(level)
			assert.GreaterOrEqual(t, conf.MinVersion, uint16(tls.VersionTLS12),
				"MinVersion should be at least TLS 1.2 for security level: %s", level)
		}
	})

	t.Run("insecure skip verify still enforces TLS 1.2", func(t *testing.T) {
		conf := WithInsecureSkipVerify(SecurityLevelNormal)
		assert.Equal(t, uint16(tls.VersionTLS12), conf.MinVersion,
			"Even with InsecureSkipVerify, MinVersion should be TLS 1.2")
	})
}

func TestConfigModifiability(t *testing.T) {
	t.Run("config can be modified after creation", func(t *testing.T) {
		conf := NewConfig(SecurityLevelNormal)

		// Should be able to add certificates
		conf.Certificates = []tls.Certificate{{}}
		assert.Len(t, conf.Certificates, 1)

		// Should be able to modify InsecureSkipVerify
		conf.InsecureSkipVerify = true
		assert.True(t, conf.InsecureSkipVerify)

		// Core security settings should still be intact
		assert.Equal(t, uint16(tls.VersionTLS12), conf.MinVersion)
		assert.NotEmpty(t, conf.CipherSuites)
	})
}
