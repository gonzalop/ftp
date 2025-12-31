package server

import (
	"bufio"
	"bytes"
	"io"
)

// asciiReader wraps an io.Reader and converts LF to CRLF on the fly for RETR (Download).
type asciiReader struct {
	r          *bufio.Reader
	prevWasCR  bool // To avoid doubling CR if file is already CRLF
	pending    byte // Pending byte to write (e.g. \n after we wrote \r)
	hasPending bool
}

func newASCIIReader(r io.Reader) *asciiReader {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return &asciiReader{
		r: br,
	}
}

func (r *asciiReader) fill() ([]byte, error) {
	peeked, _ := r.r.Peek(r.r.Buffered())
	if len(peeked) > 0 {
		return peeked, nil
	}
	// Buffer empty, try to ReadByte to trigger fill or catch EOF
	_, err := r.r.ReadByte()
	if err != nil {
		return nil, err
	}
	// Put it back to use the block logic
	_ = r.r.UnreadByte()
	peeked, _ = r.r.Peek(r.r.Buffered())
	if len(peeked) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	return peeked, nil
}

func (r *asciiReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	n := 0

	// Handle pending byte from previous Read
	if r.hasPending {
		p[n] = r.pending
		n++
		r.hasPending = false
		r.pending = 0
	}

	for n < len(p) {
		peeked, err := r.fill()
		if err != nil {
			if n > 0 {
				return n, nil
			}
			return 0, err
		}

		// Look for LF
		idx := bytes.IndexByte(peeked, '\n')
		if idx == -1 {
			// No LF, copy everything but be careful with trailing CR
			toCopy := len(peeked)
			if n+toCopy > len(p) {
				toCopy = len(p) - n
			}

			copy(p[n:], peeked[:toCopy])
			r.prevWasCR = (peeked[toCopy-1] == '\r')
			_, _ = r.r.Discard(toCopy)
			n += toCopy
		} else {
			// Found LF at idx.
			// Copy data BEFORE the LF.
			toCopy := idx
			if n+toCopy > len(p) {
				toCopy = len(p) - n
			}

			if toCopy > 0 {
				copy(p[n:], peeked[:toCopy])
				r.prevWasCR = (peeked[toCopy-1] == '\r')
				_, _ = r.r.Discard(toCopy)
				n += toCopy
			}

			if n >= len(p) {
				return n, nil
			}

			// Now we are at the LF in the reader.
			// Check if we need to insert CR.
			if r.prevWasCR {
				// Already has CR, just copy LF
				p[n] = '\n'
				n++
				_, _ = r.r.Discard(1)
				r.prevWasCR = false
			} else {
				// Insert CR
				p[n] = '\r'
				n++
				r.prevWasCR = true
				// Next byte should be LF. If we have space, write it.
				if n < len(p) {
					p[n] = '\n'
					n++
					_, _ = r.r.Discard(1)
					r.prevWasCR = false
				} else {
					// No space for LF, store as pending
					r.pending = '\n'
					r.hasPending = true
					_, _ = r.r.Discard(1)
					return n, nil
				}
			}
		}
	}

	return n, nil
}

// asciiWriter translates CRLF to LF for STOR (Upload).
// It reads from the network (CRLF) and provides a reader that yields LF.
type asciiWriter struct {
	r *bufio.Reader
}

func newASCIIWriter(r io.Reader) *asciiWriter {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return &asciiWriter{
		r: br,
	}
}

func (aw *asciiWriter) fill() ([]byte, error) {
	peeked, _ := aw.r.Peek(aw.r.Buffered())
	if len(peeked) > 0 {
		return peeked, nil
	}
	_, err := aw.r.ReadByte()
	if err != nil {
		return nil, err
	}
	_ = aw.r.UnreadByte()
	peeked, _ = aw.r.Peek(aw.r.Buffered())
	if len(peeked) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	return peeked, nil
}

func (aw *asciiWriter) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	for n < len(p) {
		peeked, err := aw.fill()
		if err != nil {
			if n > 0 {
				return n, nil
			}
			return 0, err
		}

		idx := bytes.IndexByte(peeked, '\r')
		if idx == -1 {
			toCopy := len(peeked)
			if n+toCopy > len(p) {
				toCopy = len(p) - n
			}
			copy(p[n:], peeked[:toCopy])
			_, _ = aw.r.Discard(toCopy)
			n += toCopy
		} else {
			// Copy up to CR
			toCopy := idx
			if n+toCopy > len(p) {
				toCopy = len(p) - n
			}
			if toCopy > 0 {
				copy(p[n:], peeked[:toCopy])
				_, _ = aw.r.Discard(toCopy)
				n += toCopy
			}

			if n >= len(p) {
				return n, nil
			}

			// We are at the CR. Check if CRLF.
			peeked, _ = aw.r.Peek(2)
			if len(peeked) >= 2 && peeked[1] == '\n' {
				// Skip CR
				_, _ = aw.r.Discard(1)
				// Next loop iteration will copy the LF
			} else if len(peeked) == 1 {
				// Only CR in buffer. Is it EOF?
				// Try to peek more.
				// If we can't get more data now, we MUST return to avoid blocking.
				// But we don't know if next byte is LF.
				// Safest is to return what we have and let next Read deal with it.
				return n, nil
			} else {
				// Single CR, copy it
				p[n] = '\r'
				n++
				_, _ = aw.r.Discard(1)
			}
		}
	}

	return n, nil
}
