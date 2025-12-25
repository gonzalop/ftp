package ftp_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
	"github.com/gonzalop/ftp/server"
)

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *safeBuffer) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.buf.Bytes()...)
}

// setupServer starts a local FTP server for testing.
// Returns the server address, a cleanup function, and the root directory path.
func setupServer(t *testing.T) (string, func(), string) {
	// 1. Setup temporary directory for server root
	rootDir := t.TempDir()

	// 2. Start Server
	driver, err := server.NewFSDriver(rootDir,
		server.WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil // Allow write access in rootDir
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Use a random port
	s, err := server.NewServer("127.0.0.1:0", server.WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	// Run server in goroutine
	listener, err := SystemListener()
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		if err := s.Serve(listener); err != nil && err != server.ErrServerClosed {
			t.Logf("Server stopped: %v", err)
		}
	}()

	addr := listener.Addr().String()

	cleanup := func() {
		// Create a context with timeout for shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}

	return addr, cleanup, rootDir
}

// SystemListener creates a listener on a random port
func SystemListener() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}

// generateCert creates a certificate and writes it to disk.
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

func TestClient_ExplicitTLS(t *testing.T) {
	// 1. Generate Server Cert
	serverCertPath, serverKeyPath, _, _ := generateCert(t, false, nil, nil)
	serverTLSConfig, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Start Server with TLS support
	rootDir := t.TempDir()
	driver, err := server.NewFSDriver(rootDir,
		server.WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil // Allow write access
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	s, err := server.NewServer("127.0.0.1:0",
		server.WithDriver(driver),
		server.WithTLS(&tls.Config{
			Certificates: []tls.Certificate{serverTLSConfig},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := SystemListener()
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		if err := s.Serve(listener); err != nil && err != server.ErrServerClosed {
			t.Logf("Server stopped: %v", err)
		}
	}()

	addr := listener.Addr().String()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// 3. Connect with Client using Explicit TLS
	c, err := ftp.Dial(addr,
		ftp.WithTimeout(5*time.Second),
		ftp.WithExplicitTLS(&tls.Config{
			InsecureSkipVerify: true, // Self-signed cert
		}),
	)
	if err != nil {
		t.Fatalf("Failed to dial with Explicit TLS: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	// 4. Authenticate
	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// 5. Test Operations (should be encrypted)
	if _, err := c.CurrentDir(); err != nil {
		t.Errorf("CurrentDir failed over TLS: %v", err)
	}
}

func TestClient_ImplicitTLS(t *testing.T) {
	// 1. Generate Server Cert
	serverCertPath, serverKeyPath, _, _ := generateCert(t, false, nil, nil)
	serverTLSConfig, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Start Server with TLS support
	rootDir := t.TempDir()
	driver, err := server.NewFSDriver(rootDir,
		server.WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil // Allow write access
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	s, err := server.NewServer("127.0.0.1:0",
		server.WithDriver(driver),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Create a TCP listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	// Wrap in TLS listener for implicit TLS
	tlsListener := tls.NewListener(ln, &tls.Config{
		Certificates: []tls.Certificate{serverTLSConfig},
	})

	go func() {
		if err := s.Serve(tlsListener); err != nil && err != server.ErrServerClosed {
			t.Logf("Server stopped: %v", err)
		}
	}()

	addr := tlsListener.Addr().String()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// 3. Connect with Client using Implicit TLS
	c, err := ftp.Dial(addr,
		ftp.WithTimeout(5*time.Second),
		ftp.WithImplicitTLS(&tls.Config{
			InsecureSkipVerify: true, // Self-signed cert
		}),
	)
	if err != nil {
		t.Fatalf("Failed to dial with Implicit TLS: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	// 4. Authenticate
	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// 5. Test Operations
	if _, err := c.CurrentDir(); err != nil {
		t.Errorf("CurrentDir failed over Implicit TLS: %v", err)
	}
}

func TestClient_KeepAlive(t *testing.T) {
	addr, cleanup, _ := setupServer(t)
	defer cleanup()

	// Capture logs to verify keep-alive activity
	var logBuf safeBuffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	idleTimeout := 100 * time.Millisecond

	// Connect with Client using IdleTimeout and Logger
	c, err := ftp.Dial(addr,
		ftp.WithTimeout(5*time.Second),
		ftp.WithIdleTimeout(idleTimeout),
		ftp.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Wait long enough for the keep-alive ticker to fire (ticker is idleTimeout/2)
	// and for the idle check (time.Since(last) >= idleTimeout) to pass.
	// Last command was Login.
	time.Sleep(idleTimeout*2 + 50*time.Millisecond)

	// Verify logs contain "sending keep-alive NOOP"
	if !bytes.Contains(logBuf.Bytes(), []byte("sending keep-alive NOOP")) {
		t.Errorf("Expected keep-alive NOOP log, got:\n%s", logBuf.String())
	}
}

func TestClient_ActiveMode(t *testing.T) {
	addr, cleanup, _ := setupServer(t)
	defer cleanup()

	// Connect with Client using Active Mode
	c, err := ftp.Dial(addr,
		ftp.WithTimeout(5*time.Second),
		ftp.WithActiveMode(),
	)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Performing an operation that requires a data connection (should use PORT)
	_, err = c.List(".")
	if err != nil {
		t.Errorf("List in Active Mode failed: %v", err)
	}
}

func TestClient_ActiveModeIPv6(t *testing.T) {
	// Try to create an IPv6 listener for the server
	l, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 not supported or disabled:", err)
	}

	// 1. Setup temporary directory for server root
	rootDir := t.TempDir()

	// 2. Start Server
	driver, err := server.NewFSDriver(rootDir,
		server.WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil // Allow write access in rootDir
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	s, err := server.NewServer(l.Addr().String(), server.WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		if err := s.Serve(l); err != nil && err != server.ErrServerClosed {
			t.Logf("Server stopped: %v", err)
		}
	}()

	addr := l.Addr().String()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Connect with Client using Active Mode to the IPv6 address
	c, err := ftp.Dial(addr,
		ftp.WithTimeout(5*time.Second),
		ftp.WithActiveMode(),
	)
	if err != nil {
		t.Fatalf("Failed to dial IPv6: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Performing an operation that requires a data connection
	// Since we are connected via IPv6, openActiveDataConn should detect it and use EPRT
	_, err = c.List(".")
	if err != nil {
		t.Errorf("List in Active Mode (IPv6/EPRT) failed: %v", err)
	}
}

func TestClient_PASV(t *testing.T) {
	addr, cleanup, _ := setupServer(t)
	defer cleanup()

	// Connect with Client using DisableEPSV to force PASV
	c, err := ftp.Dial(addr,
		ftp.WithTimeout(5*time.Second),
		ftp.WithDisableEPSV(),
	)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Create a file so List has something to return
	if err := c.Store("pasv_test.txt", bytes.NewBufferString("test")); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Performing an operation that requires a data connection (should use PASV)
	entries, err := c.List(".")
	if err != nil {
		t.Errorf("List in PASV Mode failed: %v", err)
	}
	// Check we got the file
	if len(entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(entries))
	} else if entries[0].Name != "pasv_test.txt" {
		t.Errorf("Expected pasv_test.txt, got %s", entries[0].Name)
	}
}

func TestClient_Integration(t *testing.T) {
	addr, cleanup, rootDir := setupServer(t)
	defer cleanup()

	// Connect with Client
	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	// Authenticate
	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// 1. Test Noop
	if err := c.Noop(); err != nil {
		t.Errorf("Noop failed: %v", err)
	}

	// 2. Test CurrentDir and ChangeDir
	pwd, err := c.CurrentDir()
	if err != nil {
		t.Errorf("CurrentDir failed: %v", err)
	}
	if pwd != "/" {
		t.Errorf("Expected /, got %s", pwd)
	}

	// Create a subdirectory using OS functions
	subDir := filepath.Join(rootDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := c.ChangeDir("subdir"); err != nil {
		t.Errorf("ChangeDir failed: %v", err)
	}

	pwd, err = c.CurrentDir()
	if err != nil {
		t.Errorf("CurrentDir failed: %v", err)
	}
	if pwd != "/subdir" && pwd != "subdir" {
		t.Errorf("Expected /subdir, got %s", pwd)
	}

	if err := c.ChangeDir(".."); err != nil {
		t.Errorf("ChangeDir .. failed: %v", err)
	}

	// 3. Test MakeDir
	if err := c.MakeDir("newdir"); err != nil {
		t.Errorf("MakeDir failed: %v", err)
	}

	// Verify with OS
	if _, err := os.Stat(filepath.Join(rootDir, "newdir")); os.IsNotExist(err) {
		t.Errorf("MakeDir didn't create directory on disk")
	}

	// 4. Test Rename
	// Create file to rename
	if err := os.WriteFile(filepath.Join(rootDir, "old.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := c.Rename("old.txt", "new.txt"); err != nil {
		t.Errorf("Rename failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootDir, "old.txt")); !os.IsNotExist(err) {
		t.Error("old.txt still exists")
	}
	if _, err := os.Stat(filepath.Join(rootDir, "new.txt")); os.IsNotExist(err) {
		t.Error("new.txt does not exist")
	}

	// 5. Test Delete
	if err := c.Delete("new.txt"); err != nil {
		t.Errorf("Delete failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "new.txt")); !os.IsNotExist(err) {
		t.Error("new.txt still exists after delete")
	}

	// 6. Test RemoveDir
	if err := c.RemoveDir("newdir"); err != nil {
		t.Errorf("RemoveDir failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "newdir")); !os.IsNotExist(err) {
		t.Error("newdir still exists after RemoveDir")
	}

	// 7. Test List and NameList
	// Create some files
	if err := os.WriteFile(filepath.Join(rootDir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "b.txt"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := c.List(".")
	if err != nil {
		t.Errorf("List failed: %v", err)
	}
	if len(entries) < 2 {
		t.Errorf("List returned too few entries: %d", len(entries))
	}

	names, err := c.NameList(".")
	if err != nil {
		t.Errorf("NameList failed: %v", err)
	}
	if len(names) < 2 {
		t.Errorf("NameList returned too few entries: %d", len(names))
	}

	// Check if a.txt and b.txt are in names
	hasA, hasB := false, false
	for _, n := range names {
		if n == "a.txt" {
			hasA = true
		}
		if n == "b.txt" {
			hasB = true
		}
	}
	if !hasA || !hasB {
		t.Errorf("NameList missing files: %v", names)
	}

	// 8. Test Size (if supported)
	size, err := c.Size("a.txt")
	if err != nil {
		t.Logf("Size failed (expected if not implemented): %v", err)
	} else if size != 1 {
		t.Errorf("Expected size 1, got %d", size)
	}

	// 9. Test ModTime (MDTM)
	tTime, err := c.ModTime("a.txt")
	if err != nil {
		t.Logf("ModTime failed (expected if not implemented): %v", err)
	} else if tTime.IsZero() {
		t.Error("ModTime returned zero time")
	}

	// 10. Test Quote (custom command)
	// Send a NOOP via Quote
	if _, err := c.Quote("NOOP"); err != nil {
		t.Errorf("Quote NOOP failed: %v", err)
	}

	// 11. Test Append
	if err := c.Append("a.txt", bytes.NewBufferString(" appended")); err != nil {
		t.Errorf("Append failed: %v", err)
	}

	// Verify content
	content, err := os.ReadFile(filepath.Join(rootDir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "a appended" {
		t.Errorf("Append content mismatch. Got %q, want %q", string(content), "a appended")
	}

	// 12. Test Features
	feats, err := c.Features()
	if err != nil {
		t.Logf("Features command returned error (might be unsupported): %v", err)
	} else {
		if _, ok := feats["SIZE"]; !ok {
			t.Log("SIZE feature not advertised")
		}
	}

	// 13. Test HasFeature
	if c.HasFeature("NONEXISTENTFEATURE") {
		t.Error("HasFeature returned true for NONEXISTENTFEATURE")
	}

	// 14. Test SetOption (OPTS)
	if err := c.SetOption("UTF8", "ON"); err != nil {
		t.Logf("SetOption failed (expected if not implemented): %v", err)
	}

	// 15. Test Hash and SetHashAlgo
	if err := c.SetHashAlgo("SHA-256"); err != nil {
		t.Errorf("SetHashAlgo failed: %v", err)
	}

	hash, err := c.Hash("a.txt")
	if err != nil {
		t.Errorf("Hash failed: %v", err)
	}
	if hash == "" {
		t.Error("Hash returned empty string")
	}

	// 16. Test ProgressReader
	data := []byte("progress data")
	var totalRead int64
	pr := &ftp.ProgressReader{
		Reader: bytes.NewReader(data),
		Callback: func(read int64) {
			totalRead = read
		},
	}
	if err := c.Store("progress.txt", pr); err != nil {
		t.Errorf("Store with ProgressReader failed: %v", err)
	}
	if totalRead != int64(len(data)) {
		t.Errorf("ProgressReader total read mismatch: got %d, want %d", totalRead, len(data))
	}

	// 17. Test ProgressWriter
	var buf bytes.Buffer
	var totalWritten int64
	pw := &ftp.ProgressWriter{
		Writer: &buf,
		Callback: func(written int64) {
			totalWritten = written
		},
	}

	if err := c.Retrieve("progress.txt", pw); err != nil {
		t.Errorf("Retrieve with ProgressWriter failed: %v", err)
	}

	if totalWritten != int64(len(data)) {
		t.Errorf("ProgressWriter total written mismatch: got %d, want %d", totalWritten, len(data))
	}
	if buf.String() != string(data) {
		t.Errorf("Retrieved content mismatch: got %q, want %q", buf.String(), string(data))
	}

	// 18. Test MLStat
	entry, err := c.MLStat("progress.txt")
	if err != nil {
		t.Errorf("MLStat failed: %v", err)
	} else {
		if entry.Name != "progress.txt" {
			t.Errorf("MLStat Name = %s, want progress.txt", entry.Name)
		}
		if entry.Type != "file" {
			t.Errorf("MLStat Type = %s, want file", entry.Type)
		}
		if entry.Size != int64(len(data)) {
			t.Errorf("MLStat Size = %d, want %d", entry.Size, len(data))
		}
	}

	// 19. Test MLList
	mlEntries, err := c.MLList(".")
	if err != nil {
		t.Errorf("MLList failed: %v", err)
	} else {
		found := false
		for _, e := range mlEntries {
			if e.Name == "progress.txt" {
				found = true
				if e.Size != int64(len(data)) {
					t.Errorf("MLList entry size = %d, want %d", e.Size, len(data))
				}
				break
			}
		}
		if !found {
			t.Error("MLList did not find progress.txt")
		}
	}

	// 20. Test StoreFrom
	localUploadPath := filepath.Join(rootDir, "local_upload.txt")
	uploadContent := "StoreFrom test content"
	if err := os.WriteFile(localUploadPath, []byte(uploadContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := c.StoreFrom("stored_from.txt", localUploadPath); err != nil {
		t.Errorf("StoreFrom failed: %v", err)
	}

	// Verify on server
	serverStoredContent, err := os.ReadFile(filepath.Join(rootDir, "stored_from.txt"))
	if err != nil {
		t.Fatalf("Could not read stored file on server: %v", err)
	}
	if string(serverStoredContent) != uploadContent {
		t.Errorf("StoreFrom content mismatch: got %q, want %q", string(serverStoredContent), uploadContent)
	}

	// 21. Test RetrieveTo
	localDownloadPath := filepath.Join(rootDir, "local_download.txt")
	if err := c.RetrieveTo("stored_from.txt", localDownloadPath); err != nil {
		t.Errorf("RetrieveTo failed: %v", err)
	}

	downloadedContent, err := os.ReadFile(localDownloadPath)
	if err != nil {
		t.Fatalf("Could not read downloaded file: %v", err)
	}
	if string(downloadedContent) != uploadContent {
		t.Errorf("RetrieveTo content mismatch: got %q, want %q", string(downloadedContent), uploadContent)
	}

	// 22. Test RetrieveFrom (Resume Download / REST + RETR)
	// We'll download the last part of "stored_from.txt" which has "StoreFrom test content"
	// Let's skip the first 10 bytes: "StoreFrom "
	offset := int64(10)
	expectedPartial := uploadContent[offset:]
	var partialBuf bytes.Buffer

	if err := c.RetrieveFrom("stored_from.txt", &partialBuf, offset); err != nil {
		t.Errorf("RetrieveFrom failed: %v", err)
	}
	if partialBuf.String() != expectedPartial {
		t.Errorf("RetrieveFrom content mismatch: got %q, want %q", partialBuf.String(), expectedPartial)
	}

	// 23. Test StoreAt (Resume Upload / REST + STOR/APPE)
	// We'll append to a new file using StoreAt
	// First create a file with some content
	initialContent := "Initial "
	if err := c.Store("resume_upload.txt", bytes.NewBufferString(initialContent)); err != nil {
		t.Fatal(err)
	}

	// Now append using StoreAt with offset
	// Note: StoreAt with offset > 0 uses APPE which might not strictly use REST depending on implementation,
	// but the client implementation checks offset > 0.
	// Actually client.StoreAt uses APPE if offset > 0.
	// Let's verify it works.
	appendContent := "Appended"
	if err := c.StoreAt("resume_upload.txt", bytes.NewBufferString(appendContent), int64(len(initialContent))); err != nil {
		t.Errorf("StoreAt failed: %v", err)
	}

	// Verify full content
	fullContent, err := os.ReadFile(filepath.Join(rootDir, "resume_upload.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(fullContent) != initialContent+appendContent {
		t.Errorf("StoreAt content mismatch: got %q, want %q", string(fullContent), initialContent+appendContent)
	}

	// 24. Test RestartAt directly
	// Send REST command then RETR manually (via Retrieve but setting offset first)
	// Note: Client.Retrieve doesn't send REST. We have to use RestartAt then Retrieve?
	// But Retrieve resets state?
	// Looking at client code, Retrieve just sends RETR.
	// So:
	if err := c.RestartAt(5); err != nil {
		t.Errorf("RestartAt failed: %v", err)
	}
	// Verify it affects the next command?
	// The client library doesn't strictly track the state between calls unless we look at the implementation.
	// But the server should respect it if we send RETR next.
	var restBuf bytes.Buffer
	if err := c.Retrieve("stored_from.txt", &restBuf); err != nil {
		t.Errorf("Retrieve after RestartAt failed: %v", err)
	}
	// The server should have sent from offset 5
	expectedRest := uploadContent[5:]
	if restBuf.String() != expectedRest {
		t.Errorf("RestartAt + Retrieve content mismatch: got %q, want %q", restBuf.String(), expectedRest)
	}
}
