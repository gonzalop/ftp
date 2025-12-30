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
	t.Parallel()
	// 1. Setup
	rootDir := t.TempDir()

	// Create a directory with a .message file
	msgDir := filepath.Join(rootDir, "info")
	fatalIfErr(t, os.Mkdir(msgDir, 0755), "Failed to create info dir")
	messageContent := "Welcome to the info directory.\nPlease behave."
	fatalIfErr(t, os.WriteFile(filepath.Join(msgDir, ".message"), []byte(messageContent), 0644), "Failed to write .message")

	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	fatalIfErr(t, err, "Failed to create FS driver")

	// Enable Directory Messages
	server, err := NewServer(":0",
		WithDriver(driver),
		WithEnableDirMessage(true),
	)
	fatalIfErr(t, err, "Failed to create server")

	ln, err := net.Listen("tcp", ":0")
	fatalIfErr(t, err, "Failed to listen")
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
	fatalIfErr(t, err, "Dial failed")
	defer func() { _ = c.Quit() }()

	fatalIfErr(t, c.Login("test", "test"), "Login failed")

	// 3. Change Directory and check response
	// We use Quote to get the raw response message
	resp, err := c.Quote("CWD info")
	fatalIfErr(t, err, "CWD failed")

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
func manualRetrieve(_ *testing.T, c *ftp.Client, path string) ([]byte, error) {
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
func manualStore(_ *testing.T, c *ftp.Client, path string, content []byte) error {
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
	t.Parallel()
	// 1. Setup
	rootDir := t.TempDir()

	// Create a text file with Unix line endings (LF)
	contentLF := "line1\nline2\n"
	filename := "unix.txt"
	fatalIfErr(t, os.WriteFile(filepath.Join(rootDir, filename), []byte(contentLF), 0644), "Failed to write unix.txt")

	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	fatalIfErr(t, err, "Failed to create FS driver")

	server, err := NewServer(":0", WithDriver(driver))
	fatalIfErr(t, err, "Failed to create server")

	ln, err := net.Listen("tcp", ":0")
	fatalIfErr(t, err, "Failed to listen")
	addr := ln.Addr().String()

	go func() { _ = server.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	c, err := ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	fatalIfErr(t, err, "Dial failed")
	defer func() { _ = c.Quit() }()

	fatalIfErr(t, c.Login("test", "test"), "Login failed")

	// 2. Test Download (RETR) in ASCII mode
	fatalIfErr(t, c.Type("A"), "Type ASCII failed")

	buf, err := manualRetrieve(t, c, filename)
	fatalIfErr(t, err, "manualRetrieve failed")

	// Expect CRLF
	expectedCRLF := "line1\r\nline2\r\n"
	if string(buf) != expectedCRLF {
		t.Errorf("ASCII Download mismatch.\nGot: %q\nWant: %q", string(buf), expectedCRLF)
	}

	// Reconnect to clear control channel state (unconsumed 226 response)
	_ = c.Quit()
	c, err = ftp.Dial(addr, ftp.WithTimeout(2*time.Second))
	fatalIfErr(t, err, "Dial failed (reconnect)")
	defer func() { _ = c.Quit() }()

	fatalIfErr(t, c.Login("test", "test"), "Login failed (reconnect)")

	// 3. Test Upload (STOR) in ASCII mode
	fatalIfErr(t, c.Type("A"), "Type ASCII failed")

	// We send CRLF, expect LF on disk
	uploadName := "upload.txt"
	uploadContentCRLF := []byte("foo\r\nbar\r\n")
	fatalIfErr(t, manualStore(t, c, uploadName, uploadContentCRLF), "manualStore failed")

	// Wait a bit for server to process close
	time.Sleep(100 * time.Millisecond)

	// Verify on disk (should be LF)
	diskContent, err := os.ReadFile(filepath.Join(rootDir, uploadName))
	fatalIfErr(t, err, "Failed to read uploaded file")

	expectedLF := "foo\nbar\n"
	if string(diskContent) != expectedLF {
		t.Errorf("ASCII Upload mismatch.\nGot on disk: %q\nWant: %q", string(diskContent), expectedLF)
	}
}

func TestABOR(t *testing.T) {
	t.Parallel()
	// 1. Setup
	rootDir := t.TempDir()
	// Create a large-ish file to have time to abort
	largeFile := "large.bin"
	// 1MB of zeros
	if err := os.WriteFile(filepath.Join(rootDir, largeFile), make([]byte, 1024*1024), 0644); err != nil {
		t.Fatal(err)
	}

	driver, _ := NewFSDriver(rootDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
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
	fatalIfErr(t, err, "Dial failed")
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
	fatalIfErr(t, err, "Dial data port failed")
	defer dataConn.Close()

	// Send RETR
	_, err = c.Quote("RETR " + largeFile)
	fatalIfErr(t, err, "RETR failed")

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
	t.Parallel()
	rootDir := t.TempDir()

	// Create test file structure
	fatalIfErr(t, os.WriteFile(filepath.Join(rootDir, "file1.txt"), []byte("content1"), 0644), "Failed to write file1.txt")
	fatalIfErr(t, os.Mkdir(filepath.Join(rootDir, "subdir"), 0755), "Failed to create subdir")
	fatalIfErr(t, os.WriteFile(filepath.Join(rootDir, "subdir", "file2.txt"), []byte("content2"), 0644), "Failed to write file2.txt")

	var logBuf bytes.Buffer
	driver, err := NewFSDriver(rootDir, WithAnonWrite(true))
	fatalIfErr(t, err, "Failed to create FS driver")

	s, err := NewServer(":0",
		WithDriver(driver),
		WithTransferLog(&logBuf),
	)
	fatalIfErr(t, err, "Failed to create server")

	ln, err := net.Listen("tcp", ":0")
	fatalIfErr(t, err, "Failed to listen")
	go func() {
		if err := s.Serve(ln); err != ErrServerClosed {
			t.Errorf("Serve() execution error: %v", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown failed: %v", err)
		}
	}()

	addr := ln.Addr().String()

	testAnonWriteAndTransferLog(t, addr, &logBuf)
	testRecursiveList(t, addr)
	testUmask(t)
}

func testAnonWriteAndTransferLog(t *testing.T, addr string, logBuf *bytes.Buffer) {
	conn, err := rawLogin(addr, "anonymous", "test@example.com")
	fatalIfErr(t, err, "rawLogin failed")
	defer conn.Close()

	dataAddr, err := rawEnterPasv(conn)
	fatalIfErr(t, err, "rawEnterPasv failed")

	dataConn, err := net.DialTimeout("tcp", dataAddr, 5*time.Second)
	fatalIfErr(t, err, "Dial data port failed")
	defer dataConn.Close()

	fmt.Fprintf(conn, "STOR upload.txt\r\n")
	fmt.Fprintf(dataConn, "uploaded content")
	dataConn.Close()

	code, _, err := rawReadResponse(conn)
	fatalIfErr(t, err, "rawReadResponse failed")
	if code == 150 {
		code, _, err = rawReadResponse(conn)
		fatalIfErr(t, err, "rawReadResponse (226) failed")
	}
	if code != 226 {
		t.Errorf("Expected 226 Transfer complete, got %d", code)
	}

	conn.Close()

	time.Sleep(100 * time.Millisecond)
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "upload.txt") {
		t.Errorf("Log should contain filename 'upload.txt', got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "i a anonymous") {
		t.Errorf("Log should indicate incoming anonymous transfer, got: %s", logOutput)
	}
}

func testRecursiveList(t *testing.T, addr string) {
	conn, err := rawLogin(addr, "anonymous", "test@example.com")
	fatalIfErr(t, err, "rawLogin failed")
	defer conn.Close()

	dataAddr, err := rawEnterPasv(conn)
	fatalIfErr(t, err, "rawEnterPasv failed")

	dataConn, err := net.DialTimeout("tcp", dataAddr, 5*time.Second)
	fatalIfErr(t, err, "Dial data port failed")

	fmt.Fprintf(conn, "LIST -R\r\n")

	var buf bytes.Buffer
	fatalIfErr(t, dataConn.SetReadDeadline(time.Now().Add(5*time.Second)), "SetReadDeadline failed")
	_, err = buf.ReadFrom(dataConn)
	dataConn.Close()
	fatalIfErr(t, err, "ReadFrom failed")

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

func testUmask(t *testing.T) {
	rootDirUmask := t.TempDir()
	driverUmask, err := NewFSDriver(rootDirUmask, WithAnonWrite(true), WithSettings(&Settings{
		Umask: 0077,
	}))
	fatalIfErr(t, err, "Failed to create FS driver")

	var logBuf bytes.Buffer
	sUmask, err := NewServer(":0", WithDriver(driverUmask), WithTransferLog(&logBuf))
	fatalIfErr(t, err, "Failed to create server")
	lnUmask, err := net.Listen("tcp", ":0")
	fatalIfErr(t, err, "Failed to listen")
	go func() {
		_ = sUmask.Serve(lnUmask)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sUmask.Shutdown(ctx)
	}()

	addrUmask := lnUmask.Addr().String()

	conn, err := rawLogin(addrUmask, "anonymous", "test@example.com")
	fatalIfErr(t, err, "rawLogin failed")
	defer conn.Close()

	dataAddr, err := rawEnterPasv(conn)
	fatalIfErr(t, err, "rawEnterPasv failed")

	dataConn, err := net.DialTimeout("tcp", dataAddr, 5*time.Second)
	fatalIfErr(t, err, "Dial data port failed")
	defer dataConn.Close()

	fmt.Fprintf(conn, "STOR private.txt\r\n")
	fmt.Fprintf(dataConn, "secret")
	dataConn.Close()

	_, _, _ = rawReadResponse(conn)
	_, _, _ = rawReadResponse(conn)

	info, err := os.Stat(filepath.Join(rootDirUmask, "private.txt"))
	fatalIfErr(t, err, "Stat failed")
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("Expected 0600 permission with umask 077, got %v", perm)
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
	if err := c.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return 0, "", err
	}
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
