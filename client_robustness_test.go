package ftp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// withEPSV makes ms answer EPSV with a fresh data listener and returns it.
func withEPSV(t *testing.T, ms *mockServer) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ms.dataListener = l
	_, port, _ := net.SplitHostPort(l.Addr().String())
	ms.handlers["EPSV"] = func(c *textproto.Conn, _ string) {
		_ = c.PrintfLine("229 Entering Extended Passive Mode (|||%s|)", port)
	}
	ms.handlers["NOOP"] = func(c *textproto.Conn, _ string) {
		_ = c.PrintfLine("200 NOOP ok")
	}
	return l
}

func dialMock(t *testing.T, ms *mockServer, opts ...Option) *Client {
	t.Helper()
	ms.start()
	t.Cleanup(ms.stop)
	c, err := Dial(ms.addr, append([]Option{WithTimeout(2 * time.Second)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Quit() })
	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatal(err)
	}
	return c
}

// TestListingReadErrorKeepsRepliesInSync checks that when reading a listing
// fails, the transfer's final reply is still consumed, so the next command
// gets its own reply.
func TestListingReadErrorKeepsRepliesInSync(t *testing.T) {
	t.Parallel()
	tests := []struct {
		cmd  string
		list func(*Client) error
	}{
		{"LIST", func(c *Client) error { _, err := c.List("."); return err }},
		{"NLST", func(c *Client) error { _, err := c.NameList("."); return err }},
		{"MLSD", func(c *Client) error { _, err := c.MLList("."); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			t.Parallel()
			ms := newMockServer(t)
			dl := withEPSV(t, ms)
			// A line longer than bufio.Scanner's 64 KiB limit.
			ms.handlers[tt.cmd] = func(c *textproto.Conn, _ string) {
				_ = c.PrintfLine("150 Here it comes.")
				dc, err := dl.Accept()
				if err != nil {
					return
				}
				_, _ = dc.Write([]byte(strings.Repeat("x", 70<<10) + "\r\n"))
				dc.Close()
				_ = c.PrintfLine("226 Transfer complete.")
			}
			c := dialMock(t, ms)

			if err := tt.list(c); err == nil {
				t.Fatal("listing with an oversized line succeeded, want error")
			}
			resp, err := c.Quote("NOOP")
			if err != nil {
				t.Fatalf("NOOP after failed listing: %v", err)
			}
			if resp.Message != "NOOP ok" {
				t.Errorf("NOOP got reply %d %q: replies are out of sync", resp.Code, resp.Message)
			}
		})
	}
}

