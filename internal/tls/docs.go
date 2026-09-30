// Copyright 2025 Redpanda Data, Inc.

package tls

import "github.com/redpanda-data/benthos/v4/internal/docs"

const (
	tlsSummary = "Configure Transport Layer Security (TLS) settings to secure network connections. This includes options for standard TLS as well as mutual TLS (mTLS) authentication where both client and server authenticate each other using certificates."

	clientCertsSummary = "A list of client certificates for mutual TLS (mTLS) authentication. Configure this field to enable mTLS, authenticating the client to the server with these certificates."

	clientCertsPairing = "**Certificate pairing rules**: For each certificate item, provide either:\n\n" +
		"- Inline PEM data using both `cert` *and* `key` or\n" +
		"- File paths using both `cert_file` *and* `key_file`.\n\n" +
		"Mixing inline and file-based values within the same item is not supported."
)

// FieldSpec returns a spec for a common TLS field, including an `enabled`
// field that switches the custom settings on.
func FieldSpec() docs.FieldSpec {
	return fieldSpec(true)
}

// NonToggledFieldSpec returns a spec for a TLS field without an `enabled`
// field, for components where the settings always apply.
func NonToggledFieldSpec() docs.FieldSpec {
	return fieldSpec(false)
}

func fieldSpec(toggled bool) docs.FieldSpec {
	tlsDesc := tlsSummary + " Key configuration options include `client_certs` for mTLS authentication, `root_cas`/`root_cas_file` for custom certificate authorities, and `skip_cert_verify` for development environments."
	clientCertsDesc := clientCertsSummary + "\n\n" + clientCertsPairing
	if toggled {
		tlsDesc = tlsSummary + " Key configuration options include `enabled` to enable TLS, `client_certs` for mTLS authentication, `root_cas`/`root_cas_file` for custom certificate authorities, and `skip_cert_verify` for development environments."
		clientCertsDesc = clientCertsSummary + "\n\nYou must set `tls.enabled: true` for the client certificates to take effect.\n\n" + clientCertsPairing
	}

	var children []docs.FieldSpec
	if toggled {
		children = append(children, docs.FieldBool(
			"enabled", "Whether to enable TLS for secure connections. Set to `true` to enable TLS encryption. Required to be `true` for other TLS options (like `client_certs`, `root_cas`, etc.) to take effect.",
		).HasDefault(false))
	}
	children = append(children,
		docs.FieldBool(
			"skip_cert_verify", "Whether to skip server-side certificate verification. Set to `true` only for testing environments as this reduces security by disabling certificate validation. When using self-signed certificates or in development, this may be necessary, but should never be used in production. Consider using `root_cas` or `root_cas_file` to specify trusted certificates instead of disabling verification entirely.",
		).HasDefault(false),

		docs.FieldBool(
			"enable_renegotiation", "Whether to allow the remote server to repeatedly request renegotiation. Enable this option if you're seeing the error message `local error: tls: no renegotiation`.",
		).AtVersion("3.45.0").Advanced().HasDefault(false),

		docs.FieldString(
			"root_cas", "Specify a root certificate authority to use (optional). This is a string that represents a certificate chain from the parent-trusted root certificate, through possible intermediate signing certificates, to the host certificate. Use either this field for inline certificate data or `root_cas_file` for file-based certificate loading.", "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
		).HasDefault("").Secret(),

		docs.FieldString(
			"root_cas_file", "Specify the path to a root certificate authority file (optional). This is a file, often with a `.pem` extension, which contains a certificate chain from the parent-trusted root certificate, through possible intermediate signing certificates, to the host certificate. Use either this field for file-based certificate loading or `root_cas` for inline certificate data.", "./root_cas.pem",
		).HasDefault(""),

		docs.FieldObject(
			"client_certs", clientCertsDesc,
			[]any{
				map[string]any{
					"cert": "foo",
					"key":  "bar",
				},
			},
			[]any{
				map[string]any{
					"cert_file": "./example.pem",
					"key_file":  "./example.key",
				},
			},
		).Array().WithChildren(
			docs.FieldString("cert", "The plaintext certificate to use for TLS authentication. Must be paired with the corresponding private key in the `key` field when using inline PEM data for mTLS client certificates.").HasDefault(""),
			docs.FieldString("key", "Private key for mTLS client certificate as inline PEM data. Must correspond to the client certificate specified in the `cert` field. Use this field together with `cert` when providing certificate data inline rather than through files.").HasDefault("").Secret(),
			docs.FieldString("cert_file", "The path to a file containing the certificate to use for TLS authentication. Must be paired with the corresponding private key file in the `key_file` field when using file-based configuration for mTLS client certificates.").HasDefault(""),
			docs.FieldString("key_file", "Path to private key file for mTLS client certificate in PEM format. Must correspond to the client certificate specified in the `cert_file` field. Use this field together with `cert_file` when loading certificate data from files.").HasDefault(""),
			docs.FieldString("password", `The password to use for the private key (specified in the `+"`key`"+` or `+"`key_file`"+` fields), if it is password-protected. The PKCS#1 and PKCS#8 formats are supported. Supports environment variable interpolation for secure password management.

The `+"`pbeWithMD5AndDES-CBC`"+` algorithm is obsolete and not supported for the PKCS#8 format. This algorithm does not authenticate the ciphertext, making it vulnerable to padding oracle attacks that can let an attacker recover the plaintext.`, "foo", "${KEY_PASSWORD}").HasDefault("").Secret().AtVersion("4.3.0"),
		).HasDefault([]any{}),
	)

	return docs.FieldObject("tls", tlsDesc).WithChildren(children...).Advanced()
}
