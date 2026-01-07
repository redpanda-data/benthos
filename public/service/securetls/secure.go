// Copyright 2026 Redpanda Data, Inc.

// Package securetls provides secure TLS configuration helpers that comply with
// Redpanda Transport Security Guidelines.
package securetls

import "crypto/tls"

// SecurityLevel defines the TLS security level, which determines
// the appropriate cipher suites according to Redpanda Transport Security Guidelines.
type SecurityLevel string

const (
	// SecurityLevelStrict is for internal Redpanda-to-Redpanda communication.
	// Uses TLS 1.3 only with a restricted set of cipher suites for maximum security.
	SecurityLevelStrict SecurityLevel = "strict"

	// SecurityLevelLax is for external Redpanda-to-customer communication.
	// Uses TLS 1.2+ with a broader set of cipher suites for compatibility.
	SecurityLevelLax SecurityLevel = "lax"
)

// NewConfig creates a *tls.Config that complies with Redpanda Transport
// Security Guidelines. It sets appropriate MinVersion and CipherSuites based on
// the security level.
//
// Example usage:
//
//	// For internal Redpanda-to-Redpanda communication (strict security)
//	tlsConf := securetls.NewConfig(securetls.SecurityLevelStrict)
//
//	// For external customer-facing services (lax for compatibility)
//	tlsConf := securetls.NewConfig(securetls.SecurityLevelLax)
//	tlsConf.Certificates = []tls.Certificate{cert}
func NewConfig(level SecurityLevel) *tls.Config {
	conf := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	switch level {
	case SecurityLevelStrict:
		conf.CipherSuites = getStrictCipherSuites()
	case SecurityLevelLax:
		conf.CipherSuites = getLaxCipherSuites()
	default:
		// Default to lax (more permissive) if security level is unspecified
		conf.CipherSuites = getLaxCipherSuites()
	}

	return conf
}

// getStrictCipherSuites returns cipher suites for strict security level
// (internal Redpanda-to-Redpanda communication) per Transport Security Guidelines.
//
// NOTE: This returns TLS 1.3 cipher suite constants for documentation purposes,
// but Go's crypto/tls ignores them - TLS 1.3 cipher suites are not configurable
// and are always enabled when TLS 1.3 is negotiated. By not including any TLS 1.2
// suites, this effectively enforces TLS 1.3 only (with Go's hardcoded secure suites).
//
// When TLS 1.3 is used, Go will automatically use these suites:
// - TLS_AES_256_GCM_SHA384
// - TLS_CHACHA20_POLY1305_SHA256
// - TLS_AES_128_GCM_SHA256
func getStrictCipherSuites() []uint16 {
	return []uint16{
		// TLS 1.3 suites (for documentation only - Go ignores these)
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
		tls.TLS_AES_128_GCM_SHA256,
		// No TLS 1.2 suites = TLS 1.3 only
	}
}

// getLaxCipherSuites returns cipher suites for lax security level
// (external Redpanda-to-customer communication) per Transport Security Guidelines.
//
// NOTE: TLS 1.3 cipher suites listed here are for documentation only - Go's
// crypto/tls ignores them as TLS 1.3 suites are not configurable. When TLS 1.3
// is negotiated, Go automatically uses its hardcoded secure suites. The TLS 1.2
// suites below are actually configured and enforced.
//
// Includes TLS 1.2 suites for backward compatibility with various client types.
func getLaxCipherSuites() []uint16 {
	return []uint16{
		// TLS 1.3 suites (for documentation only - Go ignores these)
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
		tls.TLS_AES_128_GCM_SHA256,

		// TLS 1.2 suites with ECDHE (actually configured - forward secrecy)
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,

		// Backward compatibility suites (disabled by default)
		// Uncomment only if required for specific legacy client support:
		// tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	}
}

// WithInsecureSkipVerify returns a secure config with InsecureSkipVerify enabled.
// This should only be used in testing or when certificate verification is handled
// through other means.
//
// WARNING: This disables certificate verification and should not be used in production
// without understanding the security implications.
func WithInsecureSkipVerify(level SecurityLevel) *tls.Config {
	conf := NewConfig(level)
	conf.InsecureSkipVerify = true
	return conf
}
