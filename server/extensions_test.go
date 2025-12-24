package server

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

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
