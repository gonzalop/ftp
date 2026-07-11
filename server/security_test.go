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

func TestHashSizeLimit(t *testing.T) {
	// 1. Setup
	rootDir := t.TempDir()
	largeFile := filepath.Join(rootDir, "large.dat")

	// Create a file larger than 250MB
	f, err := os.Create(largeFile)
	if err != nil {
		t.Fatal(err)
	}
	// Use 251MB
	if err := f.Truncate(251 * 1024 * 1024); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	driver, _ := NewFSDriver(rootDir, WithAuthenticator(func(u, p, h string, _ net.IP) (string, bool, error) {
		return rootDir, false, nil
	}))

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	server, _ := NewServer(addr, WithDriver(driver))

	go func() { _ = server.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	// 2. Connect and try HASH
	client, err := ftp.Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Quit()

	_ = client.Login("user", "pass")

	_, err = client.Hash("large.dat")
	if err == nil {
		t.Fatal("expected error for HASH on large file, got nil")
	}

	if !strings.Contains(err.Error(), "552") {
		t.Errorf("expected 552 error, got: %v", err)
	}
}

func TestPassiveHijack(t *testing.T) {
	// 1. Setup server
	rootDir := t.TempDir()
	driver, _ := NewFSDriver(rootDir, WithAuthenticator(func(u, p, h string, _ net.IP) (string, bool, error) {
		return rootDir, false, nil
	}))

	srv, _ := NewServer("127.0.0.1:0", WithDriver(driver))

	passiveAddr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	pasvList, err := net.ListenTCP("tcp", passiveAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer pasvList.Close()

	s := &session{
		server:    srv,
		pasvList:  pasvList,
		remoteIP:  "8.8.8.8", // dummy IP different from 127.0.0.1
	}

	// In a separate goroutine, dial the passive port (which will connect from 127.0.0.1)
	go func() {
		conn, err := net.Dial("tcp", pasvList.Addr().String())
		if err == nil {
			conn.Close()
		}
	}()

	_, err = s.connPassive()
	if err == nil {
		t.Fatal("expected passive connection to be rejected due to IP mismatch, got nil error")
	}
	if !strings.Contains(err.Error(), "security violation") {
		t.Errorf("expected security violation error, got: %v", err)
	}
}




