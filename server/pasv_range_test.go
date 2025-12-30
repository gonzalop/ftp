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
	t.Parallel()
	// 1. Setup temporary directory for server root
	rootDir := t.TempDir()

	// 2. Start Server with PASV range settings
	minPort := 30000
	maxPort := 30005

	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return rootDir, false, nil
		}),
		WithSettings(&Settings{
			PasvMinPort: minPort,
			PasvMaxPort: maxPort,
		}),
	)
	fatalIfErr(t, err, "Failed to create FS driver")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	fatalIfErr(t, err, "Failed to listen")
	addr := ln.Addr().String()

	server, err := NewServer(addr, WithDriver(driver))
	fatalIfErr(t, err, "Failed to create server")

	go func() {
		if err := server.Serve(ln); err != nil && err != ErrServerClosed {
			t.Logf("Server stopped: %v", err)
		}
	}()

	// 3. Connect with Client
	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
	fatalIfErr(t, err, "Failed to dial")
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	fatalIfErr(t, c.Login("anonymous", "anonymous"), "Failed to login")

	// 4. Send PASV command
	resp, err := c.Quote("PASV")
	fatalIfErr(t, err, "PASV command failed")

	if resp.Code != 227 {
		t.Fatalf("Expected 227 Entering Passive Mode, got %d %s", resp.Code, resp.Message)
	}

	// Parse PASV response: "Entering Passive Mode (h1,h2,h3,h4,p1,p2)"
	start := -1
	end := -1
	for i, r := range resp.Message {
		switch r {
		case '(':
			start = i
		case ')':
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
	fatalIfErr(t, err, "Invalid p1")
	p2, err := strconv.Atoi(parts[5])
	fatalIfErr(t, err, "Invalid p2")

	port := p1*256 + p2

	t.Logf("PASV returned port: %d", port)

	if port < minPort || port > maxPort {
		t.Errorf("PASV port %d is out of range [%d, %d]", port, minPort, maxPort)
	}
}
