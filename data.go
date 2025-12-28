package ftp

import (
	"crypto/tls"
	"fmt"
	"net"
	"regexp"
	"strconv"
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
// If the PASV response contains 0.0.0.0, it replaces it with the control connection host.
func resolveDataAddr(pasvAddr, controlHost string) string {
	host, port, err := net.SplitHostPort(pasvAddr)
	if err != nil {
		// If we can't split it, return as is (dialer will likely fail later)
		return pasvAddr
	}

	if host == "0.0.0.0" {
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
	return &activeDataConn{
		listener:  listener,
		tlsConfig: c.tlsConfig,
		timeout:   c.timeout,
	}, nil
}

// activeDataConn wraps a listener for active mode connections.
type activeDataConn struct {
	listener  net.Listener
	conn      net.Conn
	tlsConfig *tls.Config
	timeout   time.Duration
}

func (a *activeDataConn) accept() error {
	if a.timeout > 0 {
		if l, ok := a.listener.(*net.TCPListener); ok {
			_ = l.SetDeadline(time.Now().Add(a.timeout))
		}
	}
	c, err := a.listener.Accept()
	if err != nil {
		return err
	}
	a.conn = c

	// Wrap in TLS if needed
	if a.tlsConfig != nil {
		tlsConn := tls.Server(a.conn, a.tlsConfig)
		// Set deadline for handshake
		if a.timeout > 0 {
			_ = a.conn.SetDeadline(time.Now().Add(a.timeout))
		}
		if err := tlsConn.Handshake(); err != nil {
			a.conn.Close()
			return err
		}
		a.conn = tlsConn
	}
	return nil
}

func (a *activeDataConn) Read(p []byte) (n int, err error) {
	if a.conn == nil {
		if err := a.accept(); err != nil {
			return 0, err
		}
	}
	if a.timeout > 0 {
		_ = a.conn.SetReadDeadline(time.Now().Add(a.timeout))
	}
	return a.conn.Read(p)
}

func (a *activeDataConn) Write(p []byte) (n int, err error) {
	if a.conn == nil {
		if err := a.accept(); err != nil {
			return 0, err
		}
	}
	if a.timeout > 0 {
		_ = a.conn.SetWriteDeadline(time.Now().Add(a.timeout))
	}
	return a.conn.Write(p)
}

func (a *activeDataConn) Close() error {
	var err1, err2 error
	if a.conn != nil {
		err1 = a.conn.Close()
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
	if a.conn != nil {
		return a.conn.LocalAddr()
	}
	return a.listener.Addr()
}

func (a *activeDataConn) RemoteAddr() net.Addr {
	if a.conn != nil {
		return a.conn.RemoteAddr()
	}
	return nil
}

func (a *activeDataConn) SetDeadline(t time.Time) error {
	if a.conn != nil {
		return a.conn.SetDeadline(t)
	}
	return nil
}

func (a *activeDataConn) SetReadDeadline(t time.Time) error {
	if a.conn != nil {
		return a.conn.SetReadDeadline(t)
	}
	return nil
}

func (a *activeDataConn) SetWriteDeadline(t time.Time) error {
	if a.conn != nil {
		return a.conn.SetWriteDeadline(t)
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
	dataConn, err := c.dialer.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to data port: %w", err)
	}

	// If TLS is enabled, wrap the data connection
	if c.tlsConfig != nil {
		tlsConn := tls.Client(dataConn, c.tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			dataConn.Close()
			return nil, fmt.Errorf("data connection TLS handshake failed: %w", err)
		}
		dataConn = tlsConn
	}

	// Wrap with deadline connection if timeout is set
	if c.timeout > 0 {
		return &deadlineConn{Conn: dataConn, timeout: c.timeout}, nil
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

	// Check for preliminary success (1xx) or immediate success (2xx)
	if !resp.Is2xx() && !resp.Is3xx() && resp.Code < 100 || resp.Code >= 200 {
		// For data transfer commands, we expect:
		// - 1xx (preliminary positive) - transfer starting
		// - 2xx (positive completion) - already done
		// We'll be lenient and accept both
		if resp.Code < 100 || resp.Code >= 400 {
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
	}

	return resp, dataConn, nil
}

// finishDataConn closes the data connection and reads the final response.
// This should be called after the data transfer is complete.
func (c *Client) finishDataConn(dataConn net.Conn) error {
	// Close the data connection
	if err := dataConn.Close(); err != nil {
		return fmt.Errorf("failed to close data connection: %w", err)
	}

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

	// Mark transfer as complete
	c.mu.Lock()
	c.activeDataConn = nil
	c.mu.Unlock()

	if !resp.Is2xx() {
		return &ProtocolError{
			Command:  "DATA_TRANSFER",
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	return nil
}
