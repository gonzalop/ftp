package server

import (
	"bytes"
	"io"
	"testing"
)

func TestTelnetReader(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "Normal command",
			input:    []byte("USER anonymous\r\n"),
			expected: []byte("USER anonymous\r\n"),
		},
		{
			name:     "IAC WILL",
			input:    []byte{telnetIAC, telnetWILL, 0x01, 'A', 'B', 'C'},
			expected: []byte("ABC"),
		},
		{
			name:     "IAC WONT",
			input:    []byte{telnetIAC, telnetWONT, 0x02, 'D', 'E', 'F'},
			expected: []byte("DEF"),
		},
		{
			name:     "IAC DO",
			input:    []byte{telnetIAC, telnetDO, 0x03, 'G', 'H', 'I'},
			expected: []byte("GHI"),
		},
		{
			name:     "IAC DONT",
			input:    []byte{telnetIAC, telnetDONT, 0x04, 'J', 'K', 'L'},
			expected: []byte("JKL"),
		},
		{
			name:     "IAC Escaping",
			input:    []byte{'X', telnetIAC, telnetIAC, 'Y'}, // 0xFF 0xFF -> 0xFF
			expected: []byte{'X', telnetIAC, 'Y'},
		},
		{
			name:     "Mixed sequence",
			input:    []byte{telnetIAC, telnetDO, 0x01, 'U', 'S', 'E', 'R', ' ', telnetIAC, telnetIAC, '\r', '\n'},
			expected: []byte("USER \xff\r\n"),
		},
		{
			name:     "Split negotiation",
			input:    []byte{telnetIAC, telnetDO, 0x01, 'O', 'K'},
			expected: []byte("OK"),
		},
		{
			name:     "Unknown command (2 byte)",
			input:    []byte{telnetIAC, 0xF0, 'A'}, // 0xF0 is not WILL/WONT/DO/DONT
			expected: []byte("A"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTelnetReader(bytes.NewReader(tt.input))
			buf := new(bytes.Buffer)
			_, err := io.Copy(buf, r)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !bytes.Equal(buf.Bytes(), tt.expected) {
				t.Errorf("expected %q, got %q", tt.expected, buf.Bytes())
			}
		})
	}
}
