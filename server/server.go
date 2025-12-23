package server

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"sync/atomic"
	"time"
)

// Server is the FTP server.
//
// It handles listening for incoming connections and dispatching them to
// client sessions. Each connection runs in its own goroutine.
//
// Lifecycle:
//  1. Create server with NewServer()
//  2. Start with ListenAndServe() or Serve()
//  3. Server runs until an error occurs or the listener is closed
//  4. For graceful shutdown, close the listener from another goroutine
//
// Basic example:
//
//	driver, _ := server.NewFSDriver("/tmp/ftp")
//	s, err := server.NewServer(":21", server.WithDriver(driver))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	log.Fatal(s.ListenAndServe())
//
// With graceful shutdown:
//
//	ln, _ := net.Listen("tcp", ":21")
//	go func() {
//	    <-shutdownChan
//	    ln.Close() // Stops accepting new connections
//	}()
//	s.Serve(ln)
type Server struct {
	// addr is the TCP address to listen on (e.g., ":21").
	addr string

	// driver is the backend driver for authentication and file operations.
	driver Driver

	// logger is the logger instance.
	logger *slog.Logger

	// tlsConfig is the TLS configuration for FTPS.
	// If nil, TLS is disabled.
	tlsConfig *tls.Config

	// disableMLSD disables the MLSD command (for compatibility testing).
	disableMLSD bool

	// maxIdleTime is the maximum time a connection can be idle before being closed.
	// Defaults to 5 minutes.
	maxIdleTime time.Duration

	// maxConnections is the maximum number of simultaneous connections.
	// If 0, there is no limit.
	maxConnections int

	// activeConns tracks the number of currently active connections.
	activeConns atomic.Int32
}

// NewServer creates a new FTP server with the given address and options.
// The address should be in the form ":port" or "host:port".
// The driver must be provided via the WithDriver option.
//
// Default values:
//   - Logger: slog.Default()
//   - MaxIdleTime: 5 minutes
//   - MaxConnections: 0 (unlimited)
//   - TLS: disabled
//
// Basic example:
//
//	driver, _ := server.NewFSDriver("/tmp/ftp")
//	s, err := server.NewServer(":21", server.WithDriver(driver))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// With TLS (Explicit FTPS):
//
//	cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")
//	tlsConfig := &tls.Config{
//	    Certificates: []tls.Certificate{cert},
//	    MinVersion:   tls.VersionTLS12,
//	}
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithTLS(tlsConfig),
//	)
//
// With connection limits:
//
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithMaxConnections(100),
//	    server.WithMaxIdleTime(10*time.Minute),
//	)
func NewServer(addr string, options ...Option) (*Server, error) {
	s := &Server{
		addr:        addr,
		logger:      slog.Default(),
		maxIdleTime: 5 * time.Minute,
	}

	// Apply options
	for _, opt := range options {
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	// Validate required fields
	if s.driver == nil {
		return nil, fmt.Errorf("driver is required (use WithDriver option)")
	}

	return s, nil
}

// ListenAndServe starts the FTP server on the configured address.
// It blocks until the server stops or an error occurs.
//
// This is a convenience method that creates a TCP listener and calls Serve().
// For more control (e.g., graceful shutdown), use net.Listen() and Serve() directly.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}

	s.logger.Info("FTP server listening", "addr", s.addr)
	return s.Serve(ln)
}

// Serve accepts incoming connections on the listener l.
// It blocks until the listener is closed or an error occurs.
//
// Each connection is handled in a separate goroutine. The server enforces
// connection limits (if configured) and idle timeouts.
//
// For graceful shutdown, close the listener from another goroutine:
//
//	ln, _ := net.Listen("tcp", ":21")
//	go func() {
//	    <-ctx.Done()
//	    ln.Close()
//	}()
//	s.Serve(ln)
func (s *Server) Serve(l net.Listener) error {
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			s.logger.Error("accept error", "error", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

// handleConnection handles a new client connection.
func (s *Server) handleConnection(conn net.Conn) {
	// Create a new session for this connection
	s.handleSession(conn)
}

// handleSession handles a new client connection.
func (s *Server) handleSession(conn net.Conn) {
	if s.maxConnections > 0 && s.activeConns.Load() >= int32(s.maxConnections) {
		// Send 421 service not available
		fmt.Fprintf(conn, "421 Too many users, sorry.\r\n")
		conn.Close()
		return
	}

	s.activeConns.Add(1)
	defer s.activeConns.Add(-1)

	session := newSession(s, conn)
	session.serve()
}
