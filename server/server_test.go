package server

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

// TestServerIntegration performs a full end-to-end test of the server
// using the local ftp client package.
func TestServerIntegration(t *testing.T) {
	// 1. Setup temporary directory for server root
	rootDir := t.TempDir()

	// Create a dummy file to download
	testContent := "Hello, FTP World!"
	err := os.WriteFile(filepath.Join(rootDir, "test.txt"), []byte(testContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Start Server
	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil // Allow write access in rootDir
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Listen on random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	server, err := NewServer(addr, WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	// Run server in goroutine
	go func() {
		if err := server.Serve(ln); err != nil && err != ErrServerClosed {
			t.Logf("Server stopped: %v", err)
		}
	}()

	// 3. Connect with Client
	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
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

	// 5. Test PWD
	pwd, err := c.CurrentDir()
	if err != nil {
		t.Fatalf("CurrentDir failed: %v", err)
	}
	if pwd != "/" {
		t.Errorf("Expected /, got %s", pwd)
	}

	// 6. Test LIST
	entries, err := c.List(".")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	found := false
	for _, entry := range entries {
		if entry.Name == "test.txt" {
			found = true
			if entry.Size != int64(len(testContent)) {
				t.Errorf("Expected size %d, got %d", len(testContent), entry.Size)
			}
			break
		}
	}
	if !found {
		t.Error("test.txt not found in listing")
	}

	// 7. Test RETR (Download)
	var buf bytes.Buffer
	err = c.Retrieve("test.txt", &buf)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if buf.String() != testContent {
		t.Errorf("Content mismatch: got %q, want %q", buf.String(), testContent)
	}

	// 8. Test STOR (Upload)
	uploadContent := "Upload success"
	uploadBuf := bytes.NewBufferString(uploadContent)
	if err := c.Store("upload.txt", uploadBuf); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Verify upload on disk
	diskContent, err := os.ReadFile(filepath.Join(rootDir, "upload.txt"))
	if err != nil {
		t.Fatalf("Could not read uploaded file: %v", err)
	}
	if string(diskContent) != uploadContent {
		t.Errorf("Uploaded content mismatch: got %q, want %q", string(diskContent), uploadContent)
	}
}
