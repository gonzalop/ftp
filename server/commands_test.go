package server

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

// TestAdminCommands performs integration tests for MKD, RMD, DELE, APPE.
func TestAdminCommands(t *testing.T) {
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

	server, err := NewServer(":2122", WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	// Run server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil {
			t.Logf("Server stopped: %v", err)
		}
	}()

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// 3. Connect with Client
	c, err := ftp.Dial("localhost:2122", ftp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

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
	rootDir := t.TempDir()

	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, true, nil // READ ONLY
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(":2123", WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			t.Logf("Server stopped: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)

	c, err := ftp.Dial("localhost:2123", ftp.WithTimeout(5*time.Second))
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
