package server

import (
	"bufio"
	"context"
	"net"
	"strings"
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

func TestMaxCommandLength(t *testing.T) {
	// 1. Setup server
	rootDir := t.TempDir()
	driver, err := NewFSDriver(rootDir, WithAuthenticator(func(u, p, h string, _ net.IP) (string, bool, error) {
		return rootDir, false, nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(":0", WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	go func() {
		_ = server.Serve(ln)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	// 2. Connect via raw TCP
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Read greeting
	_, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	// 3. Send command that is exactly MaxCommandLength + 1
	// MaxCommandLength is 4096.
	// We'll send "USER " + 4092 'A's + "\n" = 4098 bytes (including \n)
	// ReadSlice('\n') will see more than 4096 before \n.
	oversized := "USER " + strings.Repeat("A", 4100) + "\n"
	_, err = conn.Write([]byte(oversized))
	if err != nil {
		t.Fatal(err)
	}

	// 4. Expect 500 error
	resp, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if !strings.HasPrefix(resp, "500 ") {
		t.Errorf("Expected 500 response for oversized command, got: %s", resp)
	}

	// 5. Expect connection to be closed by server
	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, err = reader.ReadByte()
	if err == nil {
		t.Error("Expected connection to be closed after oversized command, but it remains open")
	}
}

func TestPassiveTimeout(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	driver, _ := NewFSDriver(rootDir, WithAuthenticator(func(u, p, h string, _ net.IP) (string, bool, error) {
		return rootDir, false, nil
	}))

	srv, _ := NewServer("127.0.0.1:0", WithDriver(driver))
	srv.passiveTimeout = 1 * time.Second

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	client, err := ftp.Dial(ln.Addr().String(), ftp.WithTimeout(1*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Quit()

	_ = client.Login("user", "pass")

	// Enter passive mode
	resp, err := client.Quote("EPSV")
	if err != nil {
		t.Fatal(err)
	}

	// Parse EPSV response: 229 Entering Extended Passive Mode (|||port|)
	msg := resp.Message
	start := strings.Index(msg, "|||")
	if start == -1 {
		t.Fatalf("unexpected EPSV response: %s", msg)
	}
	end := strings.Index(msg[start+3:], "|")
	if end == -1 {
		t.Fatalf("unexpected EPSV response: %s", msg)
	}
	portStr := msg[start+3 : start+3+end]

	// Wait 2 seconds (passive timeout is 1 second)
	time.Sleep(2 * time.Second)

	// Trying to connect to the passive port should now fail/timeout or be refused
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", portStr), 500*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Fatal("expected connection to passive port to fail after timeout, but it succeeded")
	}
}

