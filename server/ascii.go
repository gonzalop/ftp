package server

import (
	"bufio"
	"io"
)

// asciiReader wraps an io.Reader and converts LF to CRLF on the fly for RETR (Download).
type asciiReader struct {
	r          io.Reader
	prevWasCR  bool // To avoid doubling CR if file is already CRLF
	pending    byte // Pending byte to write (e.g. \n after we wrote \r)
	hasPending bool
}

func newASCIIReader(r io.Reader) *asciiReader {
	return &asciiReader{
		r: r,
	}
}

func (r *asciiReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	n := 0

	if r.hasPending {
		p[n] = r.pending
		n++
		r.hasPending = false
		r.pending = 0
	}

	br, ok := r.r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r.r)
		r.r = br
	}

	for n < len(p) {
		b, err := br.ReadByte()
		if err != nil {
			return n, err
		}

		if b == '\n' && !r.prevWasCR {
			// Emit CR
			p[n] = '\r'
			n++
			r.prevWasCR = true

			// We need to output \n next.
			if n < len(p) {
				p[n] = '\n'
				n++
				r.prevWasCR = false
			} else {
				r.pending = '\n'
				r.hasPending = true
				return n, nil
			}
		} else {
			p[n] = b
			n++
			r.prevWasCR = (b == '\r')
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
	return &asciiWriter{
		r: bufio.NewReader(r),
	}
}

func (aw *asciiWriter) Read(p []byte) (n int, err error) {
	for n < len(p) {
		b, err := aw.r.ReadByte()
		if err != nil {
			return n, err
		}

		if b == '\r' {
			next, err := aw.r.Peek(1)
			if err == nil && next[0] == '\n' {
				// CRLF -> skip CR
				continue
			}
		}

		p[n] = b
		n++
	}
	return n, nil
}
