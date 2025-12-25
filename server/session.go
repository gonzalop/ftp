package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MaxCommandLength is the maximum length of a command line.
const MaxCommandLength = 4096

// session represents an FTP client session.
type session struct {
	server *Server
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	mu     sync.Mutex // Protects writer and state

	// Session tracking
	sessionID string
	remoteIP  string

	// State
	isLoggedIn    bool
	user          string
	renameFrom    string // For RNFR/RNTO
	fs            ClientContext
	restartOffset int64  // For REST command
	host          string // From HOST command
	selectedHash  string // Default SHA-256
	transferType  string // Transfer type (A=ASCII, I=Binary), default I

	// Background transfer state
	busy           bool
	transferCtx    context.Context
	transferCancel context.CancelFunc

	// Reader synchronization
	cmdReqChan chan struct{}

	// Data connection state
	dataConn   net.Conn
	pasvList   net.Listener
	activeIP   string
	activePort int
	prot       string // PROT P or C

	// Cache for PASV IP resolution
	lastPublicHost string
	resolvedIP     net.IP
}

// validateActiveIP ensures the data connection target matches the control connection source.
// This prevents FTP bounce attacks.
func (s *session) validateActiveIP(ip net.IP) bool {
	remoteAddr := s.conn.RemoteAddr().String()
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr // Fallback
	}

	remoteIP := net.ParseIP(host)
	if remoteIP == nil {
		return false
	}

	return ip.Equal(remoteIP)
}

// generateSessionID generates a unique 8-character session ID.
func generateSessionID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%08x", b)
}

// redactPath returns the path with redaction applied if enabled.
func (s *session) redactPath(path string) string {
	return s.server.redactPath(path)
}

// redactIP returns the IP with redaction applied if enabled.
func (s *session) redactIP(ip string) string {
	return s.server.redactIP(ip)
}

// newSession creates a new session.
func newSession(server *Server, conn net.Conn) *session {
	// Generate unique session ID
	sessionID := generateSessionID()

	// Extract remote IP
	remoteAddr := conn.RemoteAddr().String()
	remoteIP, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		remoteIP = remoteAddr // Fallback to full address
	}

	s := &session{
		server:       server,
		conn:         conn,
		reader:       bufio.NewReader(newTelnetReader(conn)),
		writer:       bufio.NewWriter(conn),
		sessionID:    sessionID,
		remoteIP:     remoteIP,
		prot:         "C", // Default to clear
		selectedHash: "SHA-256",
		transferType: "I",
		cmdReqChan:   make(chan struct{}),
	}

	// Detect Implicit TLS (connection is already a *tls.Conn)
	if _, ok := conn.(*tls.Conn); ok {
		s.prot = "P" // Default to private for implicit TLS
	}

	return s
}

type command struct {
	line string
	err  error
}

