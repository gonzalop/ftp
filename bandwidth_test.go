package ftp_test

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
	"github.com/gonzalop/ftp/server"
)

func TestClient_BandwidthLimit(t *testing.T) {
	t.Parallel()
	addr, cleanup, _ := setupServer(t)
	defer cleanup()

	// Create 10KB test data
	data := make([]byte, 10*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Connect with 5KB/s bandwidth limit
	c, err := ftp.Dial(addr,
		ftp.WithTimeout(30*time.Second),
		ftp.WithBandwidthLimit(5*1024), // 5 KB/s
	)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit error: %v", err)
		}
	}()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Test upload with bandwidth limit
	start := time.Now()
	if err := c.Store("bandwidth_test.txt", bytes.NewReader(data)); err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	uploadDuration := time.Since(start)

	// With token bucket burst capacity, first 5KB transfers instantly,
	// then remaining 5KB takes 1 second at 5KB/s = ~1 second total minimum
	// Allow some margin for overhead
	if uploadDuration < 800*time.Millisecond {
		t.Errorf("Upload completed too quickly (%v), bandwidth limiting may not be working", uploadDuration)
	}
	// But shouldn't take more than 3 seconds (with reasonable overhead)
	if uploadDuration > 3*time.Second {
		t.Errorf("Upload took too long (%v), possible performance issue", uploadDuration)
	}

	// Test download with bandwidth limit
	var buf bytes.Buffer
	start = time.Now()
	if err := c.Retrieve("bandwidth_test.txt", &buf); err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	downloadDuration := time.Since(start)

	// With token bucket burst capacity, first 5KB transfers instantly,
	// then remaining 5KB takes 1 second at 5KB/s = ~1 second total minimum
	// Allow some margin for overhead
	if downloadDuration < 800*time.Millisecond {
		t.Errorf("Download completed too quickly (%v), bandwidth limiting may not be working", downloadDuration)
	}
	// But shouldn't take more than 3 seconds (with reasonable overhead)
	if downloadDuration > 3*time.Second {
		t.Errorf("Download took too long (%v), possible performance issue", downloadDuration)
	}

	// Verify data integrity
	if !bytes.Equal(data, buf.Bytes()) {
		t.Error("Data mismatch after bandwidth-limited transfer")
	}
}

func TestServer_BandwidthLimit(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()

	driver, err := server.NewFSDriver(rootDir,
		server.WithAuthenticator(func(user, pass, host string, remoteIP net.IP) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	s, err := server.NewServer("127.0.0.1:0",
		server.WithDriver(driver),
		server.WithBandwidthLimit(10*1024, 5*1024),
	)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := SystemListener()
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		if err := s.Serve(listener); err != nil && err != server.ErrServerClosed {
			t.Logf("Server stopped: %v", err)
		}
	}()

	addr := listener.Addr().String()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	data := make([]byte, 10*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	c, err := ftp.Dial(addr, ftp.WithTimeout(30*time.Second))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit error: %v", err)
		}
	}()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	testServerBandwidthUpload(t, c, data)
	testServerBandwidthDownload(t, c, data)
}

func testServerBandwidthUpload(t *testing.T, c *ftp.Client, data []byte) {
	start := time.Now()
	if err := c.Store("server_bandwidth_test.txt", bytes.NewReader(data)); err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	uploadDuration := time.Since(start)

	// With token bucket burst capacity, allow for faster initial transfer
	if uploadDuration < 800*time.Millisecond {
		t.Errorf("Upload completed too quickly (%v), server bandwidth limiting may not be working", uploadDuration)
	}
	if uploadDuration > 3*time.Second {
		t.Errorf("Upload took too long (%v), possible performance issue", uploadDuration)
	}
}

func testServerBandwidthDownload(t *testing.T, c *ftp.Client, data []byte) {
	var buf bytes.Buffer
	start := time.Now()
	if err := c.Retrieve("server_bandwidth_test.txt", &buf); err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	downloadDuration := time.Since(start)

	// With token bucket burst capacity, allow for faster initial transfer
	if downloadDuration < 800*time.Millisecond {
		t.Errorf("Download completed too quickly (%v), server bandwidth limiting may not be working", downloadDuration)
	}
	if downloadDuration > 3*time.Second {
		t.Errorf("Download took too long (%v), possible performance issue", downloadDuration)
	}

	if !bytes.Equal(data, buf.Bytes()) {
		t.Error("Data mismatch after server bandwidth-limited transfer")
	}
}
