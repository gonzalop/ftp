package server

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

// TestAdminCommands performs integration tests for MKD, RMD, DELE, APPE.
func TestAdminCommands(t *testing.T) {
	t.Parallel()
	// 1. Setup temporary directory for server root
	rootDir := t.TempDir()

	// 2. Start Server
	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil // Allow write access in rootDir
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

	// 4. Authenticate
	if err := c.Login("admin", "admin"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Test MKD
	newDir := "new_folder"
	if err := c.MakeDir(newDir); err != nil {
		t.Errorf("MakeDir failed: %v", err)
	}
	// Verify checking dir exists
	info, err := os.Stat(filepath.Join(rootDir, newDir))
	if err != nil || !info.IsDir() {
		t.Errorf("Directory not created on disk")
	}

	// Test APPE
	appendFile := "append.txt"
	initialContent := "Part1"
	if err := os.WriteFile(filepath.Join(rootDir, appendFile), []byte(initialContent), 0644); err != nil {
		t.Fatal(err)
	}

	appendData := "Part2"
	buf := bytes.NewBufferString(appendData)
	if err := c.Append(appendFile, buf); err != nil {
		t.Errorf("Append failed: %v", err)
	}

	// Verify content
	fullContent, err := os.ReadFile(filepath.Join(rootDir, appendFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(fullContent) != initialContent+appendData {
		t.Errorf("Append content mismatch: got %q", string(fullContent))
	}

	// Test DELE
	wcFile := "wc_file"
	if err := os.WriteFile(filepath.Join(rootDir, wcFile), []byte("foo"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(wcFile); err != nil {
		t.Errorf("Delete failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, wcFile)); !os.IsNotExist(err) {
		t.Errorf("File not deleted on disk")
	}

	// Test RMD
	if err := c.RemoveDir(newDir); err != nil {
		t.Errorf("RemoveDir failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, newDir)); !os.IsNotExist(err) {
		t.Errorf("Directory not removed on disk")
	}
}

func TestReadOnlyCommands(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()

	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, true, nil // READ ONLY
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

	go func() {
		if err := server.Serve(ln); err != nil && err != ErrServerClosed {
			t.Logf("Server stopped: %v", err)
		}
	}()

	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	// Clean up
	defer func() {
		_ = c.Quit()
	}()

	if err := c.Login("readonly", "readonly"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Test MKD
	if err := c.MakeDir("foo"); err == nil {
		t.Error("MakeDir succeeded in read-only mode")
	}

	// Test DELE
	if err := c.Delete("foo.txt"); err == nil {
		t.Error("Delete succeeded in read-only mode")
	}

	// Test APPE
	buf := bytes.NewBufferString("data")
	if err := c.Append("foo.txt", buf); err == nil {
		t.Error("Append succeeded in read-only mode")
	}
}

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

func TestExtensions_Integration(t *testing.T) {
	// 1. Setup
	rootDir := t.TempDir()
	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil // allow write
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

	go func() {
		if err := server.Serve(ln); err != nil && err != ErrServerClosed {
			t.Logf("Server stopped: %v", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// 2. Connect
	c, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit error: %v", err)
		}
	}()

	if err := c.Login("user", "pass"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// 3. Test SITE CHMOD
	filename := "chmod_test.txt"
	filePath := filepath.Join(rootDir, filename)
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to 0600
	if err := c.Chmod(filename, 0600); err != nil {
		t.Errorf("Chmod failed: %v", err)
	}

	// Verify on disk
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Chmod mismatch: got %v, want -rw-------", info.Mode())
	}

	// 4. Test MFMT (SetModTime)
	// Set to a specific time in the past
	newTime := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := c.SetModTime(filename, newTime); err != nil {
		t.Errorf("SetModTime failed: %v", err)
	}

	// Verify on disk
	info, err = os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(newTime) {
		t.Errorf("ModTime mismatch: got %v, want %v", info.ModTime(), newTime)
	}
}
