package ftp

import (
	"bufio"
	"fmt"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadResponse_Limits(t *testing.T) {
	t.Parallel()
	t.Run("line too long", func(t *testing.T) {
		// Create a line longer than 4096 bytes
		longLine := "220 " + strings.Repeat("a", 4100) + "\r\n"
		reader := bufio.NewReader(strings.NewReader(longLine))
		_, err := readResponse(reader)
		if err == nil {
			t.Error("expected error for line too long, got nil")
		} else if !strings.Contains(err.Error(), "line too long") {
			t.Errorf("expected 'line too long' error, got: %v", err)
		}
	})

	t.Run("too many lines", func(t *testing.T) {
		// Create a multi-line response with > 1000 lines
		var builder strings.Builder
		builder.WriteString("220-Start\r\n")
		for i := 0; i < 1001; i++ {
			builder.WriteString("220-Line\r\n")
		}
		builder.WriteString("220 End\r\n")
		reader := bufio.NewReader(strings.NewReader(builder.String()))
		_, err := readResponse(reader)
		if err == nil {
			t.Error("expected error for too many lines, got nil")
		} else if !strings.Contains(err.Error(), "too many response lines") {
			t.Errorf("expected 'too many response lines' error, got: %v", err)
		}
	})
}

func TestDownloadDir_Traversal(t *testing.T) {
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

	ms.handlers["EPSV"] = func(c *textproto.Conn, _ string) {
		_ = c.PrintfLine("%s", epsvResp)
	}

	ms.handlers["LIST"] = func(c *textproto.Conn, _ string) {
		_ = c.PrintfLine("150 File status okay.")
		dconn, err := ms.dataListener.Accept()
		if err != nil {
			t.Errorf("Mock server failed to accept data conn: %v", err)
			return
		}
		defer dconn.Close()

		// Send a malicious entry that attempts to traverse up and escape the target local directory
		fmt.Fprintf(dconn, "-rw-r--r-- 1 owner group 4 Jan 01 12:00 ../../../traversal.txt\r\n")
		_ = c.PrintfLine("226 Closing data connection.")
	}

	ms.start()
	defer ms.stop()

	c, err := Dial(ms.addr, WithTimeout(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Quit() }()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	
	// DownloadDir with root "" should attempt to download the current directory.
	// The malicious listing contains "../../../traversal.txt".
	// Without path traversal protection, it would write to tempDir/../traversal.txt.
	err = c.DownloadDir("", tempDir)
	if err == nil {
		t.Error("expected error due to path traversal, got nil")
	} else if !strings.Contains(err.Error(), "path traversal") && !strings.Contains(err.Error(), "security violation") {
		t.Errorf("expected path traversal / security violation error, got: %v", err)
	}

	// Verify that the file was NOT created outside tempDir
	traversalFile := filepath.Join(filepath.Dir(tempDir), "traversal.txt")
	if _, err := os.Stat(traversalFile); !os.IsNotExist(err) {
		t.Errorf("security check failed: traversal file was created at %s", traversalFile)
		_ = os.Remove(traversalFile) // Clean up if created
	}
}

func TestWalk_LoopPrevention(t *testing.T) {
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

	ms.handlers["EPSV"] = func(c *textproto.Conn, _ string) {
		_ = c.PrintfLine("%s", epsvResp)
	}

	// Any LIST command will return a single sub-directory entry of type "dir".
	// This simulates an infinite directory tree (either generated dynamically or via loops).
	ms.handlers["LIST"] = func(c *textproto.Conn, _ string) {
		_ = c.PrintfLine("150 File status okay.")
		dconn, err := ms.dataListener.Accept()
		if err != nil {
			t.Errorf("Mock server failed to accept data conn: %v", err)
			return
		}
		defer dconn.Close()

		// Return a sub-directory named "sub"
		fmt.Fprintf(dconn, "drwxr-xr-x 1 owner group 0 Jan 01 12:00 sub\r\n")
		_ = c.PrintfLine("226 Closing data connection.")
	}

	ms.start()
	defer ms.stop()

	c, err := Dial(ms.addr, WithTimeout(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Quit() }()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatal(err)
	}

	// Walk should return an error when maximum depth or infinite loop is detected,
	// rather than stack overflowing and crashing.
	err = c.Walk(".", func(path string, info *Entry, err error) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected Walk to fail with recursion depth limit / loop detection error, got nil")
	}
	if !strings.Contains(err.Error(), "depth exceeded") && !strings.Contains(err.Error(), "loop detected") && !strings.Contains(err.Error(), "security violation") {
		t.Errorf("expected security violation or depth exceeded error, got: %v", err)
	}
}


