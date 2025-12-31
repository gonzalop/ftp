// Package common provides a wrapper that makes quic.Stream implement net.Conn.
// This allows QUIC streams to be used as drop-in replacements for TCP connections
// in the FTP client and server implementations.
package common

import (
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

// QuicConn wraps a quic.Stream to implement the net.Conn interface.
// This allows QUIC streams to be used anywhere a net.Conn is expected,
// enabling the existing FTP client and server code to work over QUIC
// without modification.
type QuicConn struct {
	stream *quic.Stream
	conn   *quic.Conn // Parent connection for address information
}

// NewQuicConn creates a new QuicConn wrapping the given stream.
func NewQuicConn(stream *quic.Stream, conn *quic.Conn) *QuicConn {
	return &QuicConn{
		stream: stream,
		conn:   conn,
	}
}

// Read reads data from the stream.
func (q *QuicConn) Read(b []byte) (int, error) {
	return q.stream.Read(b)
}

// Write writes data to the stream.
func (q *QuicConn) Write(b []byte) (int, error) {
	return q.stream.Write(b)
}

// Close closes the stream.
func (q *QuicConn) Close() error {
	return q.stream.Close()
}

// LocalAddr returns the local network address from the parent QUIC connection.
func (q *QuicConn) LocalAddr() net.Addr {
	return q.conn.LocalAddr()
}

// RemoteAddr returns the remote network address from the parent QUIC connection.
func (q *QuicConn) RemoteAddr() net.Addr {
	return q.conn.RemoteAddr()
}

// SetDeadline sets the read and write deadlines for the stream.
func (q *QuicConn) SetDeadline(t time.Time) error {
	return q.stream.SetDeadline(t)
}

// SetReadDeadline sets the read deadline for the stream.
func (q *QuicConn) SetReadDeadline(t time.Time) error {
	return q.stream.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline for the stream.
func (q *QuicConn) SetWriteDeadline(t time.Time) error {
	return q.stream.SetWriteDeadline(t)
}

// Ensure QuicConn implements net.Conn at compile time.
var _ net.Conn = (*QuicConn)(nil)
