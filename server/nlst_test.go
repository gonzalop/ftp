package server

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

func TestNLST(t *testing.T) {
	// 1. Setup temporary directory for server root
	rootDir := t.TempDir()

	// Create some files
	files := []string{"file1.txt", "file2.log", "image.png"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(rootDir, f), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// 2. Start Server
	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
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
	c, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		_ = c.Quit()
	}()

	if err := c.Login("test", "test"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// 4. Test NLST
	entries, err := c.NameList(".")
	if err != nil {
		t.Fatalf("NameList failed: %v", err)
	}

	// Check if we got exactly the filenames
	if len(entries) != len(files) {
		t.Errorf("Expected %d entries, got %d", len(files), len(entries))
	}

	for _, f := range files {
		found := slices.Contains(entries, f)
		if !found {
			t.Errorf("Expected file %q not found in NLST response", f)
		}
	}

	// Additional check: ensure no extra info (like permissions)
	for _, e := range entries {
		if strings.Contains(e, " ") {
			t.Errorf("NLST response contains spaces (likely detailed listing): %q", e)
		}
	}
}