// serve handles the FTP session. It uses a concurrent architecture to handle
// commands and data transfers, enabling support for commands like ABOR.
//
// Concurrency Model:
//
//  1. Reader Goroutine: A dedicated goroutine is spawned to read commands from
//     the client's control connection. It sends each command to the main `serve`
//     loop via the `cmdChan`.
//
//  2. Main Loop (`serve`): This loop receives commands from `cmdChan` and
//     dispatches them to handlers. It is the single point of control for the
//     session's state.
//
//  3. Synchronization (`cmdReqChan`): To prevent data races during connection
//     upgrades (e.g., AUTH TLS), the reader goroutine waits for a signal on
//     `cmdReqChan` before reading the next command. The main loop sends this
//     signal only after the current command handler has finished. This ensures
//     that handlers that modify the connection or reader/writer state (like
//     `handleAUTH`) can do so safely.
//
//  4. Asynchronous Transfers: Data transfer commands (RETR, STOR, etc.) are
//     handled asynchronously. They start a new goroutine for the actual data
//     copy, set a `busy` flag on the session, and return immediately. This allows
//     the main loop to process other commands, specifically ABOR and STAT.
//
//  5. Aborting Transfers (`ABOR`): If a transfer is in progress (`busy == true`),
//     the `handleABOR` command can interrupt it by closing the data connection and
//     canceling the `transferCtx`. The background transfer goroutine detects
//     this and exits gracefully.
//
//  6. State Protection (`s.mu`): A mutex protects session fields that are accessed
//     by multiple goroutines (e.g., `writer`, `conn`, `reader`, `busy`). This is
//     crucial because the main loop, reader goroutine, and transfer goroutines
//     all interact with the session's state.
//
//  7. Goroutine Cleanup (`done`): A `done` channel is created in `serve` and
//     closed on exit. The reader goroutine selects on this channel to ensure it
//     terminates when the session ends, preventing goroutine leaks.
func (s *session) serve() {
	defer s.close()

	// Send welcome message
	if strings.HasPrefix(s.server.welcomeMessage, "220 ") {
		// Message already has code, send raw
		s.mu.Lock()
		fmt.Fprintf(s.writer, "%s\r\n", s.server.welcomeMessage)
		s.writer.Flush()
		s.mu.Unlock()
	} else if strings.HasPrefix(s.server.welcomeMessage, "220") {
		// Has code but no space, add it
		s.mu.Lock()
		fmt.Fprintf(s.writer, "220 %s\r\n", s.server.welcomeMessage[3:])
		s.writer.Flush()
		s.mu.Unlock()
	} else {
		// No code, use reply
		s.reply(220, s.server.welcomeMessage)
	}

	done := make(chan struct{})
	defer close(done)

	cmdChan := make(chan command)
	go func() {
		defer close(cmdChan)
		for {
			// Apply read timeout (for command reading)
			s.mu.Lock()
			conn := s.conn
			s.mu.Unlock()

			if s.server.readTimeout > 0 {
				_ = conn.SetReadDeadline(time.Now().Add(s.server.readTimeout))
			} else if s.server.maxIdleTime > 0 {
				_ = conn.SetReadDeadline(time.Now().Add(s.server.maxIdleTime))
			}

			line, err := s.readCommand()

			select {
			case cmdChan <- command{line, err}:
			case <-done:
				return
			}

			if err != nil {
				return
			}

			// Wait for next request signal
			select {
			case <-s.cmdReqChan:
			case <-done:
				return
			}
		}
	}()

	for {
		cmd, ok := <-cmdChan
		if !ok {
			return
		}

		if cmd.err != nil {
			if cmd.err != io.EOF && cmd.err.Error() != "command too long" {
				s.server.logger.Warn("read error",
					"session_id", s.sessionID,
					"remote_ip", s.remoteIP,
					"user", s.user,
					"error", cmd.err,
				)
			}
			if cmd.err.Error() == "command too long" {
				s.reply(500, "Command line too long.")
			}
			return
		}

		// Clear read deadline
		_ = s.conn.SetReadDeadline(time.Time{})

		// Apply write timeout (for response writing)
		if s.server.writeTimeout > 0 {
			_ = s.conn.SetWriteDeadline(time.Now().Add(s.server.writeTimeout))
		}

		s.handleCommand(cmd.line)

		// Clear write deadline
		if s.server.writeTimeout > 0 {
			_ = s.conn.SetWriteDeadline(time.Time{})
		}

		// Signal reader to continue
		// This must happen AFTER handleCommand, so any AUTH TLS changes are applied.
		select {
		case s.cmdReqChan <- struct{}{}:
		case <-time.After(1 * time.Second):
			// Should not happen unless reader died
		}
	}
}

// readCommand reads a line from the reader with a limit.
func (s *session) readCommand() (string, error) {
	var line []byte
	for {
		// Protect reader access (needed because reader might be swapped by AUTH TLS)
		s.mu.Lock()
		r := s.reader
		s.mu.Unlock()

		b, err := r.ReadByte()
		if err != nil {
			return string(line), err
		}

		if len(line) >= MaxCommandLength {
			return "", fmt.Errorf("command too long")
		}

		if b == '\n' {
			return string(line), nil
		}
		line = append(line, b)
	}
}

// close closes the session and underlying connection.
func (s *session) close() {
	if s.fs != nil {
		s.fs.Close()
	}
	if s.pasvList != nil {
		s.pasvList.Close()
	}
	if s.dataConn != nil {
		s.dataConn.Close()
	}
	s.conn.Close()
	s.server.logger.Debug("session closed",
		"session_id", s.sessionID,
		"remote_ip", s.remoteIP,
		"user", s.user,
	)
}

