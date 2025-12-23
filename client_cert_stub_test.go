package ftp

import "testing"

// TestClientCertificateSupport is a stub to document that client certificate
// support (mTLS) is verified in the server package integration tests.
//
// See: server/client_cert_test.go
func TestClientCertificateSupport(t *testing.T) {
	t.Log("Skipping: mTLS integration tests are located in server/client_cert_test.go to avoid circular dependencies")
}
