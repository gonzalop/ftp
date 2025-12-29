package server

import (
	"bytes"
	"context"
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
	t.Parallel()
	rootDir := t.TempDir()

	testContent := "Hello, FTP World!"
	err := os.WriteFile(filepath.Join(rootDir, "test.txt"), []byte(testContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
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

	go func() {
		if err := server.Serve(ln); err != nil && err != ErrServerClosed {
			t.Logf("Server stopped: %v", err)
		}
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

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	testPWD(t, c)
	testLIST(t, c, testContent)
	testRETR(t, c, testContent)
	testSTOR(t, c, rootDir)
	testSTOU(t, c, rootDir)
}

func testPWD(t *testing.T, c *ftp.Client) {
	pwd, err := c.CurrentDir()
	if err != nil {
		t.Fatalf("CurrentDir failed: %v", err)
	}
	if pwd != "/" {
		t.Errorf("Expected /, got %s", pwd)
	}
}

func testLIST(t *testing.T, c *ftp.Client, testContent string) {
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
}

func testRETR(t *testing.T, c *ftp.Client, testContent string) {
	var buf bytes.Buffer
	err := c.Retrieve("test.txt", &buf)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if buf.String() != testContent {
		t.Errorf("Content mismatch: got %q, want %q", buf.String(), testContent)
	}
}

func testSTOR(t *testing.T, c *ftp.Client, rootDir string) {
	uploadContent := "Upload success"
	uploadBuf := bytes.NewBufferString(uploadContent)
	if err := c.Store("upload.txt", uploadBuf); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	diskContent, err := os.ReadFile(filepath.Join(rootDir, "upload.txt"))
	if err != nil {
		t.Fatalf("Could not read uploaded file: %v", err)
	}
	if string(diskContent) != uploadContent {
		t.Errorf("Uploaded content mismatch: got %q, want %q", string(diskContent), uploadContent)
	}
}

func testSTOU(t *testing.T, c *ftp.Client, rootDir string) {
	uniqueContent := "Unique upload"
	uniqueBuf := bytes.NewBufferString(uniqueContent)
	uniqueName, err := c.StoreUnique(uniqueBuf)
	if err != nil {
		t.Fatalf("StoreUnique failed: %v", err)
	}
	if uniqueName == "" {
		t.Error("StoreUnique returned empty filename")
	} else {
		t.Logf("StoreUnique generated: %s", uniqueName)
		diskUniqueContent, err := os.ReadFile(filepath.Join(rootDir, uniqueName))
		if err != nil {
			t.Fatalf("Could not read unique file %s: %v", uniqueName, err)
		}
		if string(diskUniqueContent) != uniqueContent {
			t.Errorf("Unique content mismatch: got %q, want %q", string(diskUniqueContent), uniqueContent)
		}
	}
}

func TestServer_ActiveMode(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	err := os.WriteFile(filepath.Join(rootDir, "active.txt"), []byte("active mode content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	driver, _ := NewFSDriver(rootDir, WithAuthenticator(func(u, p, h string, _ net.IP) (string, bool, error) {
		return rootDir, false, nil
	}))

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	server, _ := NewServer(ln.Addr().String(), WithDriver(driver))
	go func() {
		if err := server.Serve(ln); err != nil && err != ErrServerClosed {
			t.Logf("Server serve error: %v", err)
		}
	}()
	defer func() {
		if err := server.Shutdown(context.Background()); err != nil {
			t.Logf("Server shutdown error: %v", err)
		}
	}()

	c, err := ftp.Dial(ln.Addr().String(), ftp.WithActiveMode())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	if err := c.Login("test", "test"); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := c.Retrieve("active.txt", &buf); err != nil {
		t.Fatal(err)
	}

	if buf.String() != "active mode content" {
		t.Errorf("Content mismatch: %s", buf.String())
	}
}

func TestServer_Restart(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	content := "0123456789"
	err := os.WriteFile(filepath.Join(rootDir, "resume.txt"), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	driver, _ := NewFSDriver(rootDir, WithAuthenticator(func(u, p, h string, _ net.IP) (string, bool, error) {
		return rootDir, false, nil
	}))

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	server, _ := NewServer(ln.Addr().String(), WithDriver(driver))
	go func() {
		if err := server.Serve(ln); err != nil && err != ErrServerClosed {
			t.Logf("Server serve error: %v", err)
		}
	}()
	defer func() {
		if err := server.Shutdown(context.Background()); err != nil {
			t.Logf("Server shutdown error: %v", err)
		}
	}()

	c, err := ftp.Dial(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	if err := c.Login("test", "test"); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	// Use RetrieveFrom which handles RestartAt internally
	if err := c.RetrieveFrom("resume.txt", &buf, 5); err != nil {
		t.Fatal(err)
	}

	if buf.String() != "56789" {
		t.Errorf("Expected 56789, got %s", buf.String())
	}
}

func TestListenAndServe(t *testing.T) {
	// Use a random port
	addr := "127.0.0.1:0"
	rootDir := t.TempDir()

	// Start server in a goroutine
	// Since ListenAndServe blocks, we run it async.
	// However, we can't easily shut it down remotely with this simple helper unless we close the listener?
	// The helper doesn't return the server instance, which makes testing graceful shutdown hard.
	// But for this test, we just want to see if it starts up correctly.
	// We'll trust that if it returns an error immediately, it failed.

	errChan := make(chan error, 1)
	go func() {
		errChan <- ListenAndServe(addr, rootDir)
	}()

	select {
	case err := <-errChan:
		t.Fatalf("ListenAndServe failed immediately: %v", err)
	case <-time.After(200 * time.Millisecond):
		// Assume it started successfully if it hasn't returned in 200ms
	}
}
