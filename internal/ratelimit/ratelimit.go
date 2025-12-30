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
//
// The token bucket algorithm allows for burst transfers up to the bucket
// capacity while maintaining the average rate over time.
type Limiter struct {
	rate       float64   // bytes per second
	burst      float64   // bucket capacity (max tokens)
	tokens     float64   // current available tokens
	lastUpdate time.Time // last time tokens were updated
	mu         sync.Mutex
}

// New creates a new rate limiter with the specified bytes per second limit.
// The limiter uses a token bucket algorithm with burst capacity equal to
// one second worth of data, allowing short bursts while maintaining the
// average rate over time.
func New(bytesPerSecond int64) *Limiter {
	if bytesPerSecond <= 0 {
		return nil
	}

	rate := float64(bytesPerSecond)
	return &Limiter{
		rate:       rate,
		burst:      rate, // Allow 1 second burst
		tokens:     rate, // Start with full bucket
		lastUpdate: time.Now(),
	}
}

// take attempts to consume n tokens from the bucket.
// If insufficient tokens are available, it sleeps for the minimum time needed.
func (rl *Limiter) take(n int) {
	if rl == nil || n <= 0 {
		return
	}

	rl.mu.Lock()

	now := time.Now()
	elapsed := now.Sub(rl.lastUpdate).Seconds()

	// Add tokens based on elapsed time
	rl.tokens += elapsed * rl.rate
	if rl.tokens > rl.burst {
		rl.tokens = rl.burst
	}
	rl.lastUpdate = now

	// If we have enough tokens, consume and return immediately
	tokensNeeded := float64(n)
	if rl.tokens >= tokensNeeded {
		rl.tokens -= tokensNeeded
		rl.mu.Unlock()
		return
	}

	// Not enough tokens - calculate minimum wait time
	tokensShort := tokensNeeded - rl.tokens
	waitDuration := time.Duration(tokensShort/rl.rate*1e9) * time.Nanosecond

	// Cap wait time at 1 second to avoid excessive blocking
	const maxWait = time.Second
	if waitDuration > maxWait {
		waitDuration = maxWait
	}

	rl.mu.Unlock()

	// Sleep for the required time
	time.Sleep(waitDuration)

	// After sleeping, update tokens and consume
	rl.mu.Lock()
	now = time.Now()
	elapsed = now.Sub(rl.lastUpdate).Seconds()
	rl.tokens += elapsed * rl.rate
	if rl.tokens > rl.burst {
		rl.tokens = rl.burst
	}
	rl.lastUpdate = now

	// Consume what we can (might be less than requested if we hit max wait)
	if rl.tokens >= tokensNeeded {
		rl.tokens -= tokensNeeded
	} else {
		rl.tokens = 0 // Consume all available
	}
	rl.mu.Unlock()
}

// rateLimitedReader wraps an io.Reader to limit read speed.
type reader struct {
	r       io.Reader
	limiter *Limiter
}

// NewReader creates a new rate-limited reader.
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
	if len(p) == 0 {
		return 0, nil
	}

	// Limit chunk size to avoid excessive waits
	// Use smaller chunks for better rate limiting accuracy
	const maxChunkSize = 8 * 1024 // 8KB chunks
	readSize := len(p)
	if readSize > maxChunkSize {
		readSize = maxChunkSize
	}

	// Consume tokens for this read
	r.limiter.take(readSize)

	// Read the data
	return r.r.Read(p[:readSize])
}

// rateLimitedWriter wraps an io.Writer to limit write speed.
type writer struct {
	w       io.Writer
	limiter *Limiter
}

// NewWriter creates a new rate-limited writer.
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
	// For writes, we consume tokens before writing to apply backpressure
	// Write in reasonable chunks to balance between overhead and responsiveness
	const maxChunkSize = 64 * 1024 // 64KB chunks

	totalWritten := 0
	for totalWritten < len(p) {
		// Calculate chunk size
		remaining := len(p) - totalWritten
		chunkSize := remaining
		if chunkSize > maxChunkSize {
			chunkSize = maxChunkSize
		}

		// Consume tokens before writing
		w.limiter.take(chunkSize)

		// Write chunk
		written, err := w.w.Write(p[totalWritten : totalWritten+chunkSize])
		totalWritten += written
		if err != nil {
			return totalWritten, err
		}
	}

	return totalWritten, nil
}
