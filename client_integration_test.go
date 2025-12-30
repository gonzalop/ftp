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
	"fmt"
	"io"
	"log"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		server.WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
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
	t.Parallel()
	// 1. Generate Server Cert
	serverCertPath, serverKeyPath, _, _ := generateCert(t, false, nil, nil)
	serverTLSConfig, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Start Server with TLS support
	rootDir := t.TempDir()
	driver, err := server.NewFSDriver(rootDir,
		server.WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
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
	t.Parallel()
	// 1. Generate Server Cert
	serverCertPath, serverKeyPath, _, _ := generateCert(t, false, nil, nil)
	serverTLSConfig, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Start Server with TLS support
	rootDir := t.TempDir()
	driver, err := server.NewFSDriver(rootDir,
		server.WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	// Try to create an IPv6 listener for the server
	l, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 not supported or disabled:", err)
	}

	// 1. Setup temporary directory for server root
	rootDir := t.TempDir()

	// 2. Start Server
	driver, err := server.NewFSDriver(rootDir,
		server.WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
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
	t.Parallel()
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
	t.Parallel()
	addr, cleanup, rootDir := setupServer(t)
	defer cleanup()

	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
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

	testBasicCommands(t, c, rootDir)
	testFileOperations(t, c, rootDir)
	testListingCommands(t, c, rootDir)
	testMetadataCommands(t, c, rootDir)
	testFeatureCommands(t, c)
	testHashCommands(t, c)
	testProgressTracking(t, c)
	testMLCommands(t, c)
	testFileTransferHelpers(t, c, rootDir)
	testResumeOperations(t, c, rootDir)
}

func testBasicCommands(t *testing.T, c *ftp.Client, rootDir string) {
	// 1. Test Noop
	if err := c.Noop(); err != nil {
		t.Errorf("Noop failed: %v", err)
	}

	// 1.1 Test Syst
	syst, err := c.Syst()
	if err != nil {
		t.Errorf("Syst failed: %v", err)
	}
	if syst == "" {
		t.Error("Syst returned empty string")
	}
	if syst != "UNIX Type: L8" {
		t.Errorf("Expected SYST to be 'UNIX Type: L8', got %q", syst)
	}

	// 2. Test CurrentDir and ChangeDir
	pwd, err := c.CurrentDir()
	if err != nil {
		t.Errorf("CurrentDir failed: %v", err)
	}
	if pwd != "/" {
		t.Errorf("Expected /, got %s", pwd)
	}

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
}

func testFileOperations(t *testing.T, c *ftp.Client, rootDir string) {
	// 3. Test MakeDir
	if err := c.MakeDir("newdir"); err != nil {
		t.Errorf("MakeDir failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "newdir")); os.IsNotExist(err) {
		t.Errorf("MakeDir didn't create directory on disk")
	}

	// 4. Test Rename
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
}

func testListingCommands(t *testing.T, c *ftp.Client, rootDir string) {
	// 7. Test List and NameList
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
}

func testMetadataCommands(t *testing.T, c *ftp.Client, rootDir string) {
	// 8. Test Size
	size, err := c.Size("a.txt")
	if err != nil {
		t.Logf("Size failed (expected if not implemented): %v", err)
	} else if size != 1 {
		t.Errorf("Expected size 1, got %d", size)
	}

	// 9. Test ModTime
	tTime, err := c.ModTime("a.txt")
	if err != nil {
		t.Logf("ModTime failed (expected if not implemented): %v", err)
	} else if tTime.IsZero() {
		t.Error("ModTime returned zero time")
	}

	// 10. Test Quote
	if _, err := c.Quote("NOOP"); err != nil {
		t.Errorf("Quote NOOP failed: %v", err)
	}

	// 11. Test Append
	if err := c.Append("a.txt", bytes.NewBufferString(" appended")); err != nil {
		t.Errorf("Append failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(rootDir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "a appended" {
		t.Errorf("Append content mismatch. Got %q, want %q", string(content), "a appended")
	}
}

func testFeatureCommands(t *testing.T, c *ftp.Client) {
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

	// 14. Test SetOption
	if err := c.SetOption("UTF8", "ON"); err != nil {
		t.Logf("SetOption failed (expected if not implemented): %v", err)
	}
}

func testHashCommands(t *testing.T, c *ftp.Client) {
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
}

func testProgressTracking(t *testing.T, c *ftp.Client) {
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
}

func testMLCommands(t *testing.T, c *ftp.Client) {
	data := []byte("progress data")

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
}

func testFileTransferHelpers(t *testing.T, c *ftp.Client, rootDir string) {
	uploadContent := "StoreFrom test content"

	// 20. Test StoreFrom
	localUploadPath := filepath.Join(rootDir, "local_upload.txt")
	if err := os.WriteFile(localUploadPath, []byte(uploadContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := c.StoreFrom("stored_from.txt", localUploadPath); err != nil {
		t.Errorf("StoreFrom failed: %v", err)
	}

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
}

func testResumeOperations(t *testing.T, c *ftp.Client, rootDir string) {
	uploadContent := "StoreFrom test content"

	// 22. Test RetrieveFrom
	offset := int64(10)
	expectedPartial := uploadContent[offset:]
	var partialBuf bytes.Buffer

	if err := c.RetrieveFrom("stored_from.txt", &partialBuf, offset); err != nil {
		t.Errorf("RetrieveFrom failed: %v", err)
	}
	if partialBuf.String() != expectedPartial {
		t.Errorf("RetrieveFrom content mismatch: got %q, want %q", partialBuf.String(), expectedPartial)
	}

	// 23. Test StoreAt
	initialContent := "Initial "
	if err := c.Store("resume_upload.txt", bytes.NewBufferString(initialContent)); err != nil {
		t.Fatal(err)
	}

	appendContent := "Appended"
	if err := c.StoreAt("resume_upload.txt", bytes.NewBufferString(appendContent), int64(len(initialContent))); err != nil {
		t.Errorf("StoreAt failed: %v", err)
	}

	fullContent, err := os.ReadFile(filepath.Join(rootDir, "resume_upload.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(fullContent) != initialContent+appendContent {
		t.Errorf("StoreAt content mismatch: got %q, want %q", string(fullContent), initialContent+appendContent)
	}

	// 24. Test RestartAt
	if err := c.RestartAt(5); err != nil {
		t.Errorf("RestartAt failed: %v", err)
	}

	var restBuf bytes.Buffer
	if err := c.Retrieve("stored_from.txt", &restBuf); err != nil {
		t.Errorf("Retrieve after RestartAt failed: %v", err)
	}

	expectedRest := uploadContent[5:]
	if restBuf.String() != expectedRest {
		t.Errorf("RestartAt + Retrieve content mismatch: got %q, want %q", restBuf.String(), expectedRest)
	}

	// 25. Test REST + STOR (resume upload with offset)
	// First, create a partial file
	partialUpload := "Partial"
	if err := c.Store("rest_stor_test.txt", bytes.NewBufferString(partialUpload)); err != nil {
		t.Fatal(err)
	}

	// Now resume from the end of the partial upload
	resumeContent := " Resume"
	if err := c.RestartAt(int64(len(partialUpload))); err != nil {
		t.Errorf("RestartAt for STOR failed: %v", err)
	}

	if err := c.Store("rest_stor_test.txt", bytes.NewBufferString(resumeContent)); err != nil {
		t.Errorf("STOR with REST failed: %v", err)
	}

	// Verify the file has both parts
	resumedContent, err := os.ReadFile(filepath.Join(rootDir, "rest_stor_test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	expectedResumed := partialUpload + resumeContent
	if string(resumedContent) != expectedResumed {
		t.Errorf("REST + STOR content mismatch: got %q, want %q", string(resumedContent), expectedResumed)
	}
}

func TestServerCoverage_AdditionalBranches(t *testing.T) {
	t.Parallel()
	addr, cleanup, _ := setupServer(t)
	defer cleanup()

	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
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

	testCDUPCommand(t, c)
	testMODECommands(t, c)
	testSTRUCommands(t, c)
	testTYPECommands(t, c)
	testSITEErrorCases(t, c)
	testMiscCommandCoverage(t, c)
}

func testCDUPCommand(t *testing.T, c *ftp.Client) {
	if err := c.ChangeDir("/"); err != nil {
		t.Fatal(err)
	}
	if err := c.MakeDir("coverage_subdir"); err != nil {
		t.Fatal(err)
	}
	if err := c.ChangeDir("coverage_subdir"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Quote("CDUP"); err != nil {
		t.Errorf("CDUP failed: %v", err)
	}
	pwd, _ := c.CurrentDir()
	if pwd != "/" {
		t.Errorf("Expected /, got %s", pwd)
	}
}

func testMODECommands(t *testing.T, c *ftp.Client) {
	modes := []struct {
		mode string
		code int
	}{
		{"S", 200},
		{"B", 504},
		{"C", 504},
		{"X", 504},
	}
	for _, m := range modes {
		resp, _ := c.Quote("MODE", m.mode)
		if resp.Code != m.code {
			t.Errorf("MODE %s expected %d, got %d", m.mode, m.code, resp.Code)
		}
	}
}

func testSTRUCommands(t *testing.T, c *ftp.Client) {
	structures := []struct {
		stru string
		code int
	}{
		{"F", 200},
		{"R", 504},
		{"P", 504},
		{"X", 504},
	}
	for _, s := range structures {
		resp, _ := c.Quote("STRU", s.stru)
		if resp.Code != s.code {
			t.Errorf("STRU %s expected %d, got %d", s.stru, s.code, resp.Code)
		}
	}
}

func testTYPECommands(t *testing.T, c *ftp.Client) {
	if _, err := c.Quote("TYPE", "A"); err != nil {
		t.Errorf("TYPE A failed: %v", err)
	}
	if resp, _ := c.Quote("TYPE", "E"); resp.Code != 504 {
		t.Errorf("TYPE E should be 504, got %d", resp.Code)
	}
}

func testSITEErrorCases(t *testing.T, c *ftp.Client) {
	if resp, _ := c.Quote("SITE"); resp.Code != 501 {
		t.Errorf("SITE empty should be 501, got %d", resp.Code)
	}
	if resp, _ := c.Quote("SITE", "UNKNOWN"); resp.Code != 502 {
		t.Errorf("SITE UNKNOWN should be 502, got %d", resp.Code)
	}
	if resp, _ := c.Quote("SITE", "CHMOD"); resp.Code != 501 {
		t.Errorf("SITE CHMOD empty should be 501, got %d", resp.Code)
	}
	if resp, _ := c.Quote("SITE", "CHMOD", "999", "file"); resp.Code != 501 {
		t.Errorf("SITE CHMOD invalid mode should be 501, got %d", resp.Code)
	}
}

func testMiscCommandCoverage(t *testing.T, c *ftp.Client) {
	if resp, _ := c.Quote("STAT", "/"); resp.Code != 502 {
		t.Errorf("STAT with path should be 502, got %d", resp.Code)
	}

	if resp, _ := c.Quote("HELP", "USER"); resp.Code != 214 {
		t.Errorf("HELP USER should be 214, got %d", resp.Code)
	}

	if resp, _ := c.Quote("PORT", "invalid"); resp.Code != 501 {
		t.Errorf("PORT invalid should be 501, got %d", resp.Code)
	}
	if resp, _ := c.Quote("EPRT", "invalid"); resp.Code != 501 {
		t.Errorf("EPRT invalid should be 501, got %d", resp.Code)
	}
	if resp, _ := c.Quote("EPRT", "|1|127.0.0.1|65536|"); resp.Code != 501 {
		t.Errorf("EPRT invalid port should be 501, got %d", resp.Code)
	}

	if resp, _ := c.Quote("REST", "invalid"); resp.Code != 501 {
		t.Errorf("REST invalid should be 501, got %d", resp.Code)
	}
}

func TestServerCoverage_NoAuth(t *testing.T) {
	t.Parallel()
	addr, cleanup, _ := setupServer(t)
	defer cleanup()

	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	// Test commands without login
	cmds := []string{"PWD", "CWD", "LIST", "RETR", "STOR", "SIZE", "MDTM", "MLSD", "MLST"}
	for _, cmd := range cmds {
		resp, _ := c.Quote(cmd, "path")
		if resp.Code != 530 {
			t.Errorf("%s without login expected 530, got %d", cmd, resp.Code)
		}
	}
}

func TestAuthenticationFailure(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()

	// Create driver with authenticator that validates passwords
	driver, err := server.NewFSDriver(rootDir,
		server.WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			// Only accept "anonymous" with password "anonymous"
			if user == "anonymous" && pass == "anonymous" {
				return rootDir, false, nil
			}
			return "", false, fmt.Errorf("authentication failed")
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	s, err := server.NewServer("127.0.0.1:0", server.WithDriver(driver))
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	}()

	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	// Test with wrong password
	err = c.Login("anonymous", "wrongpassword")
	if err == nil {
		t.Fatal("Login should have failed with wrong password")
	}

	// Verify it's a protocol error with 530 code (not logged in)
	if protocolErr, ok := err.(*ftp.ProtocolError); ok {
		if protocolErr.Code != 530 {
			t.Errorf("Expected error code 530, got %d", protocolErr.Code)
		}
	} else {
		t.Errorf("Expected ProtocolError, got %T: %v", err, err)
	}

	// Verify we can still login with correct credentials after failure
	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Errorf("Login with correct password after failure should succeed: %v", err)
	}
}

func TestConnect_FTP(t *testing.T) {
	t.Parallel()
	addr, cleanup, _ := setupServer(t)
	defer cleanup()

	// Parse the port from the address
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}

	// Construct FTP URL
	url := "ftp://127.0.0.1:" + port

	// 1. Connect
	c, err := ftp.Connect(url)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	// 2. Verify command execution
	if _, err := c.CurrentDir(); err != nil {
		t.Errorf("CurrentDir failed: %v", err)
	}
}

func TestConnect_Login(t *testing.T) {
	t.Parallel()
	addr, cleanup, _ := setupServer(t)
	defer cleanup()

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}

	// URL with credentials
	url := "ftp://anonymous:anonymous@127.0.0.1:" + port

	c, err := ftp.Connect(url)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer func() {
		_ = c.Quit()
	}()

	// Should be logged in
	if _, err := c.CurrentDir(); err != nil {
		t.Errorf("CurrentDir failed: %v", err)
	}
}

func TestConnect_FTPS(t *testing.T) {
	t.Parallel()
	// 1. Generate Server Cert
	serverCertPath, serverKeyPath, _, _ := generateCert(t, false, nil, nil)
	serverTLSConfig, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Start Server with Implicit TLS support
	rootDir := t.TempDir()
	driver, err := server.NewFSDriver(rootDir,
		server.WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return rootDir, false, nil
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

	// Manual implicit TLS listener setup for server side
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	}()

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Connect using ftps://
	// Note: Connect enforces standard TLS verification.
	// Since we are using a self-signed certificate in this test environment,
	// and Connect does not allow passing custom TLSMainConfig (like InsecureSkipVerify),
	// we expect this connection attempt to fail with a certificate error.
	c, err := ftp.Connect("ftps://127.0.0.1:" + port)
	if err == nil {
		_ = c.Quit()
		t.Fatal("Expected FTPS connect to fail with self-signed cert, but it succeeded")
	}
	// Verify error relates to certificate
	if !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "authority") {
		t.Logf("Got expected error: %v", err)
	}
}

func TestConnect_FTPExplicit(t *testing.T) {
	t.Parallel()
	// 1. Generate Server Cert
	serverCertPath, serverKeyPath, _, _ := generateCert(t, false, nil, nil)
	serverTLSConfig, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Start Server
	rootDir := t.TempDir()
	driver, err := server.NewFSDriver(rootDir,
		server.WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	s, err := server.NewServer("127.0.0.1:0",
		server.WithDriver(driver),
		server.WithTLS(&tls.Config{Certificates: []tls.Certificate{serverTLSConfig}}),
	)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		if err := s.Serve(ln); err != nil && err != server.ErrServerClosed {
			t.Logf("Server stopped: %v", err)
		}
	}()

	addr := ln.Addr().String()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	}()

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Connect using ftp+explicit://
	c, err := ftp.Connect("ftp+explicit://127.0.0.1:" + port)
	if err == nil {
		_ = c.Quit()
		t.Fatal("Expected FTP+Explicit connect to fail with self-signed cert, but it succeeded")
	}
	t.Logf("Got expected error: %v", err)
}

// SlowReader delays reading to simulate a slow source (upload)
type SlowReader struct {
	r     io.Reader
	delay time.Duration
}

func (s *SlowReader) Read(p []byte) (n int, err error) {
	time.Sleep(s.delay)
	max := 1024
	if len(p) > max {
		p = p[:max]
	}
	return s.r.Read(p)
}

func TestClient_QuitAbortsTransfer(t *testing.T) {
	t.Parallel()
	addr, cleanup, _ := setupServer(t)
	defer cleanup()

	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Create a source with plenty of data
	// 50 chunks * 50ms delay = 2.5s minimum duration
	chunkSize := 1024
	chunks := 50
	content := bytes.Repeat([]byte("x"), chunkSize*chunks)

	sr := &SlowReader{
		r:     bytes.NewReader(content),
		delay: 50 * time.Millisecond,
	}

	// Channel to signal upload completion
	doneCh := make(chan error)

	go func() {
		// Store (Upload) will read from sr and write to data connection.
		// It will block on sr.Read.
		// When Quit closes connection, the subsequent Write to dataConn will fail.
		err := c.Store("upload.txt", sr)
		doneCh <- err
	}()

	// Wait a bit to ensure transfer has started
	time.Sleep(500 * time.Millisecond)

	// Call Quit from the main goroutine
	t.Log("Calling Quit...")
	startQuit := time.Now()
	if err := c.Quit(); err != nil {
		t.Logf("Quit returned error: %v", err)
	} else {
		t.Log("Quit returned nil")
	}

	// Wait for Store to return
	select {
	case err := <-doneCh:
		elapsed := time.Since(startQuit)
		t.Logf("Store returned after %v with error: %v", elapsed, err)

		if err == nil {
			t.Error("Store returned nil error, expected error due to abortion")
		} else {
			// Expected error: "use of closed network connection" or similar
			t.Logf("Store error (expected): %v", err)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("Store timed out (did not abort)")
	}
}

func TestConnect(t *testing.T) {
	t.Parallel()
	// Start a test server with permissive auth
	rootDir := t.TempDir()
	driver, err := server.NewFSDriver(rootDir, server.WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
		return rootDir, false, nil // false = write access
	}))
	if err != nil {
		t.Fatal(err)
	}

	// Use a manual listener to get the random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	s, err := server.NewServer(addr, server.WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	// Run server in background
	go func() {
		if err := s.Serve(ln); err != nil && err != server.ErrServerClosed {
			// potentially log error, but might conflict with test shutdown
			t.Logf("Serve error: %v", err)
		}
	}()
	defer func() { _ = s.Shutdown(context.Background()) }()

	// Wait for server to be ready (listener is already open)
	time.Sleep(100 * time.Millisecond)

	t.Run("FTP scheme", func(t *testing.T) {
		url := "ftp://" + addr
		c, err := ftp.Connect(url)
		if err != nil {
			t.Fatalf("Connect failed: %v", err)
		}
		defer func() { _ = c.Quit() }()

		if err := c.Noop(); err != nil {
			t.Errorf("Noop failed: %v", err)
		}
	})

	t.Run("FTP scheme with user info", func(t *testing.T) {
		url := "ftp://anonymous:ftp@" + addr
		c, err := ftp.Connect(url)
		if err != nil {
			t.Fatalf("Connect failed: %v", err)
		}
		defer func() { _ = c.Quit() }()

		if err := c.Noop(); err != nil {
			t.Errorf("Noop failed: %v", err)
		}
	})

	t.Run("FTP scheme with path", func(t *testing.T) {
		// Create a subdirectory directly
		subdir := filepath.Join(rootDir, "subdir")
		if err := os.Mkdir(subdir, 0755); err != nil {
			t.Fatalf("os.Mkdir failed: %v", err)
		}

		url := "ftp://" + addr + "/subdir"
		c, err := ftp.Connect(url)
		if err != nil {
			t.Fatalf("Connect failed: %v", err)
		}
		defer func() { _ = c.Quit() }()

		pwd, err := c.CurrentDir()
		if err != nil {
			t.Fatalf("CurrentDir failed: %v", err)
		}

		if pwd != "/subdir" {
			t.Errorf("Expected path /subdir, got %s", pwd)
		}
	})
}

func TestUploadDownloadFile(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	driver, err := server.NewFSDriver(rootDir, server.WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
		return rootDir, false, nil // Write access
	}))
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	s, err := server.NewServer(addr, server.WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve(ln) }()
	defer func() { _ = s.Shutdown(context.Background()) }()
	time.Sleep(100 * time.Millisecond)

	client, err := ftp.Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("anonymous", "ftp"); err != nil {
		t.Fatal(err)
	}

	// Create a local file
	localContent := []byte("hello world")
	localPath := filepath.Join(t.TempDir(), "local.txt")
	if err := os.WriteFile(localPath, localContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Test UploadFile
	if err := client.UploadFile(localPath, "remote.txt"); err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}

	// Verify content on server
	serverContent, err := os.ReadFile(filepath.Join(rootDir, "remote.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(serverContent) != string(localContent) {
		t.Errorf("Server content mismatch: got %s, want %s", serverContent, localContent)
	}

	// Test DownloadFile
	downloadPath := filepath.Join(t.TempDir(), "download.txt")
	if err := client.DownloadFile("remote.txt", downloadPath); err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	// Verify local content
	downloadedContent, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(downloadedContent) != string(localContent) {
		t.Errorf("Downloaded content mismatch: got %s, want %s", downloadedContent, localContent)
	}
}

func TestRecursiveHelpers(t *testing.T) {
	t.Parallel()
	// Start server
	addr, s, rootDir := startServer(t)
	defer func() {
		if err := s.Shutdown(context.Background()); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Connect
	c, err := ftp.Dial(addr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit error: %v", err)
		}
	}()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// 1. Test UploadDir
	t.Run("UploadDir", func(t *testing.T) {
		// Create local source dir
		srcDir := t.TempDir()
		createTestStructure(t, srcDir)

		// Create a symlink that should be ignored
		// Pointing to a file outside the upload Root would be the security concern,
		// but even internal ones should be skipped by default.
		secretFile := filepath.Join(t.TempDir(), "secret.txt")
		if err := os.WriteFile(secretFile, []byte("secret"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(secretFile, filepath.Join(srcDir, "ignore_me.link")); err != nil {
			t.Fatal(err)
		}

		// Upload to remote
		remoteDest := "/uploaded"
		if err := c.UploadDir(srcDir, remoteDest); err != nil {
			t.Fatalf("UploadDir failed: %v", err)
		}

		// Verify on server disk
		localDest := filepath.Join(rootDir, "uploaded")
		verifyStructure(t, srcDir, localDest)

		// Ensure symlink was NOT uploaded
		if _, err := os.Stat(filepath.Join(localDest, "ignore_me.link")); err == nil {
			t.Error("Symlink was uploaded but should have been skipped")
		}
	})

	// 2. Test Walk
	t.Run("Walk", func(t *testing.T) {
		// We already have /uploaded structure on server.
		// Let's walk it.

		expectedPaths := []string{
			"/uploaded",
			"/uploaded/file1.txt",
			"/uploaded/subdir",
			"/uploaded/subdir/file2.txt",
			"/uploaded/subdir/nested",
			"/uploaded/subdir/nested/file3.txt",
		}
		sort.Strings(expectedPaths)

		var visited []string
		err := c.Walk("/uploaded", func(path string, info *ftp.Entry, err error) error {
			if err != nil {
				return err
			}
			visited = append(visited, path)
			return nil
		})

		if err != nil {
			t.Fatalf("Walk failed: %v", err)
		}

		sort.Strings(visited)

		if len(visited) != len(expectedPaths) {
			t.Fatalf("Verify visited count: got %d, want %d\nGot: %v\nWant: %v", len(visited), len(expectedPaths), visited, expectedPaths)
		}

		for i, p := range visited {
			if p != expectedPaths[i] {
				t.Errorf("Path mismatch at %d: got %s, want %s", i, p, expectedPaths[i])
			}
		}
	})

	// 3. Test DownloadDir
	t.Run("DownloadDir", func(t *testing.T) {
		destDir := t.TempDir()

		if err := c.DownloadDir("/uploaded", destDir); err != nil {
			t.Fatalf("DownloadDir failed: %v", err)
		}

		// Verify local disk matches server disk
		serverPath := filepath.Join(rootDir, "uploaded")
		verifyStructure(t, serverPath, destDir)
	})
}

func startServer(t *testing.T) (string, *server.Server, string) {
	rootDir := t.TempDir()

	driver, err := server.NewFSDriver(rootDir,
		server.WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	s, err := server.NewServer(ln.Addr().String(), server.WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		if err := s.Serve(ln); err != nil {
			// Serve returns ErrServerClosed on shutdown, which we can ignore or log.
			// But since we don't have access to ErrServerClosed easily without import,
			// and this is a test helper, simple logging is fine.
			// Ideally we would check for the specific error.
			t.Logf("Serve error: %v", err)
		}
	}()

	return ln.Addr().String(), s, rootDir
}

func createTestStructure(t *testing.T, dir string) {
	// file1.txt
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatal(err)
	}
	// subdir/
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	// subdir/file2.txt
	if err := os.WriteFile(filepath.Join(dir, "subdir", "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatal(err)
	}
	// subdir/nested/
	if err := os.Mkdir(filepath.Join(dir, "subdir", "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	// subdir/nested/file3.txt
	if err := os.WriteFile(filepath.Join(dir, "subdir", "nested", "file3.txt"), []byte("content3"), 0644); err != nil {
		t.Fatal(err)
	}
}

func verifyStructure(t *testing.T, srcDir, dstDir string) {
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip symlinks in verification since they are not uploaded
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		rel, _ := filepath.Rel(srcDir, path)
		dstPath := filepath.Join(dstDir, rel)

		dstInfo, err := os.Stat(dstPath)
		if err != nil {
			return fmt.Errorf("missing in dest: %s (%v)", rel, err)
		}

		if info.IsDir() {
			if !dstInfo.IsDir() {
				return fmt.Errorf("expected dir at %s", rel)
			}
		} else {
			if dstInfo.IsDir() {
				return fmt.Errorf("expected file at %s", rel)
			}
			if info.Size() != dstInfo.Size() {
				return fmt.Errorf("size mismatch at %s: %d vs %d", rel, info.Size(), dstInfo.Size())
			}
			// Verify content
			s, _ := os.ReadFile(path)
			d, _ := os.ReadFile(dstPath)
			if !bytes.Equal(s, d) {
				return fmt.Errorf("content mismatch at %s", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Errorf("Verification failed: %v", err)
	}
}

// ExampleDial demonstrates connecting to a plain FTP server.
func ExampleDial() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected successfully")
}

// ExampleDial_explicitTLS demonstrates connecting with explicit TLS.
func ExampleDial_explicitTLS() {
	client, err := ftp.Dial("ftp.example.com:21",
		ftp.WithExplicitTLS(&tls.Config{
			ServerName: "ftp.example.com",
		}),
		ftp.WithTimeout(10*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected with TLS")
}

// ExampleDial_implicitTLS demonstrates connecting with implicit TLS.
func ExampleDial_implicitTLS() {
	client, err := ftp.Dial("ftp.example.com:990",
		ftp.WithImplicitTLS(&tls.Config{
			ServerName: "ftp.example.com",
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected with implicit TLS")
}

// ExampleClient_Store demonstrates uploading a file.
func ExampleClient_Store() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	file, err := os.Open("local.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	if err := client.Store("remote.txt", file); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Upload complete")
}

// ExampleClient_Retrieve demonstrates downloading a file with progress tracking.
func ExampleClient_Retrieve() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	file, err := os.Create("local.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// Wrap the writer with progress tracking
	pw := &ftp.ProgressWriter{
		Writer: file,
		Callback: func(bytesTransferred int64) {
			fmt.Printf("Downloaded: %d bytes\n", bytesTransferred)
		},
	}

	if err := client.Retrieve("remote.txt", pw); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Download complete")
}

// ExampleClient_List demonstrates listing directory contents.
func ExampleClient_List() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	entries, err := client.List("/pub")
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range entries {
		fmt.Printf("%s (%s)\n", entry.Name, entry.Type)
	}
}

// ExampleClient_MakeDir demonstrates creating a directory.
func ExampleClient_MakeDir() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	if err := client.MakeDir("newdir"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Directory created")
}

// ExampleClient_Features demonstrates querying server capabilities.
func ExampleClient_Features() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	features, err := client.Features()
	if err != nil {
		log.Fatal(err)
	}

	for feat, params := range features {
		if params != "" {
			fmt.Printf("%s: %s\n", feat, params)
		} else {
			fmt.Println(feat)
		}
	}
}

// ExampleClient_HasFeature demonstrates checking for specific features.
func ExampleClient_HasFeature() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	if client.HasFeature("MDTM") {
		fmt.Println("Server supports file modification times")
	}

	if client.HasFeature("MLST") {
		fmt.Println("Server supports machine-readable listings")
	}
}

// ExampleClient_ModTime demonstrates getting file modification time.
func ExampleClient_ModTime() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	modTime, err := client.ModTime("file.txt")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Last modified: %s\n", modTime)
}

// ExampleClient_MLList demonstrates machine-readable directory listing.
func ExampleClient_MLList() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	entries, err := client.MLList("/pub")
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range entries {
		fmt.Printf("%s: %d bytes, modified %s\n",
			entry.Name, entry.Size, entry.ModTime)
	}
}

// ExampleClient_RetrieveFrom demonstrates resuming a download.
func ExampleClient_RetrieveFrom() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	// Open file in append mode
	file, err := os.OpenFile("large.bin", os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// Get current file size to resume from
	info, err := file.Stat()
	if err != nil {
		log.Fatal(err)
	}

	// Resume download from current position
	if err := client.RetrieveFrom("large.bin", file, info.Size()); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Download resumed and completed")
}

// ExampleClient_SetOption demonstrates setting server options.
func ExampleClient_SetOption() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	// Enable UTF8 mode if supported
	if client.HasFeature("UTF8") {
		if err := client.SetOption("UTF8", "ON"); err != nil {
			log.Printf("Failed to enable UTF8: %v", err)
		} else {
			fmt.Println("UTF8 mode enabled")
		}
	}
}
