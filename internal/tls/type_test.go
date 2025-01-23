// Copyright 2025 Redpanda Data, Inc.

package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/youmark/pkcs8"

	"github.com/redpanda-data/benthos/v4/internal/filepath/ifs"
)

func createCertificates() (certPem, keyPem []byte) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}

	priv := x509.MarshalPKCS1PrivateKey(key)

	tml := x509.Certificate{
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(5, 0, 0),
		SerialNumber: big.NewInt(123123),
		Subject: pkix.Name{
			CommonName:   "Benthos",
			Organization: []string{"Benthos"},
		},
		BasicConstraintsValid: true,
	}

	cert, err := x509.CreateCertificate(rand.Reader, &tml, &tml, &key.PublicKey, key)
	if err != nil {
		log.Fatal("Certificate cannot be created.", err.Error())
	}

	certPem = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	})

	keyPem = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: priv})

	return certPem, keyPem
}

// CreateSignedCertificate generates a certificate signed by a CA.
func CreateSignedCertificate(caCert *x509.Certificate, caKey *rsa.PrivateKey, ipAddress string) (certPEM, keyPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	tml := x509.Certificate{
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(2, 0, 0),
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   "localhost",
			Organization: []string{"Benthos"},
		},
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP(ipAddress)},
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &tml, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	return certPEM, keyPEM, nil
}

// CreateCACertificate generates a CA certificate.
func CreateCACertificate() (caCertPEM, caKeyPEM []byte, caCert *x509.Certificate, caKey *rsa.PrivateKey, err error) {
	caKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Benthos CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // Valid for 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	caCertBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertBytes})
	caKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(caKey)})

	caCert, err = x509.ParseCertificate(caCertBytes)
	return caCertPEM, caKeyPEM, caCert, caKey, err
}

type keyPair struct {
	cert []byte
	key  []byte
}

func createCertificatesWithEncryptedPKCS1Key(t *testing.T, password string) keyPair {
	t.Helper()

	certPem, keyPem := createCertificates()
	decodedKey, _ := pem.Decode(keyPem)

	//nolint:staticcheck // SA1019 Disable linting for deprecated  x509.EncryptPEMBlock call
	block, err := x509.EncryptPEMBlock(rand.Reader, decodedKey.Type, decodedKey.Bytes, []byte(password), x509.PEMCipher3DES)
	require.NoError(t, err)

	keyPem = pem.EncodeToMemory(
		block,
	)
	return keyPair{cert: certPem, key: keyPem}
}

func createCertificatesWithEncryptedPKCS8Key(t *testing.T, password string) keyPair {
	t.Helper()

	certPem, keyPem := createCertificates()
	pemBlock, _ := pem.Decode(keyPem)
	decodedKey, err := x509.ParsePKCS1PrivateKey(pemBlock.Bytes)
	require.NoError(t, err)

	keyBytes, err := pkcs8.ConvertPrivateKeyToPKCS8(decodedKey, []byte(password))
	require.NoError(t, err)

	return keyPair{cert: certPem, key: pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: keyBytes})}
}

func TestCertificateFileWithEncryptedKey(t *testing.T) {
	tests := []struct {
		name string
		kp   keyPair
	}{
		{
			name: "PKCS#1",
			kp:   createCertificatesWithEncryptedPKCS1Key(t, "benthos"),
		},
		{
			name: "PKCS#8",
			kp:   createCertificatesWithEncryptedPKCS8Key(t, "benthos"),
		},
	}

	tmpDir := t.TempDir()
	for _, test := range tests {
		fCert, _ := os.CreateTemp(tmpDir, "cert.pem")
		_, _ = fCert.Write(test.kp.cert)
		fCert.Close()

		fKey, _ := os.CreateTemp(tmpDir, "key.pem")
		_, _ = fKey.Write(test.kp.key)
		fKey.Close()

		c := ClientCertConfig{
			KeyFile:  fKey.Name(),
			CertFile: fCert.Name(),
			Password: "benthos",
		}

		_, err := c.Load(ifs.OS())
		if err != nil {
			t.Errorf("Failed to load %s certificate: %s", test.name, err)
		}
	}
}

func TestCertificateWithEncryptedKey(t *testing.T) {
	tests := []struct {
		name string
		kp   keyPair
	}{
		{
			name: "PKCS#1",
			kp:   createCertificatesWithEncryptedPKCS1Key(t, "benthos"),
		},
		{
			name: "PKCS#8",
			kp:   createCertificatesWithEncryptedPKCS8Key(t, "benthos"),
		},
	}

	for _, test := range tests {
		c := ClientCertConfig{
			Cert:     string(test.kp.cert),
			Key:      string(test.kp.key),
			Password: "benthos",
		}

		_, err := c.Load(ifs.OS())
		if err != nil {
			t.Errorf("Failed to load %s certificate: %s", test.name, err)
		}
	}
}

