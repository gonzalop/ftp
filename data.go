package ftp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"sync"
	"time"
)

var (
	// pasvRegex matches the PASV response format: 227 Entering Passive Mode (h1,h2,h3,h4,p1,p2)
	pasvRegex = regexp.MustCompile(`\((\d+),(\d+),(\d+),(\d+),(\d+),(\d+)\)`)

	// epsvRegex matches the EPSV response format: 229 Entering Extended Passive Mode (|||port|)
	epsvRegex = regexp.MustCompile(`\(\|\|\|(\d+)\|\)`)
)

// parsePASV parses a PASV response and returns the host and port.
// Example: "227 Entering Passive Mode (192,168,1,1,195,149)"
// Returns: "192.168.1.1:50069" (195*256 + 149 = 50069)
func parsePASV(response string) (string, error) {
	matches := pasvRegex.FindStringSubmatch(response)
	if len(matches) != 7 {
		return "", fmt.Errorf("invalid PASV response: %s", response)
	}

	// Parse and validate the IP address parts
	var h [4]int
	for i := range 4 {
		val, err := strconv.Atoi(matches[i+1])
		if err != nil || val < 0 || val > 255 {
			return "", fmt.Errorf("invalid PASV IP part: %s", matches[i+1])
		}
		h[i] = val
	}
	host := fmt.Sprintf("%d.%d.%d.%d", h[0], h[1], h[2], h[3])
	if ip := net.ParseIP(host); ip == nil || ip.To4() == nil {
		return "", fmt.Errorf("invalid IPv4 address from PASV: %s", host)
	}

	// Parse and validate the port parts
	p1, err1 := strconv.Atoi(matches[5])
	p2, err2 := strconv.Atoi(matches[6])
	if err1 != nil || err2 != nil || p1 < 0 || p1 > 255 || p2 < 0 || p2 > 255 {
		return "", fmt.Errorf("invalid PASV port parts: %s, %s", matches[5], matches[6])
	}
	port := p1*256 + p2

	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

// parseEPSV parses an EPSV response and returns the port.
// Example: "229 Entering Extended Passive Mode (|||6446|)"
// Returns: "6446"
func parseEPSV(response string) (string, error) {
	matches := epsvRegex.FindStringSubmatch(response)
	if len(matches) != 2 {
		return "", fmt.Errorf("invalid EPSV response: %s", response)
	}

	port, err := strconv.Atoi(matches[1])
	if err != nil || port < 0 || port > 65535 {
		return "", fmt.Errorf("invalid EPSV port: %s", matches[1])
	}

	return matches[1], nil
}

// formatPORT formats an address for the PORT command.
// Converts "192.168.1.100:50000" to "192,168,1,100,195,80"
func formatPORT(addr string) (string, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}

	// Parse IP address
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("invalid IP address: %s", host)
	}
	ip = ip.To4()
	if ip == nil {
		return "", fmt.Errorf("PORT requires IPv4 address")
	}

	// Parse port
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("invalid port: %s", portStr)
	}

	// Format as h1,h2,h3,h4,p1,p2
	p1 := port / 256
	p2 := port % 256

	return fmt.Sprintf("%d,%d,%d,%d,%d,%d", ip[0], ip[1], ip[2], ip[3], p1, p2), nil
}

// resolveDataAddr resolves the data connection address.
// To prevent FTP bounce attacks, we enforce that the data connection host matches
// the control connection host, even if the server returns a different IP in the PASV response.
func resolveDataAddr(pasvAddr, controlHost string) string {
	host, port, err := net.SplitHostPort(pasvAddr)
	if err != nil {
		return pasvAddr
	}

	// If the server returns a different IP than the one we're connected to,
	// we use the control connection host instead. This is a security measure.
	// We allow 0.0.0.0 (which is common) and also check if the hosts differ.
	if host == "0.0.0.0" || host != controlHost {
		// Special case: if controlHost is a hostname that resolves to the same IP,
		// we might be being too strict, but for security, using controlHost is always safer.
		return net.JoinHostPort(controlHost, port)
	}

	return pasvAddr
}

// openDataConn opens a data connection using either active (PORT) or passive (PASV/EPSV) mode.
// If TLS is enabled, the data connection will use TLS with session reuse.
func (c *Client) openDataConn() (net.Conn, error) {
	if c.activeMode {
		return c.openActiveDataConn()
	}
	return c.openPassiveDataConn()
}

