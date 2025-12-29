package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

func TestMaxConnections(t *testing.T) {
	t.Parallel()
	// 1. Setup
	rootDir := t.TempDir()
	driver, err := NewFSDriver(rootDir, WithAuthenticator(func(u, p, h string, _ net.IP) (string, bool, error) {
		return rootDir, false, nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	// Limit to 1 connection total
	server, err := NewServer(":0",
		WithDriver(driver),
		WithMaxConnections(1, 0),
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
		if err := server.Serve(ln); err != nil && err != ErrServerClosed {
			t.Logf("server.Serve failed: %v", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Logf("server.Shutdown failed: %v", err)
		}
	}()

	// 2. First connection should succeed
	c1, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("Client 1 failed to connect: %v", err)
	}
	// Note: We don't defer Close() immediately because we need it open for the test

	// 3. Second connection should fail (connection rejected by server)
	// The server accepts the connection but immediately sends 421 and closes.
	// So Dial might succeed but reading the greeting might fail or return 421.
	c2, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err == nil {
		// If Dial succeeds, check if we got a valid connection or if it was closed
		// c2.Noop() should fail if the server closed it.
		// Or the greeting might have been the 421.
		err = c2.Noop()
		if err == nil {
			if err := c2.Quit(); err != nil {
				t.Logf("c2.Quit failed: %v", err)
			}
			t.Fatal("Client 2 should have been rejected")
		}
	} else {
		// Dial error is also acceptable (e.g. EOF during greeting read)
		t.Logf("Client 2 rejected as expected: %v", err)
	}

	// 4. Close first connection and retry second
	if err := c1.Quit(); err != nil {
		t.Logf("c1.Quit failed: %v", err)
	}

	// Wait a bit for server to update state
	// (activeConns tracking updates on handleConnection exit)
	time.Sleep(100 * time.Millisecond)

	c3, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("Client 3 failed to connect after slot freed: %v", err)
	}
	if err := c3.Quit(); err != nil {
		t.Logf("c3.Quit failed: %v", err)
	}
}

func TestMaxConnectionsPerIP(t *testing.T) {
	t.Parallel()
	// 1. Setup
	rootDir := t.TempDir()
	driver, err := NewFSDriver(rootDir, WithAuthenticator(func(u, p, h string, _ net.IP) (string, bool, error) {
		return rootDir, false, nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	// Limit to 1 connection per IP
	server, err := NewServer(":0",
		WithDriver(driver),
		WithMaxConnections(0, 1),
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
		if err := server.Serve(ln); err != nil && err != ErrServerClosed {
			t.Logf("server.Serve failed: %v", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Logf("server.Shutdown failed: %v", err)
		}
	}()

	// 2. First connection should succeed
	c1, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("Client 1 failed to connect: %v", err)
	}

	// 3. Second connection from same IP should fail
	c2, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err == nil {
		// Expect immediate failure/close
		err = c2.Noop()
		if err == nil {
			if err := c2.Quit(); err != nil {
				t.Logf("c2.Quit failed: %v", err)
			}
			t.Fatal("Client 2 should have been rejected")
		}
	} else {
		t.Logf("Client 2 rejected as expected: %v", err)
	}

	// 4. Close first connection and retry
	if err := c1.Quit(); err != nil {
		t.Logf("c1.Quit failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	c3, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("Client 3 failed to connect after slot freed: %v", err)
	}
	if err := c3.Quit(); err != nil {
		t.Logf("c3.Quit failed: %v", err)
	}
}