func TestCertificateFileWithEncryptedKeyAndWrongPassword(t *testing.T) {
	tests := []struct {
		name string
		kp   keyPair
		err  string
	}{
		{
			name: "PKCS#1",
			kp:   createCertificatesWithEncryptedPKCS1Key(t, "benthos"),
			err:  "x509: decryption password incorrect",
		},
		{
			name: "PKCS#8",
			kp:   createCertificatesWithEncryptedPKCS8Key(t, "benthos"),
			err:  "pkcs8: incorrect password",
		},
	}

	tmpDir := t.TempDir()
	for _, test := range tests {
		fCert, _ := os.CreateTemp(tmpDir, "cert.pem")
		_, _ = fCert.Write(test.kp.cert)
		fCert.Close()

		fKey, _ := os.CreateTemp(tmpDir, "key.pem")
		_, _ = fKey.Write(test.kp.key)
		fKey.Close()

		c := ClientCertConfig{
			KeyFile:  fKey.Name(),
			CertFile: fCert.Name(),
			Password: "not_bentho",
		}

		_, err := c.Load(ifs.OS())
		require.ErrorContains(t, err, test.err, test.name)
	}
}

func TestEncryptedKeyWithWrongPassword(t *testing.T) {
	tests := []struct {
		name string
		kp   keyPair
		err  string
	}{
		{
			name: "PKCS#1",
			kp:   createCertificatesWithEncryptedPKCS1Key(t, "benthos"),
			err:  "x509: decryption password incorrect",
		},
		{
			name: "PKCS#8",
			kp:   createCertificatesWithEncryptedPKCS8Key(t, "benthos"),
			err:  "pkcs8: incorrect password",
		},
	}

	for _, test := range tests {
		c := ClientCertConfig{
			Cert:     string(test.kp.cert),
			Key:      string(test.kp.key),
			Password: "not_bentho",
		}

		_, err := c.Load(ifs.OS())
		require.ErrorContains(t, err, test.err, test.name)
	}
}

func TestCertificateFileWithNoEncryption(t *testing.T) {
	cert, key := createCertificates()

	tmpDir := t.TempDir()

	fCert, _ := os.CreateTemp(tmpDir, "cert.pem")
	_, _ = fCert.Write(cert)
	defer fCert.Close()

	fKey, _ := os.CreateTemp(tmpDir, "key.pem")
	_, _ = fKey.Write(key)
	defer fKey.Close()

	c := ClientCertConfig{
		KeyFile:  fKey.Name(),
		CertFile: fCert.Name(),
	}

	_, err := c.Load(ifs.OS())
	if err != nil {
		t.Errorf("Failed to load certificate %s", err)
	}
}

func TestCertificateWithNoEncryption(t *testing.T) {
	cert, key := createCertificates()

	c := ClientCertConfig{
		Key:  string(key),
		Cert: string(cert),
	}

	_, err := c.Load(ifs.OS())
	if err != nil {
		t.Errorf("Failed to load certificate %s", err)
	}
}

func TestRequireMutualTLS(t *testing.T) {
	// First create a test server so we can get its IP address for the certificate.
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	serverAddr := server.Listener.Addr().String()
	serverIP := strings.Split(serverAddr, ":")[0]

	// Generate CA certificate.
	caCertPem, _, caCert, caKey, err := CreateCACertificate()
	require.NoError(t, err)

	// Generate server certificate signed by the CA.
	serverCert, serverKey, err := CreateSignedCertificate(caCert, caKey, serverIP)
	require.NoError(t, err)

	// Setup the server configuration with the server certificate and the CA root.
	serverConfig := Config{
		Enabled:            true,
		ClientRootCAS:      string(caCertPem),
		RequireMutualTLS:   true,
		ClientCertificates: []ClientCertConfig{{Cert: string(serverCert), Key: string(serverKey)}},
	}

	// Get the server TLS configuration.
	serverTLSConfig, err := serverConfig.GetNonToggled(nil)
	require.NoError(t, err)

	// Set the test server's TLS configuration and start it.
	server.TLS = serverTLSConfig
	server.StartTLS()
	defer server.Close()

	// ---------------------------------------------------------------------------------------------------
	// Setup the client without a client certificate (to test rejection).
	clientConfigWithoutCert := Config{
		Enabled:            true,
		RootCAs:            string(caCertPem), // Use the CA certificate.
		ClientCertificates: []ClientCertConfig{},
	}

	// Get the client TLS configuration for the client without a certificate.
	clientWithoutTLSConfig, err := clientConfigWithoutCert.GetNonToggled(nil)
	require.NoError(t, err)

	// Create a client without a client certificate.
	clientWithoutCert := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: clientWithoutTLSConfig,
		},
	}

	// Attempt to connect without a client certificate.
	_, err = clientWithoutCert.Get(server.URL)
	require.Error(t, err, "Expected error when client does not provide a certificate")

	// ---------------------------------------------------------------------------------------------------
	// Setup the client with a client certificate

	// Generate client certificate signed by the CA.
	clientCert, clientKey, err := CreateSignedCertificate(caCert, caKey, serverIP)
	require.NoError(t, err)

	// Setup the client configuration with the client certificate and the same CA root.
	clientConfig := Config{
		Enabled:            true,
		RootCAs:            string(caCertPem),
		ClientCertificates: []ClientCertConfig{{Cert: string(clientCert), Key: string(clientKey)}},
	}

	// Get the client TLS configuration with client cert.
	clientTLSConfig, err := clientConfig.GetNonToggled(nil)
	require.NoError(t, err)

	// Test connection with a client certificate (should succeed).
	clientWithCert := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: clientTLSConfig,
		},
	}

	// Attempt to connect with a client certificate.
	resp, err := clientWithCert.Get(server.URL)
	require.NoError(t, err, "Expected no error when client provides a valid certificate")

	// Read and verify the response body.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "Hello, client\n", string(body))

	err = resp.Body.Close()
	require.NoError(t, err)
}
