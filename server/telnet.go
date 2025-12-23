package server

import (
	"bufio"
	"io"
)

const (
	// telnetIAC is Interpret As Command
	telnetIAC = 0xFF
	// telnetWILL negotiation command
	telnetWILL = 0xFB
	// telnetWONT negotiation command
	telnetWONT = 0xFC
	// telnetDO negotiation command
	telnetDO = 0xFD
	// telnetDONT negotiation command
	telnetDONT = 0xFE
)

// telnetReader is a reader that filters out Telnet commands.
// It implements the io.Reader interface.
type telnetReader struct {
	reader *bufio.Reader
}

// newTelnetReader creates a new telnetReader.
func newTelnetReader(r io.Reader) *telnetReader {
	return &telnetReader{
		reader: bufio.NewReader(r),
	}
}

// Read reads bytes from the underlying reader, filtering out Telnet commands.
func (t *telnetReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	for n < len(p) {
		// If we have read some bytes and there are no more buffered, return what we have.
		// This prevents blocking when the upstream reader is waiting for network input
		// but we already have valid data to return.
		if n > 0 && t.reader.Buffered() == 0 {
			return n, nil
		}

		b, err := t.reader.ReadByte()
		if err != nil {
			// If we have read some bytes, return them with nil error.
			// The error will be returned on the next call.
			if n > 0 {
				return n, nil
			}
			return n, err
		}

		if b == telnetIAC {
			// Peek to see the next byte
			next, err := t.reader.ReadByte()
			if err != nil {
				return n, err
			}

			if next == telnetIAC {
				// Escaped 0xFF, keep it
				p[n] = telnetIAC
				n++
				continue
			}

			// Handle Telnet commands
			switch next {
			case telnetWILL, telnetWONT, telnetDO, telnetDONT:
				// These are 3-byte sequences (IAC CMD OPT), read the third byte
				_, err := t.reader.ReadByte()
				if err != nil {
					return n, err
				}
			default:
				// Other commands are 2 bytes (IAC CMD), we already read both.
				// We ignore them.
			}

			continue
		}

		// Regular byte
		p[n] = b
		n++
	}

	return n, nil
}
