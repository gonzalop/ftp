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
// The first parameter (max) sets the global limit across all clients.
// The second parameter (maxPerIP) sets the per-IP limit.
// If either is 0, that limit is disabled.
//
// When a limit is reached, new connections receive a "421 Too many users" response.
// Both control and data connections count toward these limits.
//
// Example:
//
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithMaxConnections(100, 10), // Max 100 total, 10 per IP
//	)
func WithMaxConnections(max, maxPerIP int) Option {
	return func(s *Server) error {
		s.maxConnections = max
		s.maxConnectionsPerIP = maxPerIP
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

// WithWelcomeMessage sets a custom welcome banner sent to clients on connection.
// If not specified, defaults to "220 FTP Server Ready".
//
// The message should be a complete FTP response. If it doesn't start with "220",
// it will be prepended automatically.
//
// Example:
//
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithWelcomeMessage("220 Welcome to My FTP Server"),
//	)
func WithWelcomeMessage(message string) Option {
	return func(s *Server) error {
		s.welcomeMessage = message
		return nil
	}
}

// WithServerName sets the system type returned by the SYST command.
// If not specified, defaults to "UNIX Type: L8".
//
// Common values:
//   - "UNIX Type: L8" (default)
//   - "Windows_NT"
//   - "MACOS"
//
// Example:
//
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithServerName("Windows_NT"),
//	)
func WithServerName(name string) Option {
	return func(s *Server) error {
		s.serverName = name
		return nil
	}
}

// WithReadTimeout sets the deadline for read operations on connections.
// If 0 (default), no timeout is applied.
//
// This prevents slow-read attacks and helps detect network issues.
// The timeout is reset after each successful read operation.
//
// Example:
//
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithReadTimeout(30*time.Second),
//	)
func WithReadTimeout(duration time.Duration) Option {
	return func(s *Server) error {
		s.readTimeout = duration
		return nil
	}
}

// WithWriteTimeout sets the deadline for write operations on connections.
// If 0 (default), no timeout is applied.
//
// This prevents hanging on slow clients and helps detect network issues.
// The timeout is reset after each successful write operation.
//
// Example:
//
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithWriteTimeout(30*time.Second),
//	)
func WithWriteTimeout(duration time.Duration) Option {
	return func(s *Server) error {
		s.writeTimeout = duration
		return nil
	}
}

// WithPathRedactor sets a custom path redaction function for privacy compliance.
// The function will be called for every path logged, allowing custom redaction logic.
//
// Example - Redact middle components:
//
//	server.WithPathRedactor(func(path string) string {
//	    parts := strings.Split(path, "/")
//	    if len(parts) > 3 {
//	        for i := 2; i < len(parts)-1; i++ {
//	            parts[i] = "*"
//	        }
//	    }
//	    return strings.Join(parts, "/")
//	})
//
// Example - Redact specific patterns:
//
//	server.WithPathRedactor(func(path string) string {
//	    return regexp.MustCompile(`/users/[^/]+/`).ReplaceAllString(path, "/users/*/")
//	})
func WithPathRedactor(redactor PathRedactor) Option {
	return func(s *Server) error {
		s.pathRedactor = redactor
		return nil
	}
}

// WithRedactIPs enables IP address redaction in logs for privacy compliance.
// When enabled, the last octet of IPv4 addresses is replaced with "xxx".
//
// Example: "192.168.1.100" becomes "192.168.1.xxx"
//
// This helps comply with GDPR and other privacy regulations while maintaining
// enough information for network troubleshooting.
func WithRedactIPs(enabled bool) Option {
	return func(s *Server) error {
		s.redactIPs = enabled
		return nil
	}
}

// WithEnableDirMessage enables directory messages.
// When enabled, the server will check for a .message file in the directory
// upon entering it and display its content to the user.
func WithEnableDirMessage(enabled bool) Option {
	return func(s *Server) error {
		s.enableDirMessage = enabled
		return nil
	}
}

// WithMetricsCollector sets an optional metrics collector for monitoring.
// The collector will receive metrics about commands, transfers, connections,
// and authentication attempts.
//
// Example:
//
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithMetricsCollector(myPrometheusCollector),
//	)
func WithMetricsCollector(collector MetricsCollector) Option {
	return func(s *Server) error {
		s.metricsCollector = collector
		return nil
	}
}
