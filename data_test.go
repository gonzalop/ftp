package ftp

import (
	"net"
	"testing"
	"time"
)

func TestResolveDataAddr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		pasvAddr    string
		controlHost string
		wantAddr    string
	}{
		{
			name:        "normal address",
			pasvAddr:    "192.168.1.5:12345",
			controlHost: "10.0.0.1",
			wantAddr:    "192.168.1.5:12345",
		},
		{
			name:        "zero address",
			pasvAddr:    "0.0.0.0:12345",
			controlHost: "10.0.0.1",
			wantAddr:    "10.0.0.1:12345",
		},
		{
			name:        "invalid address",
			pasvAddr:    "invalid",
			controlHost: "10.0.0.1",
			wantAddr:    "invalid", // Or handle error? The split might fail.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDataAddr(tt.pasvAddr, tt.controlHost)
			if got != tt.wantAddr {
				t.Errorf("resolveDataAddr() = %v, want %v", got, tt.wantAddr)
			}
		})
	}
}

func TestFormatEPRT(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		addr    string
		want    string
		wantErr bool
	}{
		{
			name: "IPv4",
			addr: "127.0.0.1:12345",
			want: "|1|127.0.0.1|12345|",
		},
		{
			name: "IPv6",
			addr: "[::1]:12345",
			want: "|2|::1|12345|",
		},
		{
			name:    "Invalid",
			addr:    "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatEPRT(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("formatEPRT() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("formatEPRT() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestActiveDataConn_Coverage(t *testing.T) {
	t.Parallel()
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
