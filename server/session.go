package server

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
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

	// State
	isLoggedIn    bool
	user          string
	renameFrom    string // For RNFR/RNTO
	fs            ClientContext
	restartOffset int64  // For REST command
	host          string // From HOST command

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

// newSession creates a new session.
func newSession(server *Server, conn net.Conn) *session {
	s := &session{
		server: server,
		conn:   conn,
		reader: bufio.NewReader(newTelnetReader(conn)),
		writer: bufio.NewWriter(conn),
		prot:   "C", // Default to clear
	}

	// Detect Implicit TLS (connection is already a *tls.Conn)
	if _, ok := conn.(*tls.Conn); ok {
		s.prot = "P" // Default to private for implicit TLS
	}

	return s
}

// serve handles the FTP session.
func (s *session) serve() {
	defer s.close()

	// Send welcome message
	s.reply(220, "Service ready for new user.")

	for {
		// Set read deadline
		if s.server.maxIdleTime > 0 {
			// Ignore deadline errors as they are non-critical
			_ = s.conn.SetReadDeadline(time.Now().Add(s.server.maxIdleTime))
		}

		// Reset deadline (will be reset for reading)
		// But actually we want deadline specifically for the read.
		// Wait, the logic in previous code set it, read, then reset it.

		line, err := s.readCommand()
		if err != nil {
			if err != io.EOF && err.Error() != "command too long" {
				s.server.logger.Warn("read error", "remote", s.conn.RemoteAddr(), "error", err)
			}
			if err.Error() == "command too long" {
				s.reply(500, "Command line too long.")
			}
			return
		}

		// Clear deadline
		_ = s.conn.SetReadDeadline(time.Time{})

		s.handleCommand(line)
	}
}

// readCommand reads a line from the reader with a limit.
func (s *session) readCommand() (string, error) {
	var line []byte
	for {
		b, err := s.reader.ReadByte()
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
	s.server.logger.Debug("session closed", "remote", s.conn.RemoteAddr())
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
	s.server.logger.Debug("command received", "cmd", cmd, "arg", logArg)

	var err error
	switch cmd {
	// Access Control
	case "USER":
		err = s.handleUSER(arg)
	case "PASS":
		err = s.handlePASS(arg)
	case "QUIT":
		s.reply(221, "Service closing control connection.")
		return

	// File Management
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

	// File Transfer
	case "RETR":
		s.handleRETR(arg)
	case "STOR":
		s.handleSTOR(arg)
	case "APPE":
		s.handleAPPE(arg)
	case "STOU":
		s.handleSTOU()

	// Transfer Parameters
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

	// Information
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

	// Security
	case "AUTH":
		s.handleAUTH(arg)
	case "PROT":
		s.handlePROT(arg)
	case "PBSZ":
		s.handlePBSZ(arg)

	// RFC 1123 Compliance
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

	// Extensions
	case "HOST":
		s.handleHOST(arg)
	case "MFMT":
		s.handleMFMT(arg)

	default:
		s.reply(502, "Command not implemented.")
	}

	if err != nil {
		s.server.logger.Error("command handling error", "cmd", cmd, "error", err)
	}
}

func (s *session) connData() (net.Conn, error) {
	if s.pasvList != nil {
		s.server.logger.Debug("waiting for passive connection")
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

		// Wrap in TLS if protected
		if s.prot == "P" {
			if s.server.tlsConfig == nil {
				conn.Close()
				return nil, fmt.Errorf("TLS configuration missing")
			}
			tlsConn := tls.Server(conn, s.server.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				conn.Close()
				return nil, err
			}
			return tlsConn, nil
		}

		return conn, nil
	}

	if s.activeIP != "" {
		addr := net.JoinHostPort(s.activeIP, strconv.Itoa(s.activePort))
		s.server.logger.Debug("dialing active connection", "addr", addr)
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return nil, err
		}
		s.activeIP = "" // Reset after use

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
			return tlsConn, nil
		}

		return conn, nil
	}

	return nil, fmt.Errorf("no data connection setup")
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
	fmt.Fprintf(s.writer, "%d %s\r\n", code, message)
	s.writer.Flush()
}
