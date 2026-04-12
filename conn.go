package ftp

import (
	"io"
	"net"
	"time"
)

// deadlineConn wraps a net.Conn and sets a read/write deadline before every operation.
type deadlineConn struct {
	net.Conn
	timeout time.Duration
}

func (c *deadlineConn) Read(b []byte) (n int, err error) {
	if c.timeout > 0 {
		if err := c.Conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
			return 0, err
		}
	}
	return c.Conn.Read(b)
}

func (c *deadlineConn) Write(b []byte) (n int, err error) {
	if c.timeout > 0 {
		if err := c.Conn.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
			return 0, err
		}
	}
	return c.Conn.Write(b)
}

// prefixedConn wraps a net.Conn and an io.Reader.
// It reads from the reader until it's exhausted, then falls back to the connection.
// This is used to preserve data buffered in a bufio.Reader when upgrading to TLS.
type prefixedConn struct {
	net.Conn
	r io.Reader
}

func (c *prefixedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}
