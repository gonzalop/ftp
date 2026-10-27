package server

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestASCIIWriterCRLF(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"crlf", "a\r\nb\r\n", "a\nb\n"},
		{"lone cr mid", "a\rb", "a\rb"},
		{"trailing lone cr", "x\r", "x\r"},
		{"only cr", "\r", "\r"},
		{"crlf then lone cr", "a\r\nb\r", "a\nb\r"},
		{"cr cr lf", "a\r\r\nb", "a\r\nb"},
	}
	for _, tt := range tests {
		for _, oneByte := range []bool{false, true} {
			var src io.Reader = strings.NewReader(tt.in)
			if oneByte {
				src = iotest.OneByteReader(src)
			}
			r := newASCIIWriter(src)

			done := make(chan string, 1)
			go func() {
				b, _ := io.ReadAll(r)
				done <- string(b)
			}()
			select {
			case got := <-done:
				if got != tt.want {
					t.Errorf("%s (oneByte=%v): got %q, want %q", tt.name, oneByte, got, tt.want)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%s (oneByte=%v): read did not finish", tt.name, oneByte)
			}
		}
	}
}
