package server

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTLSDataHandshakeTimeout checks that a client that opens the data
// connection but never starts TLS cannot block the session.
func TestTLSDataHandshakeTimeout(t *testing.T) {
	t.Parallel()
	certPath, keyPath, _, _ := generateCert(t, false, nil, nil)
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	fatalIfErr(t, err, "load cert")
	rootDir := t.TempDir()
	fatalIfErr(t, os.WriteFile(filepath.Join(rootDir, "f.txt"), []byte("secret"), 0644), "write file")
	driver, err := NewFSDriver(rootDir)
	fatalIfErr(t, err, "NewFSDriver")
	shortHandshake := func(s *Server) error {
		s.dataHandshakeTimeout = 300 * time.Millisecond
		return nil
	}
	_, addr := startRawServer(t, driver,
		WithTLS(&tls.Config{Certificates: []tls.Certificate{cert}}), shortHandshake)

	c := dialRaw(t, addr)
	c.cmd("AUTH TLS")
	tlsConn := tls.Client(c.conn, &tls.Config{InsecureSkipVerify: true})
	c.conn, c.r = tlsConn, bufio.NewReader(tlsConn)
	c.cmd("USER ftp")
	c.cmd("PASS x")
	c.cmd("PBSZ 0")
	c.cmd("PROT P")

	_ = c.epsv() // connect, but never handshake
	if r := c.cmd("RETR f.txt"); !strings.HasPrefix(r, "150 ") {
		t.Fatalf("RETR: got %q, want 150", r)
	}
	if r := c.reply(); !strings.HasPrefix(r, "425 ") {
		t.Fatalf("after stalled handshake: got %q, want 425", r)
	}
	if r := c.cmd("NOOP"); !strings.HasPrefix(r, "200 ") {
		t.Fatalf("NOOP: got %q, want 200", r)
	}
}

// TestTransferLongerThanTimeouts checks that the read/write timeouts and the
// idle timeout bound stalls, not the length of a transfer.
func TestTransferLongerThanTimeouts(t *testing.T) {
	t.Parallel()
	// The timeout is short for a test, but not shorter than the time Linux
	// takes to wake a writer blocked on a multi-megabyte send buffer.
	const timeout = 500 * time.Millisecond
	const size = 32 << 20
	const chunk = 64 << 10

	rootDir := t.TempDir()
	content := bytes.Repeat([]byte("0123456789abcdef"), size/16)
	fatalIfErr(t, os.WriteFile(filepath.Join(rootDir, "big.bin"), content, 0644), "write file")
	driver, err := NewFSDriver(rootDir,
		WithAuthenticator(func(_, _, _ string, _ net.IP) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	fatalIfErr(t, err, "NewFSDriver")
	_, addr := startRawServer(t, driver,
		WithReadTimeout(timeout), WithWriteTimeout(timeout), WithMaxIdleTime(timeout))

	// login opens a new session: the short idle timeout would close a
	// session shared by the subtests.
	login := func(t *testing.T) *rawConn {
		c := dialRaw(t, addr)
		c.cmd("USER ftp")
		c.cmd("PASS x")
		c.cmd("TYPE I")
		return c
	}

	t.Run("RETR", func(t *testing.T) {
		c := login(t)
		dc := c.epsv()
		if r := c.cmd("RETR big.bin"); !strings.HasPrefix(r, "150 ") {
			t.Fatalf("RETR: got %q, want 150", r)
		}
		start := time.Now()
		buf := make([]byte, chunk)
		var got int
		for {
			_ = dc.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, err := dc.Read(buf)
			got += n
			if err != nil {
				break
			}
			time.Sleep(2 * time.Millisecond)
		}
		if elapsed := time.Since(start); elapsed < 2*timeout {
			t.Logf("transfer took %v; it may not have exercised the timeout", elapsed)
		}
		if got != size {
			t.Errorf("got %d bytes, want %d", got, size)
		}
		if r := c.reply(); !strings.HasPrefix(r, "226 ") {
			t.Errorf("after RETR: got %q, want 226", r)
		}
	})

	t.Run("STOR", func(t *testing.T) {
		c := login(t)
		dc := c.epsv()
		if r := c.cmd("STOR up.bin"); !strings.HasPrefix(r, "150 ") {
			t.Fatalf("STOR: got %q, want 150", r)
		}
		for off := 0; off < size/2; off += chunk {
			_ = dc.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_, err := dc.Write(content[off : off+chunk])
			fatalIfErr(t, err, "write data")
			time.Sleep(5 * time.Millisecond)
		}
		dc.Close()
		if r := c.reply(); !strings.HasPrefix(r, "226 ") {
			t.Fatalf("after STOR: got %q, want 226", r)
		}
		fi, err := os.Stat(filepath.Join(rootDir, "up.bin"))
		fatalIfErr(t, err, "stat upload")
		if fi.Size() != size/2 {
			t.Errorf("uploaded %d bytes, want %d", fi.Size(), size/2)
		}
	})
}

// failCloseDriver returns files whose Close fails, like a backend that
// commits the upload on Close.
type failCloseDriver struct{ *FSDriver }

type failCloseContext struct{ ClientContext }

type failCloseFile struct{ io.ReadWriteCloser }

func (d *failCloseDriver) Authenticate(user, pass, host string, ip net.IP) (ClientContext, error) {
	ctx, err := d.FSDriver.Authenticate(user, pass, host, ip)
	if err != nil {
		return nil, err
	}
	return &failCloseContext{ClientContext: ctx}, nil
}

func (c *failCloseContext) OpenFile(path string, flag int) (io.ReadWriteCloser, error) {
	f, err := c.ClientContext.OpenFile(path, flag)
	if err != nil {
		return nil, err
	}
	return &failCloseFile{f}, nil
}

func (f *failCloseFile) Close() error {
	_ = f.ReadWriteCloser.Close()
	return errors.New("commit failed")
}

// TestSTORCloseError checks that a failed Close is reported instead of 226.
func TestSTORCloseError(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	fs, err := NewFSDriver(rootDir,
		WithAuthenticator(func(_, _, _ string, _ net.IP) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	fatalIfErr(t, err, "NewFSDriver")
	_, addr := startRawServer(t, &failCloseDriver{fs})

	c := dialRaw(t, addr)
	c.cmd("USER ftp")
	c.cmd("PASS x")
	dc := c.epsv()
	if r := c.cmd("STOR f.txt"); !strings.HasPrefix(r, "150 ") {
		t.Fatalf("STOR: got %q, want 150", r)
	}
	_, err = dc.Write([]byte("data"))
	fatalIfErr(t, err, "write data")
	dc.Close()
	if r := c.reply(); !strings.HasPrefix(r, "451 ") {
		t.Fatalf("after STOR: got %q, want 451", r)
	}
	if r := c.cmd("NOOP"); !strings.HasPrefix(r, "200 ") {
		t.Fatalf("NOOP: got %q, want 200", r)
	}
}
