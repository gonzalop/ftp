package ftp

import (
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestParseFeatureLines_RFC2389(t *testing.T) {
	t.Parallel()
	// RFC 2389 format with space-prefixed feature lines
	lines := []string{
		"211-Extensions supported:",
		" MLST size*;create;modify*;perm;media-type",
		" SIZE",
		" COMPRESSION",
		" MDTM",
		"211 END",
	}

	features := parseFeatureLines(lines)

	expected := map[string]string{
		"MLST":        "size*;create;modify*;perm;media-type",
		"SIZE":        "",
		"COMPRESSION": "",
		"MDTM":        "",
	}

	if len(features) != len(expected) {
		t.Errorf("expected %d features, got %d", len(expected), len(features))
	}

	for name, params := range expected {
		if gotParams, ok := features[name]; !ok {
			t.Errorf("missing feature %s", name)
		} else if gotParams != params {
			t.Errorf("feature %s: expected params %q, got %q", name, params, gotParams)
		}
	}
}

// mockServer provides a simple way to script server responses
type mockServer struct {
	listener net.Listener
	addr     string
	// commands contains the script of expected commands and responses
	// Key: Command (e.g., "USER"), Value: Response (e.g., "331 Please specify the password.")
	// Use handlers for dynamic behavior
	handlers map[string]func(conn *textproto.Conn, args string)
	// dataListener is used for passive mode
	dataListener net.Listener
	// receivedCommands records all commands received
	receivedCommands []string
	// done channel to signal server loop exit
	done chan struct{}
}

func newMockServer(t *testing.T) *mockServer {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return &mockServer{
		listener:         l,
		addr:             l.Addr().String(),
		handlers:         make(map[string]func(*textproto.Conn, string)),
		receivedCommands: make([]string, 0),
		done:             make(chan struct{}),
	}
}

func (s *mockServer) start() {
	go func() {
		defer close(s.done)
		conn, err := s.listener.Accept()
		if err != nil {
			// t.Logf("Mock server accept error: %v", err)
			return
		}
		defer conn.Close()

		// Send welcome message
		fmt.Fprintf(conn, "220 Service ready\r\n")

		textConn := textproto.NewConn(conn)
		defer textConn.Close()

		for {
			line, err := textConn.ReadLine()
			if err != nil {
				return
			}

			parts := strings.SplitN(line, " ", 2)
			cmd := strings.ToUpper(parts[0])
			args := ""
			if len(parts) > 1 {
				args = parts[1]
			}

			s.receivedCommands = append(s.receivedCommands, cmd)

			if handler, ok := s.handlers[cmd]; ok {
				handler(textConn, args)
			} else {
				// Default behavior for common commands if no handler
				switch cmd {
				case "USER":
					_ = textConn.PrintfLine("331 User name okay, need password.")
				case "PASS":
					_ = textConn.PrintfLine("230 User logged in, proceed.")
				case "QUIT":
					_ = textConn.PrintfLine("221 Service closing control connection.")
					return
				case "TYPE":
					_ = textConn.PrintfLine("200 Command okay.")
				default:
					_ = textConn.PrintfLine("502 Command not implemented.")
				}
			}
		}
	}()
}

func (s *mockServer) stop() {
	s.listener.Close()
	if s.dataListener != nil {
		s.dataListener.Close()
	}
	<-s.done
}

func TestClient_EPSV_Fallback(t *testing.T) {
	t.Parallel()
	ms := newMockServer(t)

	// Setup PASV listener
	pasvL, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ms.dataListener = pasvL

	// Pre-calculate port bytes for PASV response
	_, portStr, _ := net.SplitHostPort(pasvL.Addr().String())
	port := 0
	_, _ = fmt.Sscanf(portStr, "%d", &port)
	p1 := port / 256
	p2 := port % 256
	pasvResp := fmt.Sprintf("227 Entering Passive Mode (127,0,0,1,%d,%d).", p1, p2)

	// Scripting the server
	ms.handlers["EPSV"] = func(c *textproto.Conn, args string) {
		_ = c.PrintfLine("502 Command not implemented.")
	}
	ms.handlers["PASV"] = func(c *textproto.Conn, args string) {
		_ = c.PrintfLine("%s", pasvResp)
	}
	ms.handlers["LIST"] = func(c *textproto.Conn, args string) {
		_ = c.PrintfLine("150 File status okay; about to open data connection.")

		// Accept data connection
		dconn, err := ms.dataListener.Accept()
		if err != nil {
			t.Errorf("Mock server failed to accept data conn: %v", err)
			return
		}
		dconn.Close() // Close immediately to signal EOF

		_ = c.PrintfLine("226 Closing data connection.")
	}

	ms.start()
	defer ms.stop()

	c, err := Dial(ms.addr, WithTimeout(1*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Quit() }()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatal(err)
	}

	// 1st List: Should try EPSV, fail, try PASV, succeed
	if _, err := c.List("."); err != nil {
		t.Errorf("First List failed: %v", err)
	}

	// 2nd List: Should NOT try EPSV, straight to PASV
	if _, err := c.List("."); err != nil {
		t.Errorf("Second List failed: %v", err)
	}

	// Verify command sequence
	// Expected: USER, PASS, FEAT (maybe), TYPE (maybe), EPSV, PASV, LIST, TYPE (maybe), PASV, LIST
	// But we only care about EPSV presence

	epsvCount := 0
	for _, cmd := range ms.receivedCommands {
		if cmd == "EPSV" {
			epsvCount++
		}
	}

	if epsvCount != 1 {
		t.Errorf("Expected exactly 1 EPSV command, got %d. Commands: %v", epsvCount, ms.receivedCommands)
	}
}

func TestClient_EPSV_Success(t *testing.T) {
	t.Parallel()
	ms := newMockServer(t)

	// Setup EPSV listener
	epsvL, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ms.dataListener = epsvL

	_, portStr, _ := net.SplitHostPort(epsvL.Addr().String())
	epsvResp := fmt.Sprintf("229 Entering Extended Passive Mode (|||%s|)", portStr)

	ms.handlers["EPSV"] = func(c *textproto.Conn, args string) {
		_ = c.PrintfLine("%s", epsvResp)
	}
	ms.handlers["LIST"] = func(c *textproto.Conn, args string) {
		_ = c.PrintfLine("150 File status okay.")
		dconn, err := ms.dataListener.Accept()
		if err != nil {
			t.Errorf("Mock server failed to accept data conn: %v", err)
			return
		}
		dconn.Close()
		_ = c.PrintfLine("226 Closing data connection.")
	}

	ms.start()
	defer ms.stop()

	c, err := Dial(ms.addr, WithTimeout(1*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Quit() }()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatal(err)
	}

	// 1st List: Should use EPSV
	if _, err := c.List("."); err != nil {
		t.Errorf("First List failed: %v", err)
	}

	// 2nd List: Should use EPSV again
	if _, err := c.List("."); err != nil {
		t.Errorf("Second List failed: %v", err)
	}

	epsvCount := 0
	for _, cmd := range ms.receivedCommands {
		if cmd == "EPSV" {
			epsvCount++
		}
	}

	if epsvCount != 2 {
		t.Errorf("Expected 2 EPSV commands, got %d. Commands: %v", epsvCount, ms.receivedCommands)
	}
}

func TestClient_EPSV_FailButNot502(t *testing.T) {
	t.Parallel()
	// Verify that if it fails with something other than 502, we don't permanently disable it.
	// The current logic only disables on 502. If it's another error, we fallback to PASV for that request but not set the disable flag.
	// So if EPSV returns 500, disableEPSV is NOT set, and we DO fallback to PASV.
	// Next time we try EPSV again.

	ms := newMockServer(t)

	// Setup PASV listener
	pasvL, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ms.dataListener = pasvL

	// Pre-calculate port bytes for PASV response
	_, portStr, _ := net.SplitHostPort(pasvL.Addr().String())
	port := 0
	_, _ = fmt.Sscanf(portStr, "%d", &port)
	p1 := port / 256
	p2 := port % 256
	pasvResp := fmt.Sprintf("227 Entering Passive Mode (127,0,0,1,%d,%d).", p1, p2)

	ms.handlers["EPSV"] = func(c *textproto.Conn, args string) {
		_ = c.PrintfLine("500 Syntax error, command unrecognized.")
	}
	ms.handlers["PASV"] = func(c *textproto.Conn, args string) {
		_ = c.PrintfLine("%s", pasvResp)
	}
	ms.handlers["LIST"] = func(c *textproto.Conn, args string) {
		_ = c.PrintfLine("150 File status okay.")
		dconn, err := ms.dataListener.Accept()
		if err != nil {
			t.Errorf("Mock server failed to accept data conn: %v", err)
			return
		}
		dconn.Close()
		_ = c.PrintfLine("226 Closing data connection.")
	}

	ms.start()
	defer ms.stop()

	c, err := Dial(ms.addr, WithTimeout(1*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Quit() }()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatal(err)
	}

	// 1st List: Should try EPSV, fail (500), try PASV
	if _, err := c.List("."); err != nil {
		t.Errorf("First List failed: %v", err)
	}

	// 2nd List: Should try EPSV again (since it wasn't 502)
	if _, err := c.List("."); err != nil {
		t.Errorf("Second List failed: %v", err)
	}

	epsvCount := 0
	for _, cmd := range ms.receivedCommands {
		if cmd == "EPSV" {
			epsvCount++
		}
	}

	if epsvCount != 2 {
		t.Errorf("Expected 2 EPSV commands (retry on non-502), got %d. Commands: %v", epsvCount, ms.receivedCommands)
	}
}
