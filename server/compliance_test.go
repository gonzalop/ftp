package server

import (
	"bufio"
	"fmt"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRFC1123Compliance(t *testing.T) {
	// Setup temporary directory for server root
	rootDir := t.TempDir()

	// Start Server
	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(":2125", WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			t.Logf("Server stopped: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)

	// Connect with raw TCP
	conn, err := net.Dial("tcp", "localhost:2125")
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Helper function to send command and read response
	sendCmd := func(cmd string) (int, string) {
		fmt.Fprintf(conn, "%s\r\n", cmd)
		line, _ := reader.ReadString('\n')
		var code int
		var msg string
		_, _ = fmt.Sscanf(line, "%d %s", &code, &msg)
		// Read multi-line responses
		fullMsg := line
		if len(line) >= 4 && line[3] == '-' {
			for {
				line, _ = reader.ReadString('\n')
				fullMsg += line
				if len(line) >= 4 && line[3] == ' ' {
					break
				}
			}
		}
		return code, strings.TrimSpace(fullMsg)
	}

	// Read welcome
	_, _ = reader.ReadString('\n')

	// Login
	sendCmd("USER test")
	sendCmd("PASS test")

	t.Run("SYST", func(t *testing.T) {
		code, msg := sendCmd("SYST")
		if code != 215 {
			t.Errorf("Expected code 215, got %d", code)
		}
		// Verify it contains expected OS type
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
	})

	t.Run("MODE", func(t *testing.T) {
		// Test valid mode
		code, _ := sendCmd("MODE S")
		if code != 200 {
			t.Errorf("Expected code 200 for MODE S, got %d", code)
		}

		// Test invalid mode
		code, _ = sendCmd("MODE B")
		if code != 504 {
			t.Errorf("Expected code 504 for MODE B, got %d", code)
		}
	})

	t.Run("STRU", func(t *testing.T) {
		// Test valid structure
		code, _ := sendCmd("STRU F")
		if code != 200 {
			t.Errorf("Expected code 200 for STRU F, got %d", code)
		}

		// Test invalid structure
		code, _ = sendCmd("STRU R")
		if code != 504 {
			t.Errorf("Expected code 504 for STRU R, got %d", code)
		}
	})

	t.Run("ACCT", func(t *testing.T) {
		code, msg := sendCmd("ACCT test")
		if code != 202 {
			t.Errorf("Expected code 202, got %d", code)
		}
		if !strings.Contains(strings.ToLower(msg), "superfluous") {
			t.Errorf("Expected 'superfluous' in message, got: %s", msg)
		}
	})

	t.Run("STAT", func(t *testing.T) {
		code, msg := sendCmd("STAT")
		if code != 211 {
			t.Errorf("Expected code 211, got %d", code)
		}
		// Should contain status information
		msgLower := strings.ToLower(msg)
		if !strings.Contains(msgLower, "logged in") && !strings.Contains(msgLower, "status") {
			t.Errorf("Expected status info in response, got: %s", msg)
		}
	})

	t.Run("HELP", func(t *testing.T) {
		code, msg := sendCmd("HELP")
		if code != 214 {
			t.Errorf("Expected code 214, got %d", code)
		}
		// Should list commands
		msgUpper := strings.ToUpper(msg)
		requiredCommands := []string{"USER", "PASS", "QUIT", "RETR", "STOR", "LIST"}
		for _, cmd := range requiredCommands {
			if !strings.Contains(msgUpper, cmd) {
				t.Errorf("Expected %s in HELP response, got: %s", cmd, msg)
			}
		}
	})
}
