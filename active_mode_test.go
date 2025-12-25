package ftp

import (
	"net"
	"testing"
	"time"
)

func TestActiveDataConn_Coverage(t *testing.T) {
	// Setup a dummy listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// We don't defer ln.Close() because adc.Close() closes it

	// Create the activeDataConn
	adc := &activeDataConn{
		listener: ln,
		timeout:  time.Second,
	}

	// Trigger accept by dialing it in a goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			return
		}
		defer conn.Close()
		// Read to drain "test" write
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
	}()

	// 1. Test Write (triggers accept)
	if _, err := adc.Write([]byte("test")); err != nil {
		t.Errorf("Write failed: %v", err)
	}

	// 2. Test LocalAddr/RemoteAddr
	if adc.LocalAddr() == nil {
		t.Error("LocalAddr is nil")
	}
	if adc.RemoteAddr() == nil {
		t.Error("RemoteAddr is nil")
	}

	// 3. Test SetDeadline methods
	if err := adc.SetDeadline(time.Now().Add(time.Hour)); err != nil {
		t.Errorf("SetDeadline failed: %v", err)
	}
	if err := adc.SetReadDeadline(time.Now().Add(time.Hour)); err != nil {
		t.Errorf("SetReadDeadline failed: %v", err)
	}
	if err := adc.SetWriteDeadline(time.Now().Add(time.Hour)); err != nil {
		t.Errorf("SetWriteDeadline failed: %v", err)
	}

	// Close adc (closes listener and conn)
	if err := adc.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	<-done
}
