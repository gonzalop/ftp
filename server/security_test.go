package server

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
		server:   srv,
		pasvList: pasvList,
		remoteIP: "8.8.8.8", // dummy IP different from 127.0.0.1
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

func TestUnauthenticatedCommands(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	driver, _ := NewFSDriver(rootDir, WithAuthenticator(func(u, p, h string, _ net.IP) (string, bool, error) {
		return rootDir, false, nil
	}))

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	srv, _ := NewServer(addr, WithDriver(driver))

	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	client, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Quit()

	// DO NOT LOGIN

	unauthCmds := []struct {
		cmd  string
		args []string
	}{
		{"REST", []string{"100"}},
		{"MODE", []string{"S"}},
		{"STRU", []string{"F"}},
		{"SYST", nil},
	}

	for _, tc := range unauthCmds {
		resp, err := client.Quote(tc.cmd, tc.args...)
		if err != nil {
			t.Errorf("Quote %s failed: %v", tc.cmd, err)
			continue
		}
		if resp.Code != 530 {
			t.Errorf("Expected 530 for unauthenticated %s, got %d %s", tc.cmd, resp.Code, resp.Message)
		}
	}
}

// closeCountingDriver wraps a Driver and counts open ClientContexts.
type closeCountingDriver struct {
	Driver
	open atomic.Int32
}

type closeCountingContext struct {
	ClientContext
	d *closeCountingDriver
}

func (d *closeCountingDriver) Authenticate(user, pass, host string, ip net.IP) (ClientContext, error) {
	ctx, err := d.Driver.Authenticate(user, pass, host, ip)
	if err != nil {
		return nil, err
	}
	d.open.Add(1)
	return &closeCountingContext{ClientContext: ctx, d: d}, nil
}

func (c *closeCountingContext) Close() error {
	c.d.open.Add(-1)
	return c.ClientContext.Close()
}

func TestReloginReleasesContext(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	fs, err := NewFSDriver(rootDir, WithAuthenticator(func(user, pass, _ string, _ net.IP) (string, bool, error) {
		if pass != "good" {
			return "", false, os.ErrPermission
		}
		return rootDir, false, nil
	}))
	fatalIfErr(t, err, "NewFSDriver")
	driver := &closeCountingDriver{Driver: fs}
	_, addr := startRawServer(t, driver)
	c := dialRaw(t, addr)

	for range 5 {
		c.cmd("USER alice")
		if r := c.cmd("PASS good"); !strings.HasPrefix(r, "230 ") {
			t.Fatalf("PASS: got %q, want 230", r)
		}
	}
	if n := driver.open.Load(); n != 1 {
		t.Errorf("open contexts after 5 logins: got %d, want 1", n)
	}

	// A failed login must end the previous one.
	c.cmd("USER mallory")
	if r := c.cmd("PASS bad"); !strings.HasPrefix(r, "530 ") {
		t.Fatalf("bad PASS: got %q, want 530", r)
	}
	if r := c.cmd("PWD"); !strings.HasPrefix(r, "530 ") {
		t.Errorf("PWD after failed login: got %q, want 530", r)
	}
	if n := driver.open.Load(); n != 0 {
		t.Errorf("open contexts after failed login: got %d, want 0", n)
	}
}

// TestTooManyLoginFailures checks that a connection is closed after
// maxLoginFailures failed PASS commands.
func TestTooManyLoginFailures(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	driver, err := NewFSDriver(rootDir, WithAuthenticator(func(_, _, _ string, _ net.IP) (string, bool, error) {
		return "", false, os.ErrPermission
	}))
	fatalIfErr(t, err, "NewFSDriver")
	_, addr := startRawServer(t, driver)
	c := dialRaw(t, addr)

	for i := 1; i <= maxLoginFailures; i++ {
		c.cmd("USER alice")
		r := c.cmd("PASS bad")
		want := "530 "
		if i == maxLoginFailures {
			want = "421 "
		}
		if !strings.HasPrefix(r, want) {
			t.Fatalf("PASS #%d: got %q, want %s", i, r, want)
		}
	}
	_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.r.ReadByte(); err == nil || os.IsTimeout(err) {
		t.Errorf("connection still open after too many failed logins: %v", err)
	}
}

// TestQUITClosesConnection checks that the server closes the control
// connection after replying to QUIT (RFC 959).
func TestQUITClosesConnection(t *testing.T) {
	t.Parallel()
	driver, err := NewFSDriver(t.TempDir())
	fatalIfErr(t, err, "NewFSDriver")
	_, addr := startRawServer(t, driver)
	c := dialRaw(t, addr)

	if r := c.cmd("QUIT"); !strings.HasPrefix(r, "221 ") {
		t.Fatalf("QUIT: got %q, want 221", r)
	}
	_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.r.ReadByte(); err == nil || os.IsTimeout(err) {
		t.Errorf("connection still open after QUIT: %v", err)
	}
}
