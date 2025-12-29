package server

import (
	"context"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

// TestServer_Shutdown verifies that Shutdown stops the server and closes connections.
func TestServer_Shutdown(t *testing.T) {
	t.Parallel()
	// 1. Setup
	rootDir := t.TempDir()
	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Make sure we use a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	// Close immediately, we just wanted a free port.
	// Actually, Serve takes a listener, so we can just use this listener.
	// But NewServer takes an addr string.
	// Let's use the addr we got.
	ln.Close()

	server, err := NewServer(addr, WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	// 2. Start Server
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// 3. Connect Client
	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// 4. Shutdown Server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// 5. Verify ListenAndServe returned (should be ErrServerClosed)
	select {
	case err := <-errCh:
		if err != ErrServerClosed {
			t.Errorf("Expected ErrServerClosed, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("ListenAndServe did not return after Shutdown")
	}

	// 6. Verify Client is disconnected
	// Any operation should fail
	_, err = c.CurrentDir()
	if err == nil {
		t.Error("Client operation succeeded after server shutdown")
	}
}

// BlockingFile is a file that blocks on Read until closed.
type BlockingFile struct {
	read chan struct{}
}

func (f *BlockingFile) Read(p []byte) (n int, err error) {
	<-f.read // Block forever unless closed
	return 0, io.EOF
}

func (f *BlockingFile) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}

func (f *BlockingFile) Close() error {
	close(f.read)
	return nil
}

func (f *BlockingFile) Stat() (os.FileInfo, error) {
	return nil, nil // Not used strictly in this test path typically
}

func (f *BlockingFile) Write(p []byte) (n int, err error) {
	return len(p), nil
}

type BlockingDriver struct {
	*FSDriver
}

// BlockingContext wraps FSContext to intercept OpenFile
type BlockingContext struct {
	ClientContext
}

func (c *BlockingContext) OpenFile(path string, flag int) (io.ReadWriteCloser, error) {
	if path == "blocking.txt" {
		// Mock a blocking file
		return &BlockingFile{read: make(chan struct{})}, nil
	}
	return c.ClientContext.OpenFile(path, flag)
}

func (d *BlockingDriver) Authenticate(user, pass, host string, remoteIP net.IP) (ClientContext, error) {
	ctx, err := d.FSDriver.Authenticate(user, pass, host, remoteIP)
	if err != nil {
		return nil, err
	}
	return &BlockingContext{ClientContext: ctx}, nil
}

func TestServer_Shutdown_DataConn(t *testing.T) {
	t.Parallel()
	// 1. Setup
	rootDir := t.TempDir()
	baseDriver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	driver := &BlockingDriver{FSDriver: baseDriver}

	// Use random port
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()

	server, err := NewServer(addr, WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	go func() { _ = server.ListenAndServe() }()
	time.Sleep(100 * time.Millisecond)

	// 2. Connect
	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatal(err)
	}

	// 3. Start blocking download in goroutine
	done := make(chan error)
	go func() {
		// Use Retrieve, Retr doesn't exist? Check client.go if needed, but Retrieve is standard
		// Retrieve writes to io.Writer. We can discard.
		err := c.Retrieve("blocking.txt", io.Discard)
		done <- err
	}()

	// Give it time to establish data connection and block
	time.Sleep(200 * time.Millisecond)

	// 4. Shutdown
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	// Shutdown should return quickly, and kill the data conn
	if err := server.Shutdown(ctx); err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// 5. Check results
	select {
	case err := <-done:
		if err == nil {
			t.Error("Expected error from Retr, got nil")
		} else {
			// This is good, it failed.
			// Ideally we wanna see "connection reset" or "EOF" or "closed connection"
			t.Logf("Retr failed as expected: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Retr blocked indefinitely! Shutdown did not kill data connection.")
	}

	if time.Since(start) > 1*time.Second {
		t.Error("Shutdown took too long, maybe blocked on connection close")
	}
}
