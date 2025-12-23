package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

// generateCert creates a certificate and writes it to disk.
// If ca is nil, creates a self-signed CA root.
// If ca is provided, creates a certificate signed by the CA.
func generateCert(t *testing.T, isCA bool, caCert *x509.Certificate, caKey *rsa.PrivateKey) (string, string, *x509.Certificate, *rsa.PrivateKey) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	if isCA {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	// Hostname needs to match for server certs
	template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	template.DNSNames = []string{"localhost"}

	var parentCert *x509.Certificate
	var parentKey *rsa.PrivateKey

	if caCert == nil {
		parentCert = &template
		parentKey = priv
	} else {
		parentCert = caCert
		parentKey = caKey
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, parentCert, &priv.PublicKey, parentKey)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	// Write to temp files
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	certOut, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("Failed to open cert.pem for writing: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		t.Fatalf("Failed to write to cert.pem: %v", err)
	}
	certOut.Close()

	keyOut, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("Failed to open key.pem for writing: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		t.Fatalf("Failed to write to key.pem: %v", err)
	}
	keyOut.Close()

	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		t.Fatalf("Failed to parse generated cert: %v", err)
	}

	return certPath, keyPath, cert, priv
}

func TestClientCertificateAuthentication(t *testing.T) {
	// 1. Generate PKI Infrastructure
	_, _, caCert, caKey := generateCert(t, true, nil, nil)
	serverCertPath, serverKeyPath, _, _ := generateCert(t, false, caCert, caKey)
	clientCertPath, clientKeyPath, _, _ := generateCert(t, false, caCert, caKey)

	// Generate an untrusted client cert (self-signed, not signed by our CA)
	untrustedCertPath, untrustedKeyPath, _, _ := generateCert(t, false, nil, nil)

	// 2. Start Server with mTLS required
	rootDir := t.TempDir()
	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	serverTLSConfig, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		t.Fatal(err)
	}

	// Create a cert pool for the CA
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})) {
		t.Fatal("Failed to append CA cert")
	}

	server, err := NewServer(":0", // Random port
		WithDriver(driver),
		WithTLS(&tls.Config{
			Certificates: []tls.Certificate{serverTLSConfig},
			ClientCAs:    clientCAs,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	go func() {
		if err := server.Serve(ln); err != nil {
			t.Logf("server stopped: %v", err)
		}
	}()
	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// 3. Test Cases

	// Case A: Success - Valid Client Certificate
	t.Run("ValidClientCert", func(t *testing.T) {
		clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
		if err != nil {
			t.Fatalf("Failed to load client cert: %v", err)
		}

		c, err := ftp.Dial(addr,
			ftp.WithExplicitTLS(&tls.Config{
				InsecureSkipVerify: true, // Verification not focus of this test
				Certificates:       []tls.Certificate{clientCert},
			}),
		)
		if err != nil {
			t.Fatalf("Dial failed: %v", err)
		}
		defer func() {
			if err := c.Quit(); err != nil {
				t.Logf("Quit failed: %v", err)
			}
		}()

		if err := c.Login("anonymous", "anonymous"); err != nil {
			t.Fatalf("Login failed: %v", err)
		}

		// Verify we can do operations
		if _, err := c.CurrentDir(); err != nil {
			t.Errorf("CurrentDir failed: %v", err)
		}
	})

	// Case B: Failure - No Client Certificate
	t.Run("NoClientCert", func(t *testing.T) {
		_, err := ftp.Dial(addr,
			ftp.WithExplicitTLS(&tls.Config{
				InsecureSkipVerify: true,
			}),
			// No client cert provided
		)
		if err == nil {
			t.Error("Expected error connecting without client cert, got nil")
		} else {
			// Expected error: "tls: bad certificate" or handshake failure or EOF
			t.Logf("Got expected error: %v", err)
		}
	})

	// Case C: Failure - Untrusted Client Certificate
	t.Run("UntrustedClientCert", func(t *testing.T) {
		untrustedCert, err := tls.LoadX509KeyPair(untrustedCertPath, untrustedKeyPath)
		if err != nil {
			t.Fatalf("Failed to load untrusted cert: %v", err)
		}

		_, err = ftp.Dial(addr,
			ftp.WithExplicitTLS(&tls.Config{
				InsecureSkipVerify: true,
				Certificates:       []tls.Certificate{untrustedCert},
			}),
		)
		if err == nil {
			t.Error("Expected error connecting with untrusted client cert, got nil")
		} else {
			t.Logf("Got expected error: %v", err)
		}
	})
}
