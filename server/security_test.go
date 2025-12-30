package server

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

func TestSecurity_SymlinkTraversal(t *testing.T) {
	t.Parallel()
	// 1. Setup directories
	// /tmp/root - FTP root
	// /tmp/outside - Outside root (forbidden)
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "root")
	outsideDir := filepath.Join(tmpDir, "outside")

	fatalIfErr(t, os.Mkdir(rootDir, 0755), "Failed to create root dir")
	fatalIfErr(t, os.Mkdir(outsideDir, 0755), "Failed to create outside dir")

	// Create a target file outside root
	targetFile := filepath.Join(outsideDir, "target.txt")
	fatalIfErr(t, os.WriteFile(targetFile, []byte("secret"), 0644), "Failed to write target file")

	// Create a symlink inside root pointing to outside
	symlink := filepath.Join(rootDir, "badlink")
	fatalIfErr(t, os.Symlink(outsideDir, symlink), "Failed to create symlink")

	// 2. Start Server
	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return rootDir, false, nil // allow write
		}),
	)
	fatalIfErr(t, err, "Failed to create FS driver")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	fatalIfErr(t, err, "Failed to listen")
	addr := ln.Addr().String()

	server, err := NewServer(addr, WithDriver(driver))
	fatalIfErr(t, err, "Failed to create server")

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

	// 3. Connect Client
	c, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	fatalIfErr(t, err, "Dial failed")
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit error: %v", err)
		}
	}()

	fatalIfErr(t, c.Login("user", "pass"), "Login failed")

	// 4. Attempt attacks via symlink

	// Attack 1: CHMOD through symlink
	// Path from FTP root perspective: /badlink/target.txt
	err = c.Chmod("badlink/target.txt", 0600)
	if err == nil {
		// Verify if it actually changed
		info, _ := os.Stat(targetFile)
		if info.Mode().Perm() == 0600 {
			t.Error("SECURITY FAIL: Chmod modified file outside root via symlink")
		} else {
			t.Log("Chmod reported success but might not have changed file (unlikely)")
		}
	} else {
		t.Logf("Chmod blocked (good): %v", err)
	}

	// Attack 2: MFMT (SetModTime) through symlink
	newTime := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	err = c.SetModTime("badlink/target.txt", newTime)
	if err == nil {
		// Verify
		info, _ := os.Stat(targetFile)
		if info.ModTime().Equal(newTime) {
			t.Error("SECURITY FAIL: SetModTime modified file outside root via symlink")
		}
	} else {
		t.Logf("SetModTime blocked (good): %v", err)
	}

	// Attack 3: Rename through symlink
	// Rename badlink/target.txt to badlink/renamed.txt
	err = c.Rename("badlink/target.txt", "badlink/renamed.txt")
	if err == nil {
		if _, err := os.Stat(filepath.Join(outsideDir, "renamed.txt")); err == nil {
			t.Error("SECURITY FAIL: Rename modified file outside root via symlink")
		}
	} else {
		t.Logf("Rename blocked (good): %v", err)
	}
}

func TestSecurity_ErrorSanitization(t *testing.T) {
	t.Parallel()
	// 1. Setup server
	rootDir := t.TempDir()
	// Create a real root path that is long/identifiable
	realRoot, _ := filepath.EvalSymlinks(rootDir)

	driver, err := NewFSDriver(realRoot,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return realRoot, false, nil
		}),
	)
	fatalIfErr(t, err, "Failed to create FS driver")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	fatalIfErr(t, err, "Failed to listen")
	addr := ln.Addr().String()

	server, err := NewServer(addr, WithDriver(driver))
	fatalIfErr(t, err, "Failed to create server")

	go func() {
		_ = server.Serve(ln)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	// 2. Connect
	c, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	fatalIfErr(t, err, "Dial failed")
	defer func() {
		_ = c.Quit()
	}()

	fatalIfErr(t, c.Login("user", "pass"), "Login failed")

	// 3. Trigger errors and check for path disclosure

	// Case 1: Rename with invalid characters or racy condition
	// (Note: we want to trigger the generic error path in driver_fs.go)
	// We'll try to rename something that definitely exists to an invalid path
	fatalIfErr(t, os.WriteFile(filepath.Join(realRoot, "exist.txt"), []byte("test"), 0644), "Failed to write exist.txt")

	// Try to rename through a non-existent directory component
	// This might trigger "failed to resolve destination path" or similar
	err = c.Rename("exist.txt", "nonexistent/new.txt")
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, realRoot) {
			t.Errorf("SECURITY FAIL: Error message leaked absolute root path!\nPath: %s\nError: %s", realRoot, errMsg)
		} else {
			t.Logf("Rename error sanitized (good): %s", errMsg)
		}
	}

	// Case 2: MFMT on non-existent path
	// This should return os.ErrNotExist, which is safe ("File not found").
	err = c.SetModTime("nonexistent.txt", time.Now())
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, realRoot) {
			t.Errorf("SECURITY FAIL: MFMT leaked absolute root path!\nPath: %s\nError: %s", realRoot, errMsg)
		} else {
			t.Logf("MFMT error sanitized (good): %s", errMsg)
		}
	}
}
