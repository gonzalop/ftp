package ftp

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Client represents an FTP client connection.
type Client struct {
	// conn is the underlying network connection (control channel)
	conn net.Conn

	// reader is a buffered reader for the control channel
	reader *bufio.Reader

	// tlsConfig is the TLS configuration (if TLS is enabled)
	tlsConfig *tls.Config

	// tlsMode indicates whether TLS is disabled, explicit, or implicit
	tlsMode tlsMode

	// timeout is the timeout for operations
	timeout time.Duration

	// idleTimeout is the maximum time to wait before sending NOOP to keep connection alive
	// If zero, no automatic keep-alive is performed
	idleTimeout time.Duration

	// logger is used for debug logging
	logger *slog.Logger

	// dialer is used to establish connections
	dialer *net.Dialer

	// host and port for the connection
	host string
	port string

	// features stores the server's advertised features from FEAT command
	features map[string]string

	// activeMode indicates whether to use active (PORT) or passive (PASV/EPSV) mode
	activeMode bool

	// disableEPSV disables the use of EPSV command, forcing PASV default
	disableEPSV bool

	// parsers stores the list of directory listing parsers
	parsers []ListingParser

	// currentType tracks the current transfer type to avoid redundant TYPE commands
	currentType string

	// mu protects concurrency-sensitive fields
	mu sync.Mutex

	// lastCommand tracks the time of the last command sent
	lastCommand time.Time

	// quitChan signals the keep-alive goroutine to stop
	quitChan chan struct{}

	// activeDataConn tracks the currently active data connection
	activeDataConn net.Conn
}

// Dial connects to an FTP server at the given address.
// The address should be in the form "host:port".
//
// Example:
//
//	client, err := ftp.Dial("ftp.example.com:21")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Quit()
//
// Example with Explicit TLS:
//
//	tlsConfig := &tls.Config{
//	    ServerName: "ftp.example.com",
//	}
//	client, err := ftp.Dial("ftp.example.com:21", ftp.WithExplicitTLS(tlsConfig))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Quit()
//
// Example with Implicit TLS and self-signed certificate (InsecureSkipVerify):
//
//	tlsConfig := &tls.Config{
//	    InsecureSkipVerify: true,
//	}
//	client, err := ftp.Dial("ftp.example.com:990", ftp.WithImplicitTLS(tlsConfig))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Quit()
func Dial(addr string, options ...Option) (*Client, error) {
	// Parse the address
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	// Create the client with defaults
	c := &Client{
		host:    host,
		port:    port,
		timeout: 30 * time.Second,
		tlsMode: tlsModeNone,
		dialer:  &net.Dialer{},
		logger:  slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError + 1})), // No-op logger by default
		parsers: []ListingParser{
			&EPLFParser{},
			&DOSParser{},
			&UnixParser{},
		},
	}

	// Apply options
	for _, opt := range options {
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	// Set dialer timeout
	c.dialer.Timeout = c.timeout

	// Establish the connection
	if err := c.connect(); err != nil {
		return nil, err
	}

	// Initialize last command time
	c.lastCommand = time.Now()

	// Start keep-alive loop if enabled
	c.startKeepAlive()

	return c, nil
}

// startKeepAlive starts a goroutine that sends NOOP commands
// if the connection has been idle for the configured idleTimeout.
func (c *Client) startKeepAlive() {
	if c.idleTimeout == 0 {
		return
	}

	c.quitChan = make(chan struct{})

	// We use a ticker that runs at half the idle timeout to be safe
	ticker := time.NewTicker(c.idleTimeout / 2)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Skip if a data transfer is in progress
				c.mu.Lock()
				transferring := c.activeDataConn != nil
				c.mu.Unlock()
				if transferring {
					continue
				}

				c.mu.Lock()
				last := c.lastCommand
				c.mu.Unlock()

				// If time since last command is greater than idle timeout, send NOOP
				if time.Since(last) >= c.idleTimeout {
					if c.logger != nil {
						c.logger.Debug("sending keep-alive NOOP")
					}
					// Ignore errors (connection might be closed)
					_ = c.Noop()
				}
			case <-c.quitChan:
				return
			}
		}
	}()
}

