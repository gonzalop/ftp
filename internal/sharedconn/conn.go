package sharedconn

import (
	"io"
	"net"
)

// PrefixedConn wraps a net.Conn and an io.Reader.
// It reads from the reader until it's exhausted, then falls back to the connection.
// This is used to preserve data buffered in a bufio.Reader when upgrading to TLS.
type PrefixedConn struct {
	net.Conn
	R io.Reader
}

func (c *PrefixedConn) Read(p []byte) (int, error) {
	return c.R.Read(p)
}