// handleCommand parses and dispatches a command.
func (s *session) handleCommand(line string) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return
	}

	parts := strings.SplitN(line, " ", 2)
	cmd := strings.ToUpper(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}

	logArg := arg
	if cmd == "PASS" {
		logArg = "***"
	}
	s.server.logger.Debug("command received",
		"session_id", s.sessionID,
		"remote_ip", s.remoteIP,
		"user", s.user,
		"cmd", cmd,
		"arg", logArg,
	)

	s.mu.Lock()
	busy := s.busy
	s.mu.Unlock()

	if busy && cmd != "ABOR" && cmd != "STAT" {
		s.reply(503, "Transfer in progress, please ABOR or wait.")
		return
	}

	var err error
	switch cmd {
	// Access Control
	case "USER", "PASS", "QUIT":
		err = s.handleAccessCommand(cmd, arg)

	// File Management
	case "CWD", "CDUP", "UP", "PWD", "LIST", "NLST", "MKD", "XMKD", "RMD", "XRMD", "DELE", "RNFR", "RNTO":
		s.handleFileCommand(cmd, arg)

	// File Transfer
	case "RETR", "STOR", "APPE", "STOU":
		s.handleTransferCommand(cmd, arg)

	// Transfer Parameters
	case "TYPE", "PORT", "PASV", "EPSV", "EPRT", "REST":
		s.handleParamCommand(cmd, arg)

	// Information
	case "SIZE", "MDTM", "FEAT", "OPTS", "MLSD", "MLST", "NOOP":
		s.handleInfoCommand(cmd, arg)

	// Security
	case "AUTH", "PROT", "PBSZ":
		s.handleSecurityCommand(cmd, arg)

	// RFC 1123 Compliance
	case "ACCT", "MODE", "STRU", "SYST", "STAT", "HELP", "SITE":
		s.handleComplianceCommand(cmd, arg)

	// Extensions
	case "HOST", "HASH", "MFMT":
		s.handleExtensionsCommand(cmd, arg)

	case "ABOR":
		s.handleABOR()

	default:
		s.reply(502, "Command not implemented.")
	}

	if err != nil {
		s.server.logger.Error("command handling error",
			"session_id", s.sessionID,
			"remote_ip", s.remoteIP,
			"user", s.user,
			"cmd", cmd,
			"error", err,
		)
	}
}

func (s *session) handleAccessCommand(cmd, arg string) error {
	switch cmd {
	case "USER":
		return s.handleUSER(arg)
	case "PASS":
		return s.handlePASS(arg)
	case "QUIT":
		s.reply(221, "Service closing control connection.")
		return nil
	}
	return nil
}

func (s *session) handleFileCommand(cmd, arg string) {
	switch cmd {
	case "CWD":
		s.handleCWD(arg)
	case "CDUP", "UP":
		s.handleCDUP()
	case "PWD":
		s.handlePWD()
	case "LIST":
		s.handleLIST(arg)
	case "NLST":
		s.handleNLST(arg)
	case "MKD", "XMKD":
		s.handleMKD(arg)
	case "RMD", "XRMD":
		s.handleRMD(arg)
	case "DELE":
		s.handleDELE(arg)
	case "RNFR":
		s.handleRNFR(arg)
	case "RNTO":
		s.handleRNTO(arg)
	}
}

func (s *session) handleTransferCommand(cmd, arg string) {
	switch cmd {
	case "RETR":
		s.handleRETR(arg)
	case "STOR":
		s.handleSTOR(arg)
	case "APPE":
		s.handleAPPE(arg)
	case "STOU":
		s.handleSTOU()
	}
}

func (s *session) handleParamCommand(cmd, arg string) {
	switch cmd {
	case "TYPE":
		s.handleTYPE(arg)
	case "PORT":
		s.handlePORT(arg)
	case "PASV":
		s.handlePASV()
	case "EPSV":
		s.handleEPSV()
	case "EPRT":
		s.handleEPRT(arg)
	case "REST":
		s.handleREST(arg)
	}
}

func (s *session) handleInfoCommand(cmd, arg string) {
	switch cmd {
	case "SIZE":
		s.handleSIZE(arg)
	case "MDTM":
		s.handleMDTM(arg)
	case "FEAT":
		s.handleFEAT()
	case "OPTS":
		s.handleOPTS(arg)
	case "MLSD":
		s.handleMLSD(arg)
	case "MLST":
		s.handleMLST(arg)
	case "NOOP":
		s.reply(200, "OK.")
	}
}

