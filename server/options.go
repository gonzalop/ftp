package server

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"time"
)

// Option is a functional option for configuring an FTP server.
type Option func(*Server) error

// WithDriver sets the backend driver for authentication and file operations.
// This option is required and can only be set once.
//
// Example:
//
//	driver, _ := server.NewFSDriver("/tmp/ftp")
//	s, _ := server.NewServer(":21", server.WithDriver(driver))
func WithDriver(driver Driver) Option {
	return func(s *Server) error {
		if s.driver != nil {
			return fmt.Errorf("driver already set")
		}
		s.driver = driver
		return nil
	}
}

// WithTLS enables TLS (FTPS) with the provided configuration.
// Supports both Explicit FTPS (AUTH TLS) and Implicit FTPS.
//
// For Explicit FTPS (recommended, port 21):
//
//	cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithTLS(&tls.Config{
//	        Certificates: []tls.Certificate{cert},
//	        MinVersion:   tls.VersionTLS12,
//	    }),
//	)
//
// For Implicit FTPS (legacy, port 990):
//
//	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
//	ln, _ := tls.Listen("tcp", ":990", tlsConfig)
//	s.Serve(ln)
func WithTLS(config *tls.Config) Option {
	return func(s *Server) error {
		s.tlsConfig = config
		return nil
	}
}

// WithLogger sets a custom logger for the server.
// If not specified, slog.Default() is used.
//
// Example with debug logging:
//
//	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
//	    Level: slog.LevelDebug,
//	}))
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithLogger(logger),
//	)
func WithLogger(logger *slog.Logger) Option {
	return func(s *Server) error {
		s.logger = logger
		return nil
	}
}

// WithMaxIdleTime sets the maximum time a connection can be idle before being closed.
// If not specified, defaults to 5 minutes.
//
// Example:
//
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithMaxIdleTime(10*time.Minute),
//	)
func WithMaxIdleTime(duration time.Duration) Option {
	return func(s *Server) error {
		s.maxIdleTime = duration
		return nil
	}
}

// WithMaxConnections sets the maximum number of simultaneous connections.
// If 0, there is no limit. This is the default.
//
// When the limit is reached, new connections receive a "421 Too many users" response.
//
// Example:
//
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithMaxConnections(100),
//	)
func WithMaxConnections(max int) Option {
	return func(s *Server) error {
		s.maxConnections = max
		return nil
	}
}

// WithDisableMLSD disables the MLSD command.
// This is primarily useful for compatibility testing with legacy clients.
//
// Most users should not need this option. MLSD is a modern, standardized
// directory listing command (RFC 3659) that provides more reliable parsing
// than the legacy LIST command.
func WithDisableMLSD(disable bool) Option {
	return func(s *Server) error {
		s.disableMLSD = disable
		return nil
	}
}
