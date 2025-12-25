package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

func TestDirectoryMessage(t *testing.T) {
	// 1. Setup
	rootDir := t.TempDir()

	// Create a directory with a .message file
	msgDir := filepath.Join(rootDir, "info")
	if err := os.Mkdir(msgDir, 0755); err != nil {
		t.Fatal(err)
	}
	messageContent := "Welcome to the info directory.\nPlease behave."
	if err := os.WriteFile(filepath.Join(msgDir, ".message"), []byte(messageContent), 0644); err != nil {
		t.Fatal(err)
	}

	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Enable Directory Messages
	server, err := NewServer(":0",
		WithDriver(driver),
		WithEnableDirMessage(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	go func() {
		if err := server.Serve(ln); err != nil && err != ErrServerClosed {
			t.Logf("Server stopped: %v", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	// 2. Connect
	c, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer func() { _ = c.Quit() }()

	if err := c.Login("test", "test"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// 3. Change Directory and check response
	// We use Quote to get the raw response message
	resp, err := c.Quote("CWD info")
	if err != nil {
		t.Fatalf("CWD failed: %v", err)
	}

	if resp.Code != 250 {
		t.Errorf("Expected 250, got %d", resp.Code)
	}

	// The message should be in the response
	// Note: The client library might concatenate multiline responses
	if !strings.Contains(resp.Message, "Welcome to the info directory") {
		t.Errorf("Response did not contain .message content. Got: %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "Please behave") {
		t.Errorf("Response did not contain second line of .message. Got: %q", resp.Message)
	}
}

// manualRetrieve performs a RETR using manual PASV/Dial to avoid the client lib forcing Binary mode.
func manualRetrieve(t *testing.T, c *ftp.Client, path string) ([]byte, error) {
	// 1. Enter Passive Mode
	resp, err := c.Quote("PASV")
	if err != nil {
		return nil, err
	}
	if resp.Code != 227 {
		return nil, fmt.Errorf("expected 227 PASV, got %d", resp.Code)
	}

	// Parse PASV response (227 Entering Passive Mode (h1,h2,h3,h4,p1,p2))
	start := strings.Index(resp.Message, "(")
	end := strings.LastIndex(resp.Message, ")")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("invalid PASV response: %s", resp.Message)
	}
	parts := strings.Split(resp.Message[start+1:end], ",")
	if len(parts) != 6 {
		return nil, fmt.Errorf("invalid PASV parts: %v", parts)
	}

	// Calculate port
	p1, _ := strconv.Atoi(parts[4])
	p2, _ := strconv.Atoi(parts[5])
	port := p1*256 + p2

	// Assume localhost for test
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// 2. Connect to data port
	dataConn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial data failed: %w", err)
	}
	defer dataConn.Close()

	// 3. Send RETR
	if _, err := c.Quote("RETR " + path); err != nil {
		return nil, fmt.Errorf("RETR failed: %w", err)
	}

	// 4. Read data
	data, err := io.ReadAll(dataConn)
	if err != nil {
		return nil, fmt.Errorf("read data failed: %w", err)
	}

	return data, nil
}

// manualStore performs a STOR using manual PASV/Dial
func manualStore(t *testing.T, c *ftp.Client, path string, content []byte) error {
	// 1. Enter Passive Mode
	resp, err := c.Quote("PASV")
	if err != nil {
		return err
	}

	// Parse PASV
	start := strings.Index(resp.Message, "(")
	end := strings.LastIndex(resp.Message, ")")
	if start == -1 || end == -1 {
		return fmt.Errorf("invalid PASV response: %s", resp.Message)
	}
	parts := strings.Split(resp.Message[start+1:end], ",")
	p1, _ := strconv.Atoi(parts[4])
	p2, _ := strconv.Atoi(parts[5])
	port := p1*256 + p2
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// 2. Connect data
	dataConn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer dataConn.Close()

	// 3. Send STOR
	if _, err := c.Quote("STOR " + path); err != nil {
		return err
	}

	// 4. Write data
	if _, err := dataConn.Write(content); err != nil {
		return err
	}
	dataConn.Close() // Close to signal EOF

	return nil
}

func TestASCIIMode(t *testing.T) {
	// 1. Setup
	rootDir := t.TempDir()

	// Create a text file with Unix line endings (LF)
	contentLF := "line1\nline2\n"
	filename := "unix.txt"
	if err := os.WriteFile(filepath.Join(rootDir, filename), []byte(contentLF), 0644); err != nil {
		t.Fatal(err)
	}

	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(":0", WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	go func() { _ = server.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	c, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Quit() }()

	if err := c.Login("test", "test"); err != nil {
		t.Fatal(err)
	}

	// 2. Test Download (RETR) in ASCII mode
	if err := c.Type("A"); err != nil {
		t.Fatalf("Type ASCII failed: %v", err)
	}

	buf, err := manualRetrieve(t, c, filename)
	if err != nil {
		t.Fatalf("manualRetrieve failed: %v", err)
	}

	// Expect CRLF
	expectedCRLF := "line1\r\nline2\r\n"
	if string(buf) != expectedCRLF {
		t.Errorf("ASCII Download mismatch.\nGot: %q\nWant: %q", string(buf), expectedCRLF)
	}

	// Reconnect to clear control channel state (unconsumed 226 response)
	_ = c.Quit()
	c, err = ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Quit() }()

	if err := c.Login("test", "test"); err != nil {
		t.Fatal(err)
	}

	// 3. Test Upload (STOR) in ASCII mode
	if err := c.Type("A"); err != nil {
		t.Fatalf("Type ASCII failed: %v", err)
	}

	// We send CRLF, expect LF on disk
	uploadName := "upload.txt"
	uploadContentCRLF := []byte("foo\r\nbar\r\n")
	if err := manualStore(t, c, uploadName, uploadContentCRLF); err != nil {
		t.Fatalf("manualStore failed: %v", err)
	}

	// Wait a bit for server to process close
	time.Sleep(100 * time.Millisecond)

	// Verify on disk (should be LF)
	diskContent, err := os.ReadFile(filepath.Join(rootDir, uploadName))
	if err != nil {
		t.Fatal(err)
	}

	expectedLF := "foo\nbar\n"
	if string(diskContent) != expectedLF {
		t.Errorf("ASCII Upload mismatch.\nGot on disk: %q\nWant: %q", string(diskContent), expectedLF)
	}
}