// formatEPRT formats an address for the EPRT command.
// Format: |d|net-prt|net-addr|tcp-port|
// d: delimiter (usually |)
// net-prt: 1 for IPv4, 2 for IPv6
// net-addr: IP address string
// tcp-port: port number
func formatEPRT(addr string) (string, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("invalid IP address: %s", host)
	}

	var netPrt int
	if ip.To4() != nil {
		netPrt = 1
	} else if ip.To16() != nil {
		netPrt = 2
	} else {
		return "", fmt.Errorf("unknown IP address family: %s", host)
	}

	return fmt.Sprintf("|%d|%s|%s|", netPrt, host, portStr), nil
}

// openActiveDataConn opens a data connection using active mode (PORT).
// The client listens on a local port and tells the server to connect to it.
func (c *Client) openActiveDataConn() (net.Conn, error) {
	// Get the local IP of the control connection
	localAddr := c.conn.LocalAddr().String()
	host, _, err := net.SplitHostPort(localAddr)
	if err != nil {
		host = "127.0.0.1" // Fallback
	}

	// Listen on a random port on the same interface
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		// Fallback to all interfaces if listening on specific IP fails
		listener, err = net.Listen("tcp", ":0")
		if err != nil {
			return nil, fmt.Errorf("failed to create listener: %w", err)
		}
	}

	// Close the listener on every error return below.
	ok := false
	defer func() {
		if !ok {
			listener.Close()
		}
	}()

	// Get the local address
	addr := listener.Addr().String()

	// Parse local address to determine protocol version
	localHost, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(localHost)
	if ip == nil {
		return nil, fmt.Errorf("failed to parse local IP: %s", localHost)
	}

	var resp *Response
	var cmd string

	// Use EPRT if IPv6, or stick to PORT for IPv4 (unless configured otherwise in future)
	// We could use EPRT for IPv4 too, but PORT is more widely supported by legacy servers.
	if ip.To4() == nil {
		// IPv6 requires EPRT
		cmd = "EPRT"
		eprtCmd, err2 := formatEPRT(addr)
		if err2 != nil {
			return nil, fmt.Errorf("failed to format EPRT command: %w", err2)
		}
		resp, err = c.sendCommand("EPRT", eprtCmd)
	} else {
		// IPv4 uses PORT
		cmd = "PORT"
		portCmd, err2 := formatPORT(addr)
		if err2 != nil {
			return nil, fmt.Errorf("failed to format PORT command: %w", err2)
		}
		resp, err = c.sendCommand("PORT", portCmd)
	}

	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", cmd, err)
	}

	if !resp.Is2xx() {
		return nil, &ProtocolError{
			Command:  cmd,
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	// Accept the connection from the server
	// Note: The actual connection happens after we send the transfer command (RETR, STOR, etc.)
	// So we return a wrapper that will accept when needed
	var serverIP net.IP
	if host, _, err := net.SplitHostPort(c.conn.RemoteAddr().String()); err == nil {
		serverIP = net.ParseIP(host)
	}

	ok = true
	return &activeDataConn{
		listener:  listener,
		tlsConfig: c.tlsConfig,
		timeout:   c.timeout,
		serverIP:  serverIP,
	}, nil
}

// activeDataConn wraps a listener for active mode connections.
//
// The transfer goroutine accepts the connection, while Abort may close it
// from another goroutine, so conn and closed are guarded by mu.
type activeDataConn struct {
	listener  net.Listener
	tlsConfig *tls.Config
	timeout   time.Duration
	serverIP  net.IP // only this peer may connect; nil skips the check

	mu     sync.Mutex
	conn   net.Conn
	closed bool
}

// getConn returns the accepted connection, or nil before accept.
func (a *activeDataConn) getConn() net.Conn {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.conn
}

// setConn stores c as the data connection. It fails, closing c, if Close
// was already called.
func (a *activeDataConn) setConn(c net.Conn) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		c.Close()
		return net.ErrClosed
	}
	a.conn = c
	return nil
}

