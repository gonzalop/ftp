package server

import (
	"bytes"
	"io"
	"net"
	"testing"
)

// mockZeroCopyConn simulates a net.Conn that supports ReadFrom and WriteTo (like TCPConn)
type mockZeroCopyConn struct {
	net.Conn
	readFromCalled bool
	writeToCalled  bool
}

func (m *mockZeroCopyConn) ReadFrom(r io.Reader) (int64, error) {
	m.readFromCalled = true
	return io.Copy(m.Conn, r)
}

func (m *mockZeroCopyConn) WriteTo(w io.Writer) (int64, error) {
	m.writeToCalled = true
	return io.Copy(w, m.Conn)
}

// onlyReader wraps an io.Reader to hide other methods (like WriterTo)
type onlyReader struct {
	io.Reader
}

// onlyWriter wraps an io.Writer to hide other methods (like ReaderFrom)
type onlyWriter struct {
	io.Writer
}

func TestTrackingConnZeroCopy(t *testing.T) {
	t.Run("ReadFrom", func(t *testing.T) {
		client, serverConn := net.Pipe()
		defer client.Close()
		defer serverConn.Close()

		mock := &mockZeroCopyConn{Conn: serverConn}
		tc := &trackingConn{
			Conn:   mock,
			server: &Server{},
		}

		content := []byte("test content")
		src := &onlyReader{bytes.NewReader(content)}

		go func() {
			buf := make([]byte, len(content))
			_, _ = io.ReadFull(client, buf)
		}()

		_, err := io.Copy(tc, src)
		if err != nil {
			t.Errorf("io.Copy failed: %v", err)
		}

		if !mock.readFromCalled {
			t.Error("ReadFrom was not called on the underlying connection")
		}
	})

	t.Run("WriteTo", func(t *testing.T) {
		client, serverConn := net.Pipe()
		defer client.Close()
		defer serverConn.Close()

		mock := &mockZeroCopyConn{Conn: serverConn}
		tc := &trackingConn{
			Conn:   mock,
			server: &Server{},
		}

		content := []byte("upload content")
		var buf bytes.Buffer
		dst := &onlyWriter{&buf}

		go func() {
			_, _ = client.Write(content)
			client.Close() // Signal EOF
		}()

		_, err := io.Copy(dst, tc)
		if err != nil {
			t.Errorf("io.Copy failed: %v", err)
		}

		if !mock.writeToCalled {
			t.Error("WriteTo was not called on the underlying connection")
		}

		if buf.String() != string(content) {
			t.Errorf("Expected %q, got %q", string(content), buf.String())
		}
	})
}