// TestActiveModeAbortRace aborts an active-mode download from another
// goroutine. Run with -race.
func TestActiveModeAbortRace(t *testing.T) {
	t.Parallel()
	ms := newMockServer(t)
	var portAddr string
	ms.handlers["PORT"] = func(c *textproto.Conn, args string) {
		p := strings.Split(args, ",")
		if len(p) != 6 {
			_ = c.PrintfLine("501 Bad PORT.")
			return
		}
		hi, _ := strconv.Atoi(p[4])
		lo, _ := strconv.Atoi(p[5])
		portAddr = net.JoinHostPort(strings.Join(p[:4], "."), strconv.Itoa(hi*256+lo))
		_ = c.PrintfLine("200 PORT ok.")
	}
	ms.handlers["RETR"] = func(c *textproto.Conn, _ string) {
		_ = c.PrintfLine("150 Opening data connection.")
		dc, err := net.Dial("tcp", portAddr)
		if err != nil {
			_ = c.PrintfLine("425 Can't open data connection.")
			return
		}
		// Send a little, then stall until the client closes the connection.
		_, _ = dc.Write([]byte("partial"))
		_, _ = io.Copy(io.Discard, dc)
		dc.Close()
		_ = c.PrintfLine("426 Transfer aborted.")
	}
	c := dialMock(t, ms, WithActiveMode())

	done := make(chan error, 1)
	go func() { done <- c.Retrieve("f.bin", io.Discard) }()

	// Sleep instead of waiting on a channel: a channel would order Abort
	// after the accept and hide a race from the race detector.
	time.Sleep(200 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("Retrieve returned before Abort: %v", err)
	default:
	}
	if err := c.Abort(); err != nil {
		t.Errorf("Abort: %v", err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("aborted Retrieve returned nil error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Retrieve did not return after Abort")
	}
}

// TestActiveModeRejectedPortClosesListener checks that a listener opened for
// PORT is closed when the server rejects the command.
func TestActiveModeRejectedPortClosesListener(t *testing.T) {
	// Not parallel: it counts this process's open file descriptors.
	if _, err := os.ReadDir("/proc/self/fd"); err != nil {
		t.Skip("needs /proc/self/fd")
	}
	ms := newMockServer(t)
	ms.handlers["PORT"] = func(c *textproto.Conn, _ string) {
		_ = c.PrintfLine("500 PORT disabled.")
	}
	c := dialMock(t, ms, WithActiveMode())

	countFDs := func() int {
		ents, _ := os.ReadDir("/proc/self/fd")
		return len(ents)
	}
	before := countFDs()
	const attempts = 50
	for range attempts {
		if _, err := c.List("."); err == nil {
			t.Fatal("List succeeded although PORT was rejected")
		}
	}
	if after := countFDs(); after-before >= attempts/2 {
		t.Errorf("open file descriptors grew from %d to %d after %d rejected PORTs", before, after, attempts)
	}
}

// TestKeepAlive checks when keepAlive sends NOOP. It checks for a transfer
// and sends NOOP under one hold of c.mu, so a transfer cannot start between
// the check and the NOOP and have its reply read by the NOOP.
func TestKeepAlive(t *testing.T) {
	t.Parallel()
	ms := newMockServer(t)
	var noops atomic.Int32
	ms.handlers["NOOP"] = func(c *textproto.Conn, _ string) {
		noops.Add(1)
		_ = c.PrintfLine("200 NOOP ok")
	}
	c := dialMock(t, ms)
	c.idleTimeout = time.Hour // set directly: no keep-alive goroutine runs

	setState := func(active net.Conn, lastCommand time.Time) {
		c.mu.Lock()
		c.activeDataConn, c.lastCommand = active, lastCommand
		c.mu.Unlock()
	}
	idle := time.Now().Add(-2 * time.Hour)

	setState(&net.TCPConn{}, idle) // a transfer is running
	if err := c.keepAlive(); err != nil || noops.Load() != 0 {
		t.Errorf("during a transfer: err=%v, NOOPs=%d; want no NOOP", err, noops.Load())
	}
	setState(nil, time.Now()) // not idle
	if err := c.keepAlive(); err != nil || noops.Load() != 0 {
		t.Errorf("not idle: err=%v, NOOPs=%d; want no NOOP", err, noops.Load())
	}
	setState(nil, idle)
	if err := c.keepAlive(); err != nil {
		t.Fatalf("idle: %v", err)
	}
	if n := noops.Load(); n != 1 {
		t.Errorf("idle: NOOPs=%d, want 1", n)
	}
}

func TestWithIdleTimeoutValues(t *testing.T) {
	t.Parallel()
	c := &Client{}
	if err := WithIdleTimeout(-time.Second)(c); err == nil {
		t.Error("WithIdleTimeout(-1s) succeeded, want error")
	}

	// A tiny timeout must not panic (time.NewTicker rejects a zero interval).
	ms := newMockServer(t)
	ms.handlers["NOOP"] = func(c *textproto.Conn, _ string) {
		_ = c.PrintfLine("200 NOOP ok")
	}
	_ = dialMock(t, ms, WithIdleTimeout(time.Nanosecond))
}

func TestParseSTOUName(t *testing.T) {
	t.Parallel()
	tests := []struct{ msg, want string }{
		{"FILE: ftp-123", "ftp-123"},
		{"file: ftp-123", "ftp-123"},
		{"FILE: dir/name with space", "dir/name with space"},
		{"Transfer complete", ""},
		{"FILE:", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := parseSTOUName(tt.msg); got != tt.want {
			t.Errorf("parseSTOUName(%q) = %q, want %q", tt.msg, got, tt.want)
		}
	}
}

func TestProtocolErrorCodeClasses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code                       int
		is2xx, is3xx, is4xx, is5xx bool
	}{
		{199, false, false, false, false},
		{200, true, false, false, false},
		{299, true, false, false, false},
		{300, false, true, false, false},
		{399, false, true, false, false},
		{400, false, false, true, false},
		{499, false, false, true, false},
		{500, false, false, false, true},
		{599, false, false, false, true},
	}
	for _, tt := range tests {
		e := &ProtocolError{Code: tt.code}
		got := fmt.Sprint(e.Is2xx(), e.Is3xx(), e.Is4xx(), e.Is5xx())
		want := fmt.Sprint(tt.is2xx, tt.is3xx, tt.is4xx, tt.is5xx)
		if got != want {
			t.Errorf("ProtocolError{%d}: Is2xx/3xx/4xx/5xx = %s, want %s", tt.code, got, want)
		}
	}
	var pe *ProtocolError
	if !errors.As(fmt.Errorf("wrapped: %w", &ProtocolError{Code: 550}), &pe) || !pe.Is5xx() {
		t.Error("errors.As did not find a wrapped ProtocolError")
	}
}