func (s *session) handleSecurityCommand(cmd, arg string) {
	switch cmd {
	case "AUTH":
		s.handleAUTH(arg)
	case "PROT":
		s.handlePROT(arg)
	case "PBSZ":
		s.handlePBSZ(arg)
	}
}

func (s *session) handleComplianceCommand(cmd, arg string) {
	switch cmd {
	case "ACCT":
		s.handleACCT(arg)
	case "MODE":
		s.handleMODE(arg)
	case "STRU":
		s.handleSTRU(arg)
	case "SYST":
		s.handleSYST()
	case "STAT":
		s.handleSTAT(arg)
	case "HELP":
		s.handleHELP(arg)
	case "SITE":
		s.handleSITE(arg)
	}
}

func (s *session) handleExtensionsCommand(cmd, arg string) {
	switch cmd {
	case "HOST":
		s.handleHOST(arg)
	case "HASH":
		s.handleHASH(arg)
	case "MFMT":
		s.handleMFMT(arg)
	}
}

func (s *session) connData() (net.Conn, error) {
	if s.pasvList != nil {
		return s.connPassive()
	}

	if s.activeIP != "" {
		return s.connActive()
	}

	return nil, fmt.Errorf("no data connection setup")
}

func (s *session) connPassive() (net.Conn, error) {
	s.server.logger.Debug("waiting for passive connection",
		"session_id", s.sessionID,
		"remote_ip", s.remoteIP,
	)
	// Set a deadline for the client to connect
	if t, ok := s.pasvList.(*net.TCPListener); ok {
		_ = t.SetDeadline(time.Now().Add(10 * time.Second))
	}
	conn, err := s.pasvList.Accept()
	if err != nil {
		return nil, err
	}
	s.pasvList.Close()
	s.pasvList = nil

	return s.wrapDataConn(conn)
}

func (s *session) connActive() (net.Conn, error) {
	addr := net.JoinHostPort(s.activeIP, strconv.Itoa(s.activePort))
	s.server.logger.Debug("dialing active connection",
		"session_id", s.sessionID,
		"remote_ip", s.remoteIP,
		"addr", addr,
	)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, err
	}
	s.activeIP = "" // Reset after use

	return s.wrapDataConn(conn)
}

func (s *session) wrapDataConn(conn net.Conn) (net.Conn, error) {
	// Wrap in TLS if protected
	if s.prot == "P" {
		if s.server.tlsConfig == nil {
			conn.Close()
			return nil, fmt.Errorf("TLS configuration missing")
		}
		// RFC 4217: The FTP server MUST act as the TLS server.
		tlsConn := tls.Server(conn, s.server.tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return nil, err
		}
		conn = tlsConn
	}

	// Apply timeouts to data connection
	if s.server.readTimeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(s.server.readTimeout))
	}
	if s.server.writeTimeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(s.server.writeTimeout))
	}

	// Track data connection
	s.server.trackConnection(conn, true)
	return &trackingConn{Conn: conn, server: s.server}, nil
}

func (s *session) handleABOR() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.busy {
		s.reply(226, "ABOR command successful; no transfer in progress.")
		return
	}

	// Transfer is in progress.
	s.server.logger.Info("transfer_abort_requested", "session_id", s.sessionID)

	// Close data connection to interrupt the background transfer goroutine.
	if s.dataConn != nil {
		s.dataConn.Close()
	}

	// Signal the transfer context to cancel.
	if s.transferCancel != nil {
		s.transferCancel()
	}

	// Per RFC 959, the server should send a 426 reply for the original
	// transfer command, followed by a 226 reply for the ABOR command.
	// Our asynchronous implementation sends 226 immediately, and the
	// transfer goroutine will send 426. This is a minor deviation but
	// is functionally acceptable for most clients.
	s.reply(226, "ABOR command successful; transfer aborted.")
}

// replyError sends a standard error response based on the error type.
func (s *session) replyError(err error) {
	if os.IsNotExist(err) {
		s.reply(550, "File not found.")
		return
	}
	if os.IsPermission(err) {
		s.reply(550, "Permission denied.")
		return
	}
	if os.IsExist(err) {
		s.reply(550, "File already exists.")
		return
	}
	s.reply(550, "Action failed: "+err.Error())
}

// reply sends a response to the client.
func (s *session) reply(code int, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.writer, "%d %s\r\n", code, message)
	s.writer.Flush()
}
