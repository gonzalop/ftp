package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gonzalop/ftp/internal/ratelimit"
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

	// nextPassivePort tracks the last used passive port to implement round-robin selection.
	nextPassivePort int32

	// Privacy-aware logging
	pathRedactor PathRedactor // Custom path redaction function (optional)
	redactIPs    bool         // Redact last octet of IP addresses in logs

	// Features
	enableDirMessage bool // Enable directory messages (.message files)

	// Metrics collection (optional)
	metricsCollector MetricsCollector

	// Shutdown handling
	mu         sync.Mutex
	listener   net.Listener
	conns      map[net.Conn]struct{}
	inShutdown atomic.Bool

	// Transfer logging (xferlog standard format)
	transferLog io.Writer

	// Bandwidth limiting
	bandwidthLimitGlobal  int64              // bytes per second, 0 = unlimited
	bandwidthLimitPerUser int64              // bytes per second, 0 = unlimited
	globalLimiter         *ratelimit.Limiter // shared across all users
}

// transferBufferPool is a pool of byte slices used for data transfers to reduce allocations.
var transferBufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 32*1024)
		return &buf
	},
}

// copyWithPooledBuffer copies from src to dst using a buffer from the pool.
func copyWithPooledBuffer(dst io.Writer, src io.Reader) (int64, error) {
	pbuf := transferBufferPool.Get().(*[]byte)
	defer transferBufferPool.Put(pbuf)
	return io.CopyBuffer(dst, src, *pbuf)
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

	// Initialize global rate limiter if bandwidth limit is set
	if s.bandwidthLimitGlobal > 0 {
		s.globalLimiter = ratelimit.New(s.bandwidthLimitGlobal)
	}

	return s, nil
}

// ListenAndServe acts as a high-level helper to start a simple filesystem-based FTP server.
// It creates an FSDriver rooted at rootPath and starts the server on addr.
//
// Defaults:
//   - Anonymous login allowed (read-only)
//   - Standard timeouts
//
// Example:
//
//	log.Fatal(server.ListenAndServe(":21", "/var/ftp"))
func ListenAndServe(addr string, rootPath string, options ...Option) error {
	driver, err := NewFSDriver(rootPath)
	if err != nil {
		return fmt.Errorf("failed to create driver: %w", err)
	}

	// Prepend the driver option so it can be overridden if needed (though unlikely for this helper)
	opts := append([]Option{WithDriver(driver)}, options...)

	s, err := NewServer(addr, opts...)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	return s.ListenAndServe()
}

// redactPath applies custom path redaction if configured.
func (s *Server) redactPath(path string) string {
	if s.pathRedactor == nil {
		return path
	}
	return s.pathRedactor(path)
}

// redactIP redacts the last octet of an IP address for privacy.
// Example: "192.168.1.100" -> "192.168.1.xxx"
// Example: "2001:db8::1" -> "2001:db8::xxx"
func (s *Server) redactIP(ip string) string {
	if !s.redactIPs || ip == "" {
		return ip
	}

	// Handle IPv4
	if strings.Contains(ip, ".") {
		parts := strings.Split(ip, ".")
		if len(parts) == 4 {
			parts[3] = "xxx"
			return strings.Join(parts, ".")
		}
	}

	// Handle IPv6
	if strings.Contains(ip, ":") {
		// Simple approach: replace everything after last colon
		lastColon := strings.LastIndex(ip, ":")
		if lastColon > 0 {
			return ip[:lastColon+1] + "xxx"
		}
	}

	return ip
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

// Shutdown gracefully stops the server.
//
// It immediately stops accepting new connections by closing the listener,
// then waits for active connections to finish or until the context is cancelled.
//
// If the context expires before all connections close, remaining connections
// are forcibly closed. Forcibly closing a connection will also cause any
// active data transfer for that session to be aborted.
//
// Example with timeout:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	if err := s.Shutdown(ctx); err != nil {
//	    log.Printf("Shutdown error: %v", err)
//	}
func (s *Server) Shutdown(ctx context.Context) error {
	s.inShutdown.Store(true)

	// Close the listener to stop accepting new connections
	s.mu.Lock()
	ln := s.listener
	s.listener = nil
	s.mu.Unlock()

	var err error
	if ln != nil {
		err = ln.Close()
	}

	// Wait for active connections to finish or context to expire
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if s.activeConns.Load() == 0 {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	select {
	case <-done:
		// All connections finished gracefully
		return err
	case <-ctx.Done():
		// Context expired, force close remaining connections
		s.mu.Lock()
		conns := s.conns
		s.conns = make(map[net.Conn]struct{})
		s.mu.Unlock()

		for conn := range maps.Keys(conns) {
			conn.Close()
		}

		if err != nil {
			return err
		}
		return ctx.Err()
	}
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
		// Metrics collection
		if s.metricsCollector != nil {
			s.metricsCollector.RecordConnection(false, "global_limit_reached")
		}
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
		if currentCount > int32(s.maxConnectionsPerIP) {
			s.connsByIPMu.Unlock()
			// Security audit: per-IP connection limit reached
			s.logger.Warn("connection_rejected",
				"remote_ip", ip,
				"reason", "per_ip_limit_reached",
				"limit", s.maxConnectionsPerIP,
			)
			// Metrics collection
			if s.metricsCollector != nil {
				s.metricsCollector.RecordConnection(false, "per_ip_limit_reached")
			}
			fmt.Fprintf(conn, "421 Too many connections from your IP address.\r\n")
			conn.Close()
			return
		}
		s.connsByIPMu.Unlock()
	}

	s.activeConns.Add(1)
	defer s.activeConns.Add(-1)

	// Metrics collection: connection accepted
	if s.metricsCollector != nil {
		s.metricsCollector.RecordConnection(true, "accepted")
	}

	session := newSession(s, conn)
	session.serve()
}
