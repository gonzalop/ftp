package server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"sync"
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

	// welcomeMessage is the banner sent to clients on connection.
	// Defaults to "220 FTP Server Ready".
	welcomeMessage string

	// serverName is the system type returned by the SYST command.
	// Defaults to "UNIX Type: L8".
	serverName string

	// maxIdleTime is the maximum time a connection can be idle before being closed.
	// Defaults to 5 minutes.
	maxIdleTime time.Duration

	// readTimeout is the deadline for read operations on connections.
	// If 0, no timeout is applied.
	readTimeout time.Duration

	// writeTimeout is the deadline for write operations on connections.
	// If 0, no timeout is applied.
	writeTimeout time.Duration

	// maxConnections is the maximum number of simultaneous connections.
	// If 0, there is no limit.
	maxConnections int

	// maxConnectionsPerIP is the maximum number of simultaneous connections per IP.
	// If 0, there is no per-IP limit.
	maxConnectionsPerIP int

	// activeConns tracks the number of currently active connections.
	activeConns atomic.Int32

	// connsByIP tracks the number of active connections per IP address.
	connsByIP   map[string]int32
	connsByIPMu sync.Mutex

	// Shutdown handling
	mu         sync.Mutex
	listener   net.Listener
	conns      map[net.Conn]struct{}
	inShutdown atomic.Bool
}

// ErrServerClosed is returned by the Server's Serve, ServeTLS, ListenAndServe,
// and ListenAndServeTLS methods after a call to Shutdown or Close.
var ErrServerClosed = errors.New("ftp: Server closed")

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
//	    server.WithMaxConnections(100, 10), // Max 100 total, 10 per IP
//	    server.WithMaxIdleTime(10*time.Minute),
//	)
func NewServer(addr string, options ...Option) (*Server, error) {
	s := &Server{
		addr:           addr,
		logger:         slog.Default(),
		welcomeMessage: "220 FTP Server Ready",
		serverName:     "UNIX Type: L8",
		maxIdleTime:    5 * time.Minute,
		conns:          make(map[net.Conn]struct{}),
		connsByIP:      make(map[string]int32),
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

// Shutdown stops the server.
//
// It closes the listener and immediately closes all active connections.
func (s *Server) Shutdown() error {
	s.inShutdown.Store(true)

	s.mu.Lock()
	ln := s.listener
	s.listener = nil
	s.mu.Unlock()

	var err error
	if ln != nil {
		err = ln.Close()
	}

	// Close all active connections (control and data)
	s.mu.Lock()
	conns := s.conns
	s.conns = make(map[net.Conn]struct{})
	s.mu.Unlock()

	for conn := range maps.Keys(conns) {
		conn.Close()
	}

	return err
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
	s.mu.Lock()
	if s.inShutdown.Load() {
		s.mu.Unlock()
		l.Close()
		return ErrServerClosed
	}
	s.listener = l
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if s.listener == l {
			s.listener = nil
		}
		s.mu.Unlock()
		l.Close()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			if s.inShutdown.Load() {
				return ErrServerClosed
			}
			s.logger.Error("accept error", "error", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

// handleConnection handles a new client connection.
func (s *Server) handleConnection(conn net.Conn) {
	if !s.trackConnection(conn, true) {
		conn.Close()
		return
	}
	defer s.trackConnection(conn, false)

	// Create a new session for this connection
	s.handleSession(conn)
}

// trackConnection returns false if we're shutting down.
func (s *Server) trackConnection(conn net.Conn, add bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.inShutdown.Load() {
		conn.Close()
		return false
	}

	if add {
		s.conns[conn] = struct{}{}

		// Track per-IP for data connections
		if s.maxConnectionsPerIP > 0 {
			remoteAddr := conn.RemoteAddr().String()
			ip, _, err := net.SplitHostPort(remoteAddr)
			if err != nil {
				ip = remoteAddr
			}

			s.connsByIPMu.Lock()
			s.connsByIP[ip]++
			s.connsByIPMu.Unlock()
		}
		return true
	}
	// remove
	delete(s.conns, conn)

	// Untrack per-IP for data connections
	if s.maxConnectionsPerIP > 0 {
		remoteAddr := conn.RemoteAddr().String()
		ip, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			ip = remoteAddr
		}

		s.connsByIPMu.Lock()
		s.connsByIP[ip]--
		if s.connsByIP[ip] <= 0 {
			delete(s.connsByIP, ip)
		}
		s.connsByIPMu.Unlock()
	}
	return true
}

// trackingConn wraps a net.Conn to track its lifetime in the server.
type trackingConn struct {
	net.Conn
	server *Server
}

func (c *trackingConn) Close() error {
	c.server.trackConnection(c.Conn, false)
	return c.Conn.Close()
}

// handleSession handles a new client connection.
func (s *Server) handleSession(conn net.Conn) {
	// Check global connection limit
	if s.maxConnections > 0 && s.activeConns.Load() >= int32(s.maxConnections) {
		// Security audit: connection limit reached
		remoteAddr := conn.RemoteAddr().String()
		ip, _, _ := net.SplitHostPort(remoteAddr)
		s.logger.Warn("connection_rejected",
			"remote_ip", ip,
			"reason", "global_limit_reached",
			"limit", s.maxConnections,
		)
		// Send 421 service not available
		fmt.Fprintf(conn, "421 Too many users, sorry.\r\n")
		conn.Close()
		return
	}

	// Check per-IP connection limit
	if s.maxConnectionsPerIP > 0 {
		// Extract IP address (remove port)
		remoteAddr := conn.RemoteAddr().String()
		ip, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			// If we can't parse the address, use the whole thing
			ip = remoteAddr
		}

		s.connsByIPMu.Lock()
		currentCount := s.connsByIP[ip]
		if currentCount >= int32(s.maxConnectionsPerIP) {
			s.connsByIPMu.Unlock()
			// Security audit: per-IP connection limit reached
			s.logger.Warn("connection_rejected",
				"remote_ip", ip,
				"reason", "per_ip_limit_reached",
				"limit", s.maxConnectionsPerIP,
			)
			fmt.Fprintf(conn, "421 Too many connections from your IP address.\r\n")
			conn.Close()
			return
		}
		s.connsByIPMu.Unlock()
	}

	s.activeConns.Add(1)
	defer s.activeConns.Add(-1)

	session := newSession(s, conn)
	session.serve()
}