func (a *activeDataConn) accept() error {
	if a.timeout > 0 {
		if l, ok := a.listener.(*net.TCPListener); ok {
			_ = l.SetDeadline(time.Now().Add(a.timeout))
		}
	}
	// Only the server may connect; anyone else who reaches the port first
	// could inject or receive the data. Drop other peers and keep waiting
	// until the listener deadline.
	var conn net.Conn
	for {
		c, err := a.listener.Accept()
		if err != nil {
			return err
		}
		if a.isServer(c.RemoteAddr()) {
			conn = c
			break
		}
		c.Close()
	}
	// Store the raw connection first, so Close can interrupt the handshake.
	if err := a.setConn(conn); err != nil {
		return err
	}

	// Wrap in TLS if needed. RFC 4217: the FTP client is always the TLS
	// client, also when the server opens the data connection.
	if a.tlsConfig != nil {
		tlsConn := tls.Client(conn, a.tlsConfig)
		// Set deadline for handshake
		if a.timeout > 0 {
			_ = conn.SetDeadline(time.Now().Add(a.timeout))
		}
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return err
		}
		if err := a.setConn(tlsConn); err != nil {
			return err
		}
	}
	return nil
}

// Handshake accepts the server's connection, and runs TLS on it if
// configured, unless that already happened.
func (a *activeDataConn) Handshake() error {
	if a.getConn() != nil {
		return nil
	}
	return a.accept()
}

// isServer reports whether addr is the FTP server. Loopback addresses may
// differ (127.0.0.1 and ::1), as the server allows too.
func (a *activeDataConn) isServer(addr net.Addr) bool {
	if a.serverIP == nil {
		return true
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.Equal(a.serverIP) || (ip.IsLoopback() && a.serverIP.IsLoopback()))
}

// connected returns the data connection, accepting it first if needed.
func (a *activeDataConn) connected() (net.Conn, error) {
	if conn := a.getConn(); conn != nil {
		return conn, nil
	}
	if err := a.accept(); err != nil {
		return nil, err
	}
	return a.getConn(), nil
}

func (a *activeDataConn) Read(p []byte) (n int, err error) {
	conn, err := a.connected()
	if err != nil {
		return 0, err
	}
	if a.timeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(a.timeout))
	}
	return conn.Read(p)
}

func (a *activeDataConn) Write(p []byte) (n int, err error) {
	conn, err := a.connected()
	if err != nil {
		return 0, err
	}
	if a.timeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(a.timeout))
	}
	return conn.Write(p)
}

// Close closes the connection and the listener. Abort and the transfer's
// cleanup may both call it, at the same time; only the first call acts.
func (a *activeDataConn) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	conn := a.conn
	a.mu.Unlock()

	var err1, err2 error
	if conn != nil {
		err1 = conn.Close()
	}
	if a.listener != nil {
		err2 = a.listener.Close()
	}
	if err1 != nil {
		return err1
	}
	return err2
}

func (a *activeDataConn) LocalAddr() net.Addr {
	if conn := a.getConn(); conn != nil {
		return conn.LocalAddr()
	}
	return a.listener.Addr()
}

func (a *activeDataConn) RemoteAddr() net.Addr {
	if conn := a.getConn(); conn != nil {
		return conn.RemoteAddr()
	}
	return nil
}

func (a *activeDataConn) SetDeadline(t time.Time) error {
	if conn := a.getConn(); conn != nil {
		return conn.SetDeadline(t)
	}
	return nil
}

func (a *activeDataConn) SetReadDeadline(t time.Time) error {
	if conn := a.getConn(); conn != nil {
		return conn.SetReadDeadline(t)
	}
	return nil
}

func (a *activeDataConn) SetWriteDeadline(t time.Time) error {
	if conn := a.getConn(); conn != nil {
		return conn.SetWriteDeadline(t)
	}
	return nil
}

