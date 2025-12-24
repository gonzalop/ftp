package server

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

func TestPasvPortRange(t *testing.T) {
	// 1. Setup temporary directory for server root
	rootDir := t.TempDir()

	// 2. Start Server with PASV range settings
	minPort := 30000
	maxPort := 30005

	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil
		}),
		WithSettings(&Settings{
			PasvMinPort: minPort,
			PasvMaxPort: maxPort,
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

	// 3. Connect with Client
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

	// 4. Send PASV command
	resp, err := c.Quote("PASV")
	if err != nil {
		t.Fatalf("PASV command failed: %v", err)
	}

	if resp.Code != 227 {
		t.Fatalf("Expected 227 Entering Passive Mode, got %d %s", resp.Code, resp.Message)
	}

	// Parse PASV response: "Entering Passive Mode (h1,h2,h3,h4,p1,p2)"
	start := -1
	end := -1
	for i, r := range resp.Message {
		if r == '(' {
			start = i
		} else if r == ')' {
			end = i
		}
	}

	if start == -1 || end == -1 || start >= end {
		t.Fatalf("Invalid PASV response format: %s", resp.Message)
	}

	parts := strings.Split(resp.Message[start+1:end], ",")
	if len(parts) != 6 {
		t.Fatalf("Invalid PASV response parts: %v", parts)
	}

	p1, err := strconv.Atoi(parts[4])
	if err != nil {
		t.Fatalf("Invalid p1: %v", err)
	}
	p2, err := strconv.Atoi(parts[5])
	if err != nil {
		t.Fatalf("Invalid p2: %v", err)
	}

	port := p1*256 + p2

	t.Logf("PASV returned port: %d", port)

	if port < minPort || port > maxPort {
		t.Errorf("PASV port %d is out of range [%d, %d]", port, minPort, maxPort)
	}
}
