// Package ratelimit provides a stdlib-only token bucket rate limiter
// for bandwidth throttling in FTP transfers.
//
// This package is used internally by both the FTP client and server
// to limit transfer speeds and prevent network saturation.
package ratelimit

import (
	"io"
	"sync"
	"time"
)

// Limiter implements a token bucket rate limiter using stdlib only.
// It limits the rate of data transfer to a specified bytes per second.
type Limiter struct {
	ticker       *time.Ticker
	bytesPerTick int64
	stopChan     chan struct{}
	mu           sync.Mutex
	stopped      bool
}

// New creates a new rate limiter with the specified bytes per second limit.
// The limiter uses a token bucket algorithm with a 100ms tick interval.
func New(bytesPerSecond int64) *Limiter {
	if bytesPerSecond <= 0 {
		return nil
	}

	// Use 100ms tick interval for good balance between accuracy and overhead
	tickInterval := 100 * time.Millisecond
	bytesPerTick := bytesPerSecond / 10 // 10 ticks per second

	// Ensure at least 1 byte per tick for very low limits
	if bytesPerTick < 1 {
		bytesPerTick = 1
	}

	rl := &Limiter{
		ticker:       time.NewTicker(tickInterval),
		bytesPerTick: bytesPerTick,
		stopChan:     make(chan struct{}),
	}

	return rl
}

// wait blocks until the next tick, allowing bytesPerTick bytes to be transferred.
func (rl *Limiter) wait() {
	if rl == nil {
		return
	}

	select {
	case <-rl.ticker.C:
		// Token available, proceed
	case <-rl.stopChan:
		// Limiter stopped
	}
}

// stop stops the rate limiter and releases resources.
func (rl *Limiter) Stop() {
	if rl == nil {
		return
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.stopped {
		return
	}

	rl.stopped = true
	rl.ticker.Stop()
	close(rl.stopChan)
}

// rateLimitedReader wraps an io.Reader to limit read speed.
type reader struct {
	r       io.Reader
	limiter *Limiter
}

// newRateLimitedReader creates a new rate-limited reader.
// If limiter is nil, returns the original reader unchanged.
func NewReader(r io.Reader, limiter *Limiter) io.Reader {
	if limiter == nil {
		return r
	}
	return &reader{
		r:       r,
		limiter: limiter,
	}
}

// Read implements io.Reader with rate limiting.
func (r *reader) Read(p []byte) (n int, err error) {
	// Limit read size to bytes per tick
	readSize := len(p)
	if int64(readSize) > r.limiter.bytesPerTick {
		readSize = int(r.limiter.bytesPerTick)
	}

	// Wait for next tick
	r.limiter.wait()

	// Read up to allowed bytes
	return r.r.Read(p[:readSize])
}

// rateLimitedWriter wraps an io.Writer to limit write speed.
type writer struct {
	w       io.Writer
	limiter *Limiter
}

// newRateLimitedWriter creates a new rate-limited writer.
// If limiter is nil, returns the original writer unchanged.
func NewWriter(w io.Writer, limiter *Limiter) io.Writer {
	if limiter == nil {
		return w
	}
	return &writer{
		w:       w,
		limiter: limiter,
	}
}

// Write implements io.Writer with rate limiting.
func (w *writer) Write(p []byte) (n int, err error) {
	// Write in chunks limited by bytes per tick
	totalWritten := 0
	for totalWritten < len(p) {
		// Calculate chunk size
		remaining := len(p) - totalWritten
		chunkSize := remaining
		if int64(chunkSize) > w.limiter.bytesPerTick {
			chunkSize = int(w.limiter.bytesPerTick)
		}

		// Wait for next tick
		w.limiter.wait()

		// Write chunk
		written, err := w.w.Write(p[totalWritten : totalWritten+chunkSize])
		totalWritten += written
		if err != nil {
			return totalWritten, err
		}
	}

	return totalWritten, nil
}
