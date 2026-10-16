package server

import (
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"strings"

	"github.com/gonzalop/ftp/internal/sharedconn"
)

// handleAUTH handles authentication mechanisms, specifically TLS (RFC 4217).
func (s *session) handleAUTH(arg string) {
	if s.server.tlsConfig == nil {
		s.reply(502, "TLS not configured.")
		return
	}
	if strings.ToUpper(arg) != "TLS" {
		s.reply(504, "Only AUTH TLS is supported.")
		return
	}

	s.reply(234, "AUTH TLS successful.")

	// Upgrade connection
	s.mu.Lock()
	defer s.mu.Unlock()

	// Bytes the client sent after AUTH TLS (the start of its TLS handshake)
	// may already sit in our buffers: first in s.reader, then, later in the
	// stream, in the telnet reader's buffer.
	var prefix []byte
	if n := s.reader.Buffered(); n > 0 {
		b, _ := s.reader.Peek(n)
		prefix = append(prefix, b...)
	}
	if n := s.tnet.reader.Buffered(); n > 0 {
		b, _ := s.tnet.reader.Peek(n)
		prefix = append(prefix, b...)
	}
	conn := net.Conn(s.conn)
	if len(prefix) > 0 {
		conn = &sharedconn.PrefixedConn{
			Conn: s.conn,
			R:    io.MultiReader(bytes.NewReader(prefix), s.conn),
		}
	}

	tlsConn := tls.Server(conn, s.server.tlsConfig)

	// Keep the telnet filter on the encrypted connection, reusing the
	// pooled readers and writer.
	s.conn = tlsConn
	s.tnet.Reset(tlsConn)
	s.reader.Reset(s.tnet)
	s.writer.Reset(tlsConn)
}

func (s *session) handlePROT(arg string) {
	if s.server.tlsConfig == nil {
		s.reply(502, "TLS not configured.")
		return
	}
	// RFC 4217
	// P - Private (TLS)
	// C - Clear (No TLS)
	switch strings.ToUpper(arg) {
	case "P":
		s.prot = "P"
		s.reply(200, "PROT P OK.")
	case "C":
		s.prot = "C"
		s.reply(200, "PROT C OK.")
	default:
		s.reply(504, "PROT not implemented.")
	}
}

func (s *session) handlePBSZ(_ string) {
	if s.server.tlsConfig == nil {
		s.reply(502, "TLS not configured.")
		return
	}
	// We only support buffer size 0.
	// RFC 4217: If the server cannot support the requested size, it should
	// respond with a 200 reply specifying the maximum size it can support.
	s.reply(200, "PBSZ=0")
}