// Connect connects to an FTP server using a URL.
// Supported schemes: "ftp", "ftps" (implicit), "ftp+explicit" (explicit TLS).
// Format: scheme://[user:password@]host[:port][/path]
//
// Examples:
//
//	ftp://ftp.example.com
//	ftp://user:pass@ftp.example.com:2121
//	ftps://ftp.example.com (Implicit TLS, port 990)
//	ftp+explicit://ftp.example.com (Explicit TLS, port 21)
func Connect(urlStr string) (*Client, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Determine port and TLS mode based on scheme
	var port string
	var options []Option
	host := u.Hostname()
	port = u.Port()

	switch strings.ToLower(u.Scheme) {
	case "ftp":
		if port == "" {
			port = "21"
		}
	case "ftps":
		if port == "" {
			port = "990"
		}
		// Use implicit TLS with default settings (server verification enabled)
		// Users who need custom TLS configs (e.g. self-signed) should use Dial instead.
		options = append(options, WithImplicitTLS(&tls.Config{ServerName: host}))
	case "ftp+explicit":
		if port == "" {
			port = "21"
		}
		options = append(options, WithExplicitTLS(&tls.Config{ServerName: host}))
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	addr := net.JoinHostPort(host, port)
	c, err := Dial(addr, options...)
	if err != nil {
		return nil, err
	}

	// Login if credentials are provided, otherwise default to anonymous
	user := u.User.Username()
	pass, hasPass := u.User.Password()

	if user == "" {
		user = "anonymous"
		pass = "anonymous@"
	} else if !hasPass {
		pass = ""
	}

	if err := c.Login(user, pass); err != nil {
		_ = c.Quit()
		return nil, fmt.Errorf("login failed: %w", err)
	}

	// Change directory if path is provided
	if u.Path != "" && u.Path != "/" {
		if err := c.ChangeDir(u.Path); err != nil {
			_ = c.Quit()
			return nil, fmt.Errorf("failed to change directory: %w", err)
		}
	}

	return c, nil
}

// connect establishes the control connection and handles the initial handshake.
func (c *Client) connect() error {
	var err error

	addr := net.JoinHostPort(c.host, c.port)
	c.logger.Debug("connecting to ftp server", "addr", addr, "tls_mode", c.tlsMode)

	// For implicit TLS, wrap the connection immediately
	if c.tlsMode == tlsModeImplicit {
		conn, err := c.dialer.Dial("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}

		// Wrap in TLS
		c.logger.Debug("starting TLS handshake", "mode", "implicit")
		tlsConn := tls.Client(conn, c.tlsConfig)

		// Set deadline for handshake
		if c.timeout > 0 {
			if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
				conn.Close()
				return fmt.Errorf("failed to set deadline: %w", err)
			}
		}

		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return fmt.Errorf("TLS handshake failed: %w", err)
		}
		c.logger.Debug("TLS handshake complete", "mode", "implicit")

		c.conn = tlsConn
	} else {
		// Plain connection or explicit TLS
		c.conn, err = c.dialer.Dial("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	}

	// Set up buffered reader
	c.reader = bufio.NewReader(c.conn)

	// Set read deadline for greeting
	if c.timeout > 0 {
		if err := c.conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
			c.conn.Close()
			return fmt.Errorf("failed to set read deadline: %w", err)
		}
	}

	// Read the greeting (220 response)
	resp, err := readResponse(c.reader)
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to read greeting: %w", err)
	}

	if c.logger != nil {
		c.logger.Debug("ftp greeting", "code", resp.Code, "message", resp.Message)
	}

	if resp.Code != 220 {
		c.conn.Close()
		return &ProtocolError{
			Command:  "CONNECT",
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	// For explicit TLS, upgrade the connection now
	if c.tlsMode == tlsModeExplicit {
		if err := c.upgradeToTLS(); err != nil {
			c.conn.Close()
			return err
		}
	}

	return nil
}

// upgradeToTLS upgrades the connection to TLS using AUTH TLS.
func (c *Client) upgradeToTLS() error {
	// Send AUTH TLS
	resp, err := c.sendCommand("AUTH", "TLS")
	if err != nil {
		return fmt.Errorf("AUTH TLS failed: %w", err)
	}

	if resp.Code != 234 {
		return &ProtocolError{
			Command:  "AUTH TLS",
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	// Wrap the connection in TLS
	c.logger.Debug("starting TLS handshake", "mode", "explicit")
	tlsConn := tls.Client(c.conn, c.tlsConfig)

	// Set deadline for handshake
	if c.timeout > 0 {
		if err := c.conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
			return fmt.Errorf("failed to set deadline: %w", err)
		}
	}

	if err := tlsConn.Handshake(); err != nil {
		return fmt.Errorf("TLS handshake failed: %w", err)
	}
	c.logger.Debug("TLS handshake complete", "mode", "explicit")

	c.conn = tlsConn
	c.reader = bufio.NewReader(c.conn)

	// Send PBSZ 0 (required for TLS)
	if _, err := c.expectCode(200, "PBSZ", "0"); err != nil {
		return fmt.Errorf("PBSZ failed: %w", err)
	}

	// Send PROT P (protect data channel)
	if _, err := c.expectCode(200, "PROT", "P"); err != nil {
		return fmt.Errorf("PROT failed: %w", err)
	}

	return nil
}

// Login authenticates with the FTP server using the provided username and password.
func (c *Client) Login(username, password string) error {
	// Send USER command
	resp, err := c.sendCommand("USER", username)
	if err != nil {
		return err
	}

	// If we get 230, we're already logged in (no password required)
	if resp.Code == 230 {
		return nil
	}

	// If we get 331, we need to send the password
	if resp.Code != 331 {
		return &ProtocolError{
			Command:  "USER",
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	// Send PASS command
	if _, err := c.expectCode(230, "PASS", password); err != nil {
		return err
	}

	return nil
}

// Quit closes the connection gracefully by sending the QUIT command.
// If a file transfer is in progress, it will be aborted by closing the data connection.
func (c *Client) Quit() error {
	if c.conn == nil {
		return nil
	}

	// Stop keep-alive loop
	if c.quitChan != nil {
		close(c.quitChan)
	}

	// Abort active transfer if any
	c.mu.Lock()
	if c.activeDataConn != nil {
		c.activeDataConn.Close()
		c.activeDataConn = nil
	}
	c.mu.Unlock()

	// Send QUIT command (ignore errors, we're closing anyway)
	_, _ = c.sendCommand("QUIT")

	// Close the connection
	return c.conn.Close()
}

// Host sends the HOST command to the server.
// This implements RFC 7151 - File Transfer Protocol HOST Command for Virtual Hosts.
// It must be sent before the USER command.
//
// Example:
//
//	if err := client.Host("ftp.example.com"); err != nil {
//	    log.Fatal(err)
//	}
func (c *Client) Host(host string) error {
	_, err := c.expect2xx("HOST", host)
	return err
}

// Type sets the transfer type (e.g., "A", "I").
func (c *Client) Type(transferType string) error {
	// Skip if already set to this type
	if c.currentType == transferType {
		c.logger.Debug("transfer type already set, skipping TYPE command", "type", transferType)
		return nil
	}

	_, err := c.expectCode(200, "TYPE", transferType)
	if err != nil {
		return err
	}

	// Track the current type
	c.currentType = transferType
	return nil
}

// Features queries the server for supported features using the FEAT command.
// Returns a map of feature names to their parameters (if any).
// This implements RFC 2389 - Feature negotiation mechanism for FTP.
//
// Example:
//
//	feats, err := client.Features()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if _, ok := feats["UTF8"]; ok {
//	    fmt.Println("Server supports UTF8")
//	}
func (c *Client) Features() (map[string]string, error) {
	// If we've already fetched features, return cached version
	if c.features != nil {
		return c.features, nil
	}

	resp, err := c.sendCommand("FEAT")
	if err != nil {
		return nil, err
	}

	if resp.Code != 211 {
		return nil, &ProtocolError{
			Command:  "FEAT",
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	// Parse features from multi-line response
	c.features = parseFeatureLines(resp.Lines)
	return c.features, nil
}

// Syst returns the system type of the server using the SYST command.
//
// Example:
//
//	syst, err := client.Syst()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Server system type: %s\n", syst)
func (c *Client) Syst() (string, error) {
	resp, err := c.expect2xx("SYST")
	if err != nil {
		return "", err
	}

	return resp.Message, nil
}

// parseFeatureLines parses the lines of a FEAT response.
// Supports both formats:
// - RFC 2389: "211-Features:\r\n FEAT1\r\n FEAT2 params\r\n211 End"
// - Traditional: "211-Features\r\n211-FEAT1\r\n211-FEAT2 params\r\n211 End"
func parseFeatureLines(lines []string) map[string]string {
	features := make(map[string]string)
	for _, line := range lines {
		var featureLine string

		// Handle RFC 2389 format: lines starting with space
		if len(line) > 0 && line[0] == ' ' {
			featureLine = strings.TrimSpace(line)
		} else if len(line) >= 4 && (line[3] == '-' || line[3] == ' ') {
			// Skip status lines (e.g., "211-Features:" or "211 End")
			continue
		} else {
			// Skip any other malformed lines
			continue
		}

		if featureLine == "" {
			continue
		}

		// Split feature name and parameters
		parts := strings.SplitN(featureLine, " ", 2)
		featName := strings.ToUpper(parts[0])
		featParams := ""
		if len(parts) > 1 {
			featParams = parts[1]
		}

		features[featName] = featParams
	}
	return features
}

// HasFeature checks if the server supports a specific feature.
// This is a convenience method that calls Features() if needed.
func (c *Client) HasFeature(feature string) bool {
	feats, err := c.Features()
	if err != nil {
		return false
	}
	_, ok := feats[strings.ToUpper(feature)]
	return ok
}

// SetOption sets an option for a feature using the OPTS command.
// This implements RFC 2389 - Feature negotiation mechanism for FTP.
//
// Example:
//
//	err := client.SetOption("UTF8", "ON")
func (c *Client) SetOption(option, value string) error {
	_, err := c.expect2xx("OPTS", option, value)
	return err
}

// Noop sends a NOOP (no operation) command to the server.
// This is useful as a keepalive to prevent the connection from timing out
// during long operations or idle periods.
//
// Example:
//
//	// Keep connection alive during long processing
//	for processing {
//	    // ... do work ...
//	    client.Noop()  // Prevent timeout
//	    time.Sleep(30 * time.Second)
//	}
func (c *Client) Noop() error {
	_, err := c.expect2xx("NOOP")
	return err
}

// Quote sends a raw command to the server and returns the response.
// This allows sending commands that are not explicitly supported by the client.
//
// Example:
//
//	resp, err := client.Quote("SITE", "CHMOD", "755", "script.sh")
func (c *Client) Quote(command string, args ...string) (*Response, error) {
	return c.sendCommand(command, args...)
}

// Abort cancels an active file transfer.
// It sends the ABOR command to the server if there's an ongoing transfer.
func (c *Client) Abort() error {
	c.mu.Lock()
	hasTransfer := c.activeDataConn != nil
	c.mu.Unlock()

	if !hasTransfer {
		return fmt.Errorf("(local) No transfer in progress")
	}

	_, err := c.expect2xx("ABOR")
	return err
}

// Hash requests the hash of a file from the server using the HASH command.
// This implements draft-bryan-ftp-hash.
//
// The algorithm used is determined by the server's default or the last
// algorithm selected via SetHashAlgo.
//
// Example:
//
//	hash, err := client.Hash("file.iso")
func (c *Client) Hash(path string) (string, error) {
	resp, err := c.sendCommand("HASH", path)
	if err != nil {
		return "", err
	}

	if resp.Code != 213 {
		return "", &ProtocolError{
			Command:  "HASH",
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	// Parse response in format: "213 <algorithm> <hash value> <filename>"
	// Some servers may return variations like "<algorithm> <hash value> <filename>"
	parts := strings.Fields(resp.Message)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid HASH response: %s", resp.Message)
	}

	// The hash is expected to be the second field (parts[1]) if the format is "ALGO HASH PATH"
	return parts[1], nil
}

// SetHashAlgo selects the hash algorithm to use for the HASH command.
// Supported algorithms depend on the server (typically SHA-1, SHA-256, MD5, CRC32).
// This uses the OPTS HASH command.
//
// Example:
//
//	err := client.SetHashAlgo("SHA-256")
func (c *Client) SetHashAlgo(algo string) error {
	_, err := c.expect2xx("OPTS", "HASH", algo)
	return err
}

// UploadFile manages the upload of a local file to the server.
// It opens the local file and streams it to the remote location using Store.
//
// Example:
//
//	err := client.UploadFile("local_image.jpg", "/public/images/remote_image.jpg")
func (c *Client) UploadFile(localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer f.Close()

	if err := c.Store(remotePath, f); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	return nil
}

// DownloadFile manages the download of a remote file to the local filesystem.
// It creates or truncates the local file and streams the remote content into it using Retrieve.
//
// Example:
//
//	err := client.DownloadFile("/public/data.csv", "local_data.csv")
func (c *Client) DownloadFile(remotePath, localPath string) error {
	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer f.Close()

	if err := c.Retrieve(remotePath, f); err != nil {
		// Clean up the partial file on error
		_ = os.Remove(localPath)
		return fmt.Errorf("download failed: %w", err)
	}

	return nil
}
