package server

import (
	"bytes"
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

func TestABOR(t *testing.T) {
	// 1. Setup
	rootDir := t.TempDir()
	// Create a large-ish file to have time to abort
	largeFile := "large.bin"
	// 1MB of zeros
	if err := os.WriteFile(filepath.Join(rootDir, largeFile), make([]byte, 1024*1024), 0644); err != nil {
		t.Fatal(err)
	}

	driver, _ := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	server, _ := NewServer(":0", WithDriver(driver))
	ln, _ := net.Listen("tcp", ":0")
	addr := ln.Addr().String()
	go func() { _ = server.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Quit() }()
	_ = c.Login("test", "test")

	// 2. Start RETR manually to avoid blocking client
	// We use PASV and Dial
	resp, _ := c.Quote("PASV")
	start := strings.Index(resp.Message, "(")
	end := strings.LastIndex(resp.Message, ")")
	parts := strings.Split(resp.Message[start+1:end], ",")
	p1, _ := strconv.Atoi(parts[4])
	p2, _ := strconv.Atoi(parts[5])
	dataAddr := fmt.Sprintf("127.0.0.1:%d", p1*256+p2)

	dataConn, err := net.Dial("tcp", dataAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer dataConn.Close()

	// Send RETR
	_, err = c.Quote("RETR " + largeFile)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Wait a tiny bit and send ABOR
	time.Sleep(50 * time.Millisecond)

	aborResp, err := c.Quote("ABOR")
	if err != nil {
		t.Fatalf("ABOR failed: %v", err)
	}

	// ABOR response should be 226 (or 225)
	if aborResp.Code != 226 && aborResp.Code != 225 {
		t.Errorf("Expected 226/225 for ABOR, got %d %s", aborResp.Code, aborResp.Message)
	}

	// Data connection should be closed by server
	// Try to read - should get EOF or error quickly
	buf := make([]byte, 1024)
	_ = dataConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, err := dataConn.Read(buf)
	if err == nil && n > 0 {
		// We might read some buffered data, that's fine.
		// But eventually it should close.
		for err == nil {
			_, err = dataConn.Read(buf)
		}
	}

	if err == nil {
		t.Error("Expected data connection to be closed after ABOR")
	}
}

func TestServerMiscFeatures(t *testing.T) {
	// Setup temporary directory
	rootDir := t.TempDir()

	// Create test file structure
	// /
	//   file1.txt
	//   subdir/
	//     file2.txt
	err := os.WriteFile(filepath.Join(rootDir, "file1.txt"), []byte("content1"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Mkdir(filepath.Join(rootDir, "subdir"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(rootDir, "subdir", "file2.txt"), []byte("content2"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Buffer for transfer log
	var logBuf bytes.Buffer

	// Create driver with Anon Write enabled
	driver, err := NewFSDriver(rootDir, WithAnonWrite(true))
	if err != nil {
		t.Fatal(err)
	}

	// Create server with transfer logging
	s, err := NewServer(":0",
		WithDriver(driver),
		WithTransferLog(&logBuf),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Start server
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		if err := s.Serve(ln); err != ErrServerClosed {
			t.Errorf("Serve() execution error: %v", err)
		}
	}()
	defer func() { _ = s.Shutdown(context.Background()) }()

	// Wait for server to start
	addr := ln.Addr().String()

	/* TEST 1: Anonymous Write (STOR) & Transfer Logging */
	{
		conn, err := rawLogin(addr, "anonymous", "test@example.com")
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		// Passive mode
		dataAddr, err := rawEnterPasv(conn)
		if err != nil {
			t.Fatal(err)
		}

		// Connect data channel
		dataConn, err := net.DialTimeout("tcp", dataAddr, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer dataConn.Close()

		// Upload file
		fmt.Fprintf(conn, "STOR upload.txt\r\n")
		// Write data
		fmt.Fprintf(dataConn, "uploaded content")
		dataConn.Close() // Close data conn to finish transfer

		// Read response
		code, _, err := rawReadResponse(conn)
		if err != nil {
			t.Fatal(err)
		}
		// Expect 150 then 226
		if code == 150 {
			code, _, err = rawReadResponse(conn)
			if err != nil {
				t.Fatal(err)
			}
		}
		if code != 226 {
			t.Errorf("Expected 226 Transfer complete, got %d", code)
		}

		conn.Close()
	}

	// Verify Log
	time.Sleep(100 * time.Millisecond) // Allow log flush
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "upload.txt") {
		t.Errorf("Log should contain filename 'upload.txt', got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "i a anonymous") { // incoming, anonymous
		t.Errorf("Log should indicate incoming anonymous transfer, got: %s", logOutput)
	}

	/* TEST 2: Recursive List (LIST -R) */
	{
		conn, err := rawLogin(addr, "anonymous", "test@example.com")
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		dataAddr, err := rawEnterPasv(conn)
		if err != nil {
			t.Fatal(err)
		}

		dataConn, err := net.DialTimeout("tcp", dataAddr, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}

		fmt.Fprintf(conn, "LIST -R\r\n")

		// Read all data from data connection
		var buf bytes.Buffer
		_, err = buf.ReadFrom(dataConn)
		dataConn.Close()
		if err != nil {
			t.Fatal(err)
		}

		// Consume control response
		_, _, _ = rawReadResponse(conn)
		_, _, _ = rawReadResponse(conn)

		listing := buf.String()
		if !strings.Contains(listing, "file1.txt") {
			t.Errorf("Recursive listing missing root file. Got:\n%s", listing)
		}
		if !strings.Contains(listing, "subdir:") {
			t.Errorf("Recursive listing missing subdir header")
		}
		if !strings.Contains(listing, "file2.txt") {
			t.Errorf("Recursive listing missing subdir file")
		}
	}
}

// Helpers for TestServerMiscFeatures

func rawLogin(addr, user, pass string) (*textConn, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	tc := &textConn{conn}

	// Read greeting
	if _, _, err := rawReadResponse(tc); err != nil {
		return nil, err
	}

	fmt.Fprintf(conn, "USER %s\r\n", user)
	if _, _, err := rawReadResponse(tc); err != nil {
		return nil, err
	}

	fmt.Fprintf(conn, "PASS %s\r\n", pass)
	if code, _, err := rawReadResponse(tc); err != nil || code != 230 {
		return nil, fmt.Errorf("login failed: %d %v", code, err)
	}

	return tc, nil
}

func rawEnterPasv(c *textConn) (string, error) {
	fmt.Fprintf(c.Conn, "PASV\r\n")
	code, msg, err := rawReadResponse(c)
	if err != nil {
		return "", err
	}
	if code != 227 {
		return "", fmt.Errorf("PASV failed: %d", code)
	}

	// Parse "227 Entering Passive Mode (h1,h2,h3,h4,p1,p2)."
	start := strings.Index(msg, "(")
	end := strings.Index(msg, ")")
	if start == -1 || end == -1 {
		return "", fmt.Errorf("invalid PASV response")
	}

	// Re-parse simplified
	var v1, v2, v3, v4, vp1, vp2 int
	_, _ = fmt.Sscanf(msg[start+1:end], "%d,%d,%d,%d,%d,%d", &v1, &v2, &v3, &v4, &vp1, &vp2)
	port := vp1*256 + vp2
	ip := fmt.Sprintf("%d.%d.%d.%d", v1, v2, v3, v4)

	return fmt.Sprintf("%s:%d", ip, port), nil
}

type textConn struct {
	net.Conn
}

func rawReadResponse(c *textConn) (int, string, error) {
	buf := make([]byte, 1024)
	n, err := c.Read(buf)
	if err != nil {
		return 0, "", err
	}
	line := string(buf[:n])
	var code int
	_, _ = fmt.Sscanf(line, "%d", &code)
	return code, line, nil
}
