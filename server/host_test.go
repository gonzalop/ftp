package server

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestHostCommand(t *testing.T) {
	t.Parallel()
	// 1. Setup server with a logger to capture output
	var logBuf safeBuffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	rootDir := t.TempDir()
	receivedHost := ""
	driver, err := NewFSDriver(rootDir, WithAuthenticator(func(u, p, h string, _ net.IP) (string, bool, error) {
		receivedHost = h
		return rootDir, false, nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(":0",
		WithDriver(driver),
		WithLogger(logger),
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

	// 2. Connect client and send HOST command
	c, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("Client failed to connect: %v", err)
	}

	hostName := "ftp.example.com"
	if err := c.Host(hostName); err != nil {
		t.Fatalf("Host command failed: %v", err)
	}

	// 3. Login and perform an action that logs the host
	if err := c.Login("test", "test"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Verify authenticator received the host
	if receivedHost != hostName {
		t.Errorf("Authenticator received host %q, want %q", receivedHost, hostName)
	}

	if err := c.MakeDir("testdir"); err != nil {
		t.Fatalf("MakeDir failed: %v", err)
	}

	if err := c.Quit(); err != nil {
		t.Logf("c.Quit failed: %v", err)
	}

	// 4. Verify server logs
	logOutput := logBuf.String()
	expectedLog := "host=" + hostName
	if !strings.Contains(logOutput, expectedLog) {
		t.Errorf("Server log did not contain expected host tag.\nExpected: %s\nGot:\n%s", expectedLog, logOutput)
	}
}
