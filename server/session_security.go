package server

import (
	"bufio"
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
	conn := net.Conn(s.conn)
	s.mu.Lock()
	if s.reader.Buffered() > 0 {
		conn = &sharedconn.PrefixedConn{
			Conn: s.conn,
			R:    io.MultiReader(io.LimitReader(s.reader, int64(s.reader.Buffered())), s.conn),
		}
	}
	s.mu.Unlock()

	tlsConn := tls.Server(conn, s.server.tlsConfig)

	s.mu.Lock()
	s.conn = tlsConn
	s.reader = bufio.NewReader(tlsConn)
	s.writer = bufio.NewWriter(tlsConn)
	s.mu.Unlock()
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