// openPassiveDataConn opens a data connection using passive mode (PASV/EPSV).
// This is the default and recommended mode.
func (c *Client) openPassiveDataConn() (net.Conn, error) {
	// Try EPSV first (supports IPv6), fall back to PASV
	var addr string

	// Try EPSV
	if !c.disableEPSV {
		if resp, err := c.sendCommand("EPSV"); err == nil {
			if resp.Code == 502 { // 502 = Not implemented
				c.disableEPSV = true
			} else if resp.Is2xx() {
				port, parseErr := parseEPSV(resp.String())
				if parseErr == nil {
					// Use the same host as the control connection
					addr = net.JoinHostPort(c.host, port)
				}
			}
		}
	}

	// Fall back to PASV if EPSV failed
	if addr == "" {
		resp, err := c.sendCommand("PASV")
		if err != nil {
			return nil, fmt.Errorf("PASV failed: %w", err)
		}

		if !resp.Is2xx() {
			return nil, &ProtocolError{
				Command:  "PASV",
				Response: resp.Message,
				Code:     resp.Code,
			}
		}

		addr, err = parsePASV(resp.String())
		if err != nil {
			return nil, err
		}

		// If the server sends 0.0.0.0, we use the control connection address.
		addr = resolveDataAddr(addr, c.host)
	}

	// Connect to the data port
	var dataConn net.Conn
	var err error

	if c.customDialer != nil {
		// Use custom dialer with context
		ctx := context.Background()
		if c.timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, c.timeout)
			defer cancel()
		}
		dataConn, err = c.customDialer.DialContext(ctx, "tcp", addr)
	} else {
		// Use standard dialer
		dataConn, err = c.dialer.Dial("tcp", addr)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to data port: %w", err)
	}

	// Wrap with deadline connection if timeout is set
	if c.timeout > 0 {
		dataConn = &deadlineConn{Conn: dataConn, timeout: c.timeout}
	}

	// If TLS is enabled, wrap the data connection. The handshake runs in
	// cmdDataConnFrom after the server's 1xx reply: servers start TLS on the
	// data connection only once they have the transfer command.
	if c.tlsConfig != nil {
		dataConn = tls.Client(dataConn, c.tlsConfig)
	}

	return dataConn, nil
}

// cmdDataConnFrom executes a command that requires a data connection.
// It opens the data connection, sends the command, and returns the response and data connection.
// The caller is responsible for closing the data connection and reading the final response.
func (c *Client) cmdDataConnFrom(cmd string, args ...string) (*Response, net.Conn, error) {
	// Open the data connection first
	dataConn, err := c.openDataConn()
	if err != nil {
		return nil, nil, err
	}

	// Mark transfer as in progress and track the connection
	c.mu.Lock()
	c.activeDataConn = dataConn
	c.mu.Unlock()

	// Send the command
	resp, err := c.sendCommand(cmd, args...)
	if err != nil {
		dataConn.Close()
		c.mu.Lock()
		c.activeDataConn = nil
		c.mu.Unlock()
		return nil, nil, err
	}

	// A data transfer starts with a preliminary (1xx) reply. Any other reply
	// means the server will not use the data connection; waiting for a
	// completion reply after a 2xx or 3xx would hang until the timeout.
	if resp.Code < 100 || resp.Code >= 200 {
		dataConn.Close()
		c.mu.Lock()
		c.activeDataConn = nil
		c.mu.Unlock()
		return resp, nil, &ProtocolError{
			Command:  cmd,
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	// Start TLS on the data connection now that the server expects it.
	if h, ok := dataConn.(interface{ Handshake() error }); ok {
		if err := h.Handshake(); err != nil {
			// Read the server's reply to the failed transfer to stay in sync.
			_ = c.finishDataConn(dataConn)
			return resp, nil, fmt.Errorf("data connection TLS handshake failed: %w", err)
		}
	}

	return resp, dataConn, nil
}

// finishDataConn closes the data connection and reads the final response.
// This should be called after the data transfer is complete.
func (c *Client) finishDataConn(dataConn net.Conn) error {
	// Close the data connection. Abort may already have closed it.
	closeErr := dataConn.Close()
	if errors.Is(closeErr, net.ErrClosed) {
		closeErr = nil
	}

	// Hold c.mu while reading, so the reply cannot be taken by a concurrent
	// command such as the keep-alive NOOP.
	c.mu.Lock()
	defer c.mu.Unlock()

	// Mark transfer as complete
	c.activeDataConn = nil

	// Set read deadline for the final response
	if c.timeout > 0 {
		if err := c.conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}
	}

	// Read the final response (should be 226 Transfer complete)
	resp, err := readResponse(c.reader)
	if err != nil {
		return fmt.Errorf("failed to read completion response: %w", err)
	}

	if c.logger != nil {
		c.logger.Debug("ftp data transfer complete", "code", resp.Code, "message", resp.Message)
	}

	if !resp.Is2xx() {
		return &ProtocolError{
			Command:  "DATA_TRANSFER",
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	if closeErr != nil {
		return fmt.Errorf("failed to close data connection: %w", closeErr)
	}

	return nil
}
