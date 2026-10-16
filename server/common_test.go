package server

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func fatalIfErr(t *testing.T, err error, format string, args ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf(format+": %v", append(args, err)...)
	}
}

// startRawServer starts a server on 127.0.0.1 and returns it with its address.
// The server is shut down when the test ends.
func startRawServer(t *testing.T, driver Driver, opts ...Option) (*Server, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	fatalIfErr(t, err, "listen")
	srv, err := NewServer(ln.Addr().String(), append([]Option{WithDriver(driver)}, opts...)...)
	fatalIfErr(t, err, "NewServer")
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return srv, ln.Addr().String()
}

// rawConn is a plain-text control connection for tests that check exact replies.
type rawConn struct {
	t    *testing.T
	conn net.Conn
	r    *bufio.Reader
}

// dialRaw connects to addr and reads the greeting.
func dialRaw(t *testing.T, addr string) *rawConn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	fatalIfErr(t, err, "dial")
	t.Cleanup(func() { conn.Close() })
	c := &rawConn{t: t, conn: conn, r: bufio.NewReader(conn)}
	c.reply()
	return c
}

// reply reads one (possibly multi-line) reply and returns its last line.
func (c *rawConn) reply() string {
	c.t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		line, err := c.r.ReadString('\n')
		fatalIfErr(c.t, err, "read reply")
		line = strings.TrimRight(line, "\r\n")
		if len(line) >= 4 && line[3] == ' ' {
			return line
		}
	}
}

// cmd sends one command line and returns the last line of its reply.
func (c *rawConn) cmd(format string, args ...any) string {
	c.t.Helper()
	_, err := fmt.Fprintf(c.conn, format+"\r\n", args...)
	fatalIfErr(c.t, err, "send command")
	return c.reply()
}

// epsv sends EPSV and dials the returned data port.
func (c *rawConn) epsv() net.Conn {
	c.t.Helper()
	l := c.cmd("EPSV")
	start, end := strings.Index(l, "|||"), strings.LastIndex(l, "|")
	if start < 0 || end <= start+3 {
		c.t.Fatalf("bad EPSV reply: %q", l)
	}
	dc, err := net.Dial("tcp", "127.0.0.1:"+l[start+3:end])
	fatalIfErr(c.t, err, "dial data port")
	c.t.Cleanup(func() { dc.Close() })
	return dc
}
