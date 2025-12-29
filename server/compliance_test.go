package server

import (
	"bufio"
	"fmt"
	"net"
	"runtime"
	"strings"
	"testing"
)

func TestRFC1123Compliance(t *testing.T) {
	rootDir := t.TempDir()

	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	server, err := NewServer(addr, WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		if err := server.Serve(ln); err != nil && err != ErrServerClosed {
			t.Logf("Server stopped: %v", err)
		}
	}()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	sendCmd := makeSendCmd(conn, reader)

	_, _ = reader.ReadString('\n')

	sendCmd("USER test")
	sendCmd("PASS test")

	t.Run("SYST", func(t *testing.T) { testSYST(t, sendCmd) })
	t.Run("MODE", func(t *testing.T) { testMODE(t, sendCmd) })
	t.Run("STRU", func(t *testing.T) { testSTRU(t, sendCmd) })
	t.Run("ACCT", func(t *testing.T) { testACCT(t, sendCmd) })
	t.Run("STAT", func(t *testing.T) { testSTAT(t, sendCmd) })
	t.Run("HELP", func(t *testing.T) { testHELP(t, sendCmd) })
}

func makeSendCmd(conn net.Conn, reader *bufio.Reader) func(string) (int, string) {
	return func(cmd string) (int, string) {
		fmt.Fprintf(conn, "%s\r\n", cmd)
		line, _ := reader.ReadString('\n')
		var code int
		var msg string
		_, _ = fmt.Sscanf(line, "%d %s", &code, &msg)
		var fullMsg strings.Builder
		fullMsg.WriteString(line)
		if len(line) >= 4 && line[3] == '-' {
			for {
				line, _ = reader.ReadString('\n')
				fullMsg.WriteString(line)
				if len(line) >= 4 && line[3] == ' ' {
					break
				}
			}
		}
		return code, strings.TrimSpace(fullMsg.String())
	}
}

func testSYST(t *testing.T, sendCmd func(string) (int, string)) {
	code, msg := sendCmd("SYST")
	if code != 215 {
		t.Errorf("Expected code 215, got %d", code)
	}
	msgUpper := strings.ToUpper(msg)
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "openbsd", "netbsd":
		if !strings.Contains(msgUpper, "UNIX") {
			t.Errorf("Expected UNIX in response, got: %s", msg)
		}
	case "windows":
		if !strings.Contains(msgUpper, "WINDOWS") {
			t.Errorf("Expected Windows in response, got: %s", msg)
		}
	}
}

func testMODE(t *testing.T, sendCmd func(string) (int, string)) {
	code, _ := sendCmd("MODE S")
	if code != 200 {
		t.Errorf("Expected code 200 for MODE S, got %d", code)
	}

	code, _ = sendCmd("MODE B")
	if code != 504 {
		t.Errorf("Expected code 504 for MODE B, got %d", code)
	}
}

func testSTRU(t *testing.T, sendCmd func(string) (int, string)) {
	code, _ := sendCmd("STRU F")
	if code != 200 {
		t.Errorf("Expected code 200 for STRU F, got %d", code)
	}

	code, _ = sendCmd("STRU R")
	if code != 504 {
		t.Errorf("Expected code 504 for STRU R, got %d", code)
	}
}

func testACCT(t *testing.T, sendCmd func(string) (int, string)) {
	code, msg := sendCmd("ACCT test")
	if code != 202 {
		t.Errorf("Expected code 202, got %d", code)
	}
	if !strings.Contains(strings.ToLower(msg), "superfluous") {
		t.Errorf("Expected 'superfluous' in message, got: %s", msg)
	}
}

func testSTAT(t *testing.T, sendCmd func(string) (int, string)) {
	code, msg := sendCmd("STAT")
	if code != 211 {
		t.Errorf("Expected code 211, got %d", code)
	}
	msgLower := strings.ToLower(msg)
	if !strings.Contains(msgLower, "logged in") && !strings.Contains(msgLower, "status") {
		t.Errorf("Expected status info in response, got: %s", msg)
	}
}

func testHELP(t *testing.T, sendCmd func(string) (int, string)) {
	code, msg := sendCmd("HELP")
	if code != 214 {
		t.Errorf("Expected code 214, got %d", code)
	}
	msgUpper := strings.ToUpper(msg)
	requiredCommands := []string{"USER", "PASS", "QUIT", "RETR", "STOR", "LIST"}
	for _, cmd := range requiredCommands {
		if !strings.Contains(msgUpper, cmd) {
			t.Errorf("Expected %s in HELP response, got: %s", cmd, msg)
		}
	}
}
