package ftp

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"time"
)

// Option is a functional option for configuring an FTP client.
type Option func(*Client) error

// WithTimeout sets the timeout for connection and operations.
// This applies to both the initial connection and subsequent read/write operations.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) error {
		c.timeout = timeout
		return nil
	}
}

// WithIdleTimeout sets the maximum idle time before sending NOOP keep-alive.
// If the connection is idle for longer than this duration, a NOOP command
// will be sent automatically to prevent the server from closing the connection.
//
// This is useful for long-running operations or when keeping a connection
// open for extended periods. Set to 0 to disable automatic keep-alive.
//
// Example:
//
//	client, _ := ftp.Dial("ftp.example.com:21",
//	    ftp.WithIdleTimeout(5*time.Minute),
//	)
func WithIdleTimeout(timeout time.Duration) Option {
	return func(c *Client) error {
		c.idleTimeout = timeout
		return nil
	}
}

// WithExplicitTLS enables explicit TLS mode (AUTH TLS).
// The client connects on the standard FTP port (21) and upgrades to TLS
// using the AUTH TLS command. This is the recommended mode for FTPS.
//
// The provided tls.Config should include the ServerName for certificate validation.
// A ClientSessionCache will be automatically added if not present to enable
// TLS session reuse for data connections.
func WithExplicitTLS(config *tls.Config) Option {
	return func(c *Client) error {
		if c.tlsMode == tlsModeImplicit {
			return fmt.Errorf("explicit TLS cannot be combined with implicit TLS")
		}
		if config == nil {
			config = &tls.Config{}
		}
		// Ensure we have a session cache for TLS session reuse
		if config.ClientSessionCache == nil {
			config.ClientSessionCache = tls.NewLRUClientSessionCache(0)
		}
		c.tlsConfig = config
		c.tlsMode = tlsModeExplicit
		return nil
	}
}

// WithImplicitTLS enables implicit TLS mode.
// The client connects directly with TLS, typically on port 990.
// This is a legacy mode but still used by some servers.
//
// The provided tls.Config should include the ServerName for certificate validation.
// A ClientSessionCache will be automatically added if not present to enable
// TLS session reuse for data connections.
func WithImplicitTLS(config *tls.Config) Option {
	return func(c *Client) error {
		if c.tlsMode == tlsModeExplicit {
			return fmt.Errorf("implicit TLS cannot be combined with explicit TLS")
		}
		if config == nil {
			config = &tls.Config{}
		}
		// Ensure we have a session cache for TLS session reuse
		if config.ClientSessionCache == nil {
			config.ClientSessionCache = tls.NewLRUClientSessionCache(0)
		}
		c.tlsConfig = config
		c.tlsMode = tlsModeImplicit
		return nil
	}
}

// WithLogger enables debug logging using the provided logger.
// All FTP commands and responses will be logged at debug level.
//
// Example:
//
//	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
//	    Level: slog.LevelDebug,
//	}))
//	client, _ := ftp.Dial("ftp.example.com:21", ftp.WithLogger(logger))
func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) error {
		c.logger = logger
		return nil
	}
}

// WithDialer sets a custom net.Dialer for establishing connections.
// This can be used to configure source addresses, keep-alive settings, etc.
func WithDialer(dialer *net.Dialer) Option {
	return func(c *Client) error {
		c.dialer = dialer
		return nil
	}
}

// tlsMode represents the TLS mode for the connection.
type tlsMode int

const (
	tlsModeNone tlsMode = iota
	tlsModeExplicit
	tlsModeImplicit
)

// WithActiveMode enables active mode (PORT) instead of passive mode (PASV/EPSV).
// In active mode, the client opens a port and tells the server to connect to it.
// This is less common than passive mode and may not work behind NAT/firewalls.
//
// Note: Most users should use passive mode (the default). Active mode is mainly
// useful for servers behind firewalls that allow outbound connections.
func WithActiveMode() Option {
	return func(c *Client) error {
		c.activeMode = true
		return nil
	}
}

// WithDisableEPSV disables the use of the EPSV command.
// By default, the client tries EPSV before falling back to PASV.
// This option forces the client to use PASV directly, which can be useful
// for servers that don't support EPSV correctly or are behind firewalls
// that block EPSV.
func WithDisableEPSV() Option {
	return func(c *Client) error {
		c.disableEPSV = true
		return nil
	}
}

// WithCustomListParser adds a custom directory listing parser.
// Custom parsers are tried before the built-in parsers (EPLF, DOS, Unix).
// This allows handling non-standard LIST formats.
func WithCustomListParser(parser ListingParser) Option {
	return func(c *Client) error {
		// Prepend the custom parser so it has priority
		c.parsers = append([]ListingParser{parser}, c.parsers...)
		return nil
	}
}
