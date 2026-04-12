package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

func TestUnauthenticatedSitePanic(t *testing.T) {
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
		_ = server.Shutdown(ctx)
	}()

	c, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer c.Quit()

	// DO NOT LOGIN

	// This should NOT panic. It should return 530 (Not logged in)
	resp, err := c.Quote("SITE", "CHMOD", "755", "test.txt")
	if err != nil {
		t.Fatalf("Quote failed (unexpected error): %v", err)
	}

	if resp.Code != 530 {
		t.Errorf("Expected 530 Not logged in, got %d %s", resp.Code, resp.Message)
	}
}
