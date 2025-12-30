package server

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

func TestClientExtensions(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(":0",
		WithDriver(driver),
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
	time.Sleep(100 * time.Millisecond)

	c, err := ftp.Dial(addr)
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

	t.Run("SetModTime", func(t *testing.T) { testSetModTime(t, c, rootDir) })
	t.Run("Chmod", func(t *testing.T) { testChmod(t, c, rootDir) })
	t.Run("Hash", func(t *testing.T) { testHash(t, c, rootDir) })
	t.Run("Quote", func(t *testing.T) { testQuote(t, c) })
}

func testSetModTime(t *testing.T, c *ftp.Client, rootDir string) {
	filename := "test_mfmt.txt"
	path := filepath.Join(rootDir, filename)
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	newTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	if err := c.SetModTime(filename, newTime); err != nil {
		t.Fatalf("SetModTime failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if !info.ModTime().UTC().Equal(newTime) {
		t.Errorf("Time mismatch. Expected %v, got %v", newTime, info.ModTime().UTC())
	}

	mdtm, err := c.ModTime(filename)
	if err != nil {
		t.Fatalf("ModTime failed: %v", err)
	}
	if !mdtm.Equal(newTime) {
		t.Errorf("Client ModTime mismatch. Expected %v, got %v", newTime, mdtm)
	}
}

func testChmod(t *testing.T, c *ftp.Client, rootDir string) {
	filename := "test_chmod.sh"
	path := filepath.Join(rootDir, filename)
	if err := os.WriteFile(path, []byte("#!/bin/sh"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	newMode := os.FileMode(0755)
	if err := c.Chmod(filename, newMode); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if info.Mode().Perm() != newMode.Perm() {
		t.Errorf("Mode mismatch. Expected %v, got %v", newMode.Perm(), info.Mode().Perm())
	}
}

func testHash(t *testing.T, c *ftp.Client, rootDir string) {
	filename := "test_hash.txt"
	path := filepath.Join(rootDir, filename)
	content := []byte("hash me")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	if err := c.SetHashAlgo("SHA-1"); err != nil {
		t.Fatalf("SetHashAlgo failed: %v", err)
	}

	hash, err := c.Hash(filename)
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}

	expected := "43f932e4f7c6ecd136a695b7008694bb69d517bd"
	if hash != expected {
		t.Errorf("Hash mismatch. Expected %s, got %s", expected, hash)
	}
}

func testQuote(t *testing.T, c *ftp.Client) {
	resp, err := c.Quote("NOOP")
	if err != nil {
		t.Fatalf("Quote failed: %v", err)
	}
	if resp.Code != 200 {
		t.Errorf("Expected 200 response, got %d", resp.Code)
	}
}
