package server

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

func TestHostCommand(t *testing.T) {
	// 1. Setup server with a logger to capture output
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	rootDir := t.TempDir()
	driver, err := NewFSDriver(rootDir, WithAuthenticator(func(u, p, h string) (string, bool, error) {
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
