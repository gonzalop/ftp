package ftp

import "io"

// ProgressReader wraps an io.Reader and reports progress via a callback.
type ProgressReader struct {
	// Reader is the underlying reader
	Reader io.Reader

	// Callback is called after each Read with the total bytes transferred
	Callback func(bytesTransferred int64)

	// total tracks the total bytes read
	total int64
}

// Read implements io.Reader.
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.total += int64(n)
	if pr.Callback != nil && n > 0 {
		pr.Callback(pr.total)
	}
	return n, err
}

// ProgressWriter wraps an io.Writer and reports progress via a callback.
type ProgressWriter struct {
	// Writer is the underlying writer
	Writer io.Writer

	// Callback is called after each Write with the total bytes transferred
	Callback func(bytesTransferred int64)

	// total tracks the total bytes written
	total int64
}

// Write implements io.Writer.
func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.Writer.Write(p)
	pw.total += int64(n)
	if pw.Callback != nil && n > 0 {
		pw.Callback(pw.total)
	}
	return n, err
}
