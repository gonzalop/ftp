// Package server implements a compliant, flexible FTP server.
//
// # Overview
//
// This package provides a modular FTP server implementation that allows you to:
//   - Embed an FTP server into your Go application
//   - Use custom storage backends (Drivers)
//   - Serve files over IPv4 and IPv6
//   - Support modern FTP extensions
//
// # Getting Started
//
// The easiest way to start is using the provided FSDriver to serve a local directory:
//
//	package main
//
//	import (
//	    "log"
//	    "github.com/gonzalop/ftp/server"
//	)
//
//	func main() {
//	    // Create a driver to serve /tmp/ftp
//	    driver, err := server.NewFSDriver("/tmp/ftp")
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//
//	    // Create the server
//	    s, err := server.NewServer(":21", server.WithDriver(driver))
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//
//	    log.Println("Starting FTP server on :21")
//	    if err := s.ListenAndServe(); err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
// # FTPS Support
//
// The server supports both Explicit (AUTH TLS) and Implicit (legacy) FTPS modes.
//
// Explicit FTPS (RFC 4217, port 21):
//
//	cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")
//	driver, _ := server.NewFSDriver("/tmp/ftp")
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithTLS(&tls.Config{Certificates: []tls.Certificate{cert}}),
//	)
//	s.ListenAndServe()
//
// Implicit FTPS (legacy, port 990):
//
//	cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")
//	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
//	driver, _ := server.NewFSDriver("/tmp/ftp")
//	s, _ := server.NewServer(":990",
//	    server.WithDriver(driver),
//	    server.WithTLS(tlsConfig),
//	)
//	l, _ := net.Listen("tcp", ":990")
//	s.Serve(tls.NewListener(l, tlsConfig))
//
// For development/testing with self-signed certificates:
//
//	// Generate self-signed cert (for testing only!):
//	// openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes
//
//	cert, _ := tls.LoadX509KeyPair("cert.pem", "key.pem")
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithTLS(&tls.Config{
//	        Certificates: []tls.Certificate{cert},
//	        // For production, never set InsecureSkipVerify on the server side.
//	        // Clients connecting to this server will need to either:
//	        // 1. Add the cert to their trust store, or
//	        // 2. Use InsecureSkipVerify on the client side (testing only)
//	    }),
//	)
//
// # Custom Drivers
//
// You can implement the Driver interface to connect the FTP server to any backend,
// such as cloud storage (S3, GCS), an in-memory database, or a custom CMS.
//
// Implement the Driver interface:
//
//	type Driver interface {
//	    Authenticate(user, pass, host string) (ClientContext, error)
//	}
//
// And the ClientContext interface for file operations:
//
//	type ClientContext interface {
//	    ListDir(path string) ([]os.FileInfo, error)
//	    OpenFile(path string, flag int) (io.ReadWriteCloser, error)
//	    GetSettings() *Settings
//	    // ...
//	}
//
// # Authentication Patterns
//
// The server supports flexible authentication through the Driver interface.
//
// Anonymous-only access (default with FSDriver):
//
//	driver, _ := server.NewFSDriver("/tmp/ftp")
//	// Allows "anonymous" and "ftp" users with read-only access
//
// Custom authentication with per-user directories:
//
//	driver, _ := server.NewFSDriver("/tmp/ftp",
//	    server.WithAuthenticator(func(user, pass, host string) (string, bool, error) {
//	        // Validate credentials (e.g., check database)
//	        if !isValidUser(user, pass) {
//	            return "", false, os.ErrPermission
//	        }
//	        // Return user-specific root directory
//	        userRoot := filepath.Join("/tmp/ftp", user)
//	        readOnly := user == "guest"
//	        return userRoot, readOnly, nil
//	    }),
//	)
//
// Disable anonymous access:
//
//	driver, _ := server.NewFSDriver("/tmp/ftp",
//	    server.WithDisableAnonymous(true),
//	    server.WithAuthenticator(func(user, pass, host string) (string, bool, error) {
//	        // Only authenticated users allowed
//	        return validateAndGetUserRoot(user, pass)
//	    }),
//	)
//
// # Passive Mode Configuration
//
// When behind NAT or in containerized environments, configure passive mode settings:
//
//	settings := &server.Settings{
//	    PublicHost:  "ftp.example.com",  // Public IP or hostname
//	    PasvMinPort: 30000,               // Passive port range start
//	    PasvMaxPort: 30100,               // Passive port range end
//	}
//	driver, _ := server.NewFSDriver("/tmp/ftp",
//	    server.WithSettings(settings),
//	)
//
// The PublicHost is advertised to clients in PASV responses. If not set,
// the server uses the control connection's local address.
//
// Port range configuration is essential for firewall rules:
//   - Ensure the range is large enough for concurrent transfers
//   - Configure your firewall to allow incoming connections on this range
//   - Docker users: map the port range with -p 30000-30100:30000-30100
//
// # Server Configuration
//
// Connection limits and timeouts:
//
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithMaxConnections(100),           // Limit concurrent connections
//	    server.WithMaxIdleTime(10*time.Minute),   // Idle timeout
//	)
//
// Custom logging:
//
//	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
//	    Level: slog.LevelDebug,
//	}))
//	s, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithLogger(logger),
//	)
//
// # Troubleshooting
//
// Common issues and solutions:
//
// Problem: Passive mode connections fail
//   - Solution: Set PublicHost in Settings to your public IP/hostname
//   - Solution: Ensure firewall allows passive port range
//   - Solution: For Docker, map passive ports: -p 21:21 -p 30000-30100:30000-30100
//
// Problem: "Permission denied" errors
//   - Solution: Check file system permissions on the root directory
//   - Solution: Verify the user running the server has read/write access
//   - Solution: Review your Authenticator function's readOnly flag
//
// Problem: TLS handshake failures
//   - Solution: Ensure certificate and key files are valid
//   - Solution: Check that clients support the TLS version in your config
//   - Solution: For self-signed certs, clients may need to disable verification
//
// Problem: Connection refused on port 21
//   - Solution: Port 21 requires root/admin privileges on most systems
//   - Solution: Use a higher port (e.g., :2121) for development
//   - Solution: On Linux, use setcap: sudo setcap CAP_NET_BIND_SERVICE=+eip ./server
//
// # RFC Compliance
//
// This server implements the following RFCs:
//   - RFC 959 (Base FTP)
//   - RFC 1123 (Requirements for Internet Hosts - minimum implementation)
//   - RFC 2389 (Feature Negotiation)
//   - RFC 2428 (IPv6 / NAT)
//   - RFC 3659 (Extensions: SIZE, MDTM, MLSD, MLST, REST)
//   - RFC 4217 (Securing FTP with TLS)
//   - RFC 7151 (HOST Command)
//   - draft-somers-ftp-mfxx (MFMT Command)
//   - draft-bryan-ftp-hash (HASH Command)

package server
