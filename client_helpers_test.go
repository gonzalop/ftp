package ftp

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gonzalop/ftp/server"
)

func TestConnect(t *testing.T) {
	// Start a test server with permissive auth
	rootDir := t.TempDir()
	driver, err := server.NewFSDriver(rootDir, server.WithAuthenticator(func(user, pass, host string) (string, bool, error) {
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
		c, err := Connect(url)
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
		c, err := Connect(url)
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
		c, err := Connect(url)
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
	rootDir := t.TempDir()
	driver, err := server.NewFSDriver(rootDir, server.WithAuthenticator(func(user, pass, host string) (string, bool, error) {
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

	client, err := Dial(addr)
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
