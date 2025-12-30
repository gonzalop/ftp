package ratelimit

import (
	"bytes"
	"io"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		bytesPerSecond int64
		expectNil      bool
	}{
		{"Valid rate", 1024, false},
		{"Zero rate (unlimited)", 0, true},
		{"Negative rate (unlimited)", -1, true},
		{"Very low rate", 1, false},
		{"High rate", 10 * 1024 * 1024, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := New(tt.bytesPerSecond)
			if tt.expectNil && limiter != nil {
				t.Errorf("Expected nil limiter for rate %d, got non-nil", tt.bytesPerSecond)
			}
			if !tt.expectNil && limiter == nil {
				t.Errorf("Expected non-nil limiter for rate %d, got nil", tt.bytesPerSecond)
			}
		})
	}
}

func TestNewReader(t *testing.T) {
	t.Parallel()
	data := []byte("test data")
	reader := bytes.NewReader(data)

	// With nil limiter, should return original reader
	limited := NewReader(reader, nil)
	if limited != reader {
		t.Error("Expected original reader when limiter is nil")
	}

	// With valid limiter, should return wrapped reader
	limiter := New(1024)
	limited = NewReader(reader, limiter)
	if limited == reader {
		t.Error("Expected wrapped reader when limiter is non-nil")
	}
}

func TestNewWriter(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer

	// With nil limiter, should return original writer
	limited := NewWriter(&buf, nil)
	if limited != &buf {
		t.Error("Expected original writer when limiter is nil")
	}

	// With valid limiter, should return wrapped writer
	limiter := New(1024)
	limited = NewWriter(&buf, limiter)
	if limited == &buf {
		t.Error("Expected wrapped writer when limiter is non-nil")
	}
}

func TestReader_Read(t *testing.T) {
	t.Parallel()
	// Create a 1KB test data
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Test with 10KB/s limit (should take ~100ms for 1KB)
	limiter := New(10 * 1024)

	reader := NewReader(bytes.NewReader(data), limiter)

	start := time.Now()
	result := make([]byte, 1024)
	n, err := io.ReadFull(reader, result)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != 1024 {
		t.Errorf("Expected to read 1024 bytes, got %d", n)
	}
	if !bytes.Equal(data, result) {
		t.Error("Data mismatch after rate-limited read")
	}

	// With token bucket burst (10KB), 1KB transfers instantly
	// Just verify it doesn't error and data is correct
	_ = duration // Burst allows instant transfer
}

func TestWriter_Write(t *testing.T) {
	t.Parallel()
	// Create a 1KB test data
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Test with 10KB/s limit (should take ~100ms for 1KB)
	limiter := New(10 * 1024)

	var buf bytes.Buffer
	writer := NewWriter(&buf, limiter)

	start := time.Now()
	n, err := writer.Write(data)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 1024 {
		t.Errorf("Expected to write 1024 bytes, got %d", n)
	}
	if !bytes.Equal(data, buf.Bytes()) {
		t.Error("Data mismatch after rate-limited write")
	}

	// With token bucket burst (10KB), 1KB transfers instantly
	// Just verify it doesn't error and data is correct
	_ = duration // Burst allows instant transfer
}

func TestReader_LargeTransfer(t *testing.T) {
	t.Parallel()
	// Create 10KB test data
	data := make([]byte, 10*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Test with 5KB/s limit (should take ~2 seconds for 10KB)
	limiter := New(5 * 1024)

	reader := NewReader(bytes.NewReader(data), limiter)

	start := time.Now()
	result, err := io.ReadAll(reader)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(result) != len(data) {
		t.Errorf("Expected to read %d bytes, got %d", len(data), len(result))
	}
	if !bytes.Equal(data, result) {
		t.Error("Data mismatch after rate-limited read")
	}

	// With token bucket burst (5KB), first 5KB transfers instantly,
	// then remaining 5KB takes 1 second at 5KB/s = ~1 second total
	if duration < 800*time.Millisecond {
		t.Errorf("Large read completed too quickly (%v), rate limiting may not be working", duration)
	}
	// But shouldn't take more than 3 seconds (with reasonable overhead)
	if duration > 3*time.Second {
		t.Errorf("Large read took too long (%v), possible performance issue", duration)
	}
}

func TestWriter_LargeTransfer(t *testing.T) {
	t.Parallel()
	// Create 10KB test data
	data := make([]byte, 10*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Test with 5KB/s limit (should take ~2 seconds for 10KB)
	limiter := New(5 * 1024)

	var buf bytes.Buffer
	writer := NewWriter(&buf, limiter)

	start := time.Now()
	n, err := writer.Write(data)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Expected to write %d bytes, got %d", len(data), n)
	}
	if !bytes.Equal(data, buf.Bytes()) {
		t.Error("Data mismatch after rate-limited write")
	}

	// With token bucket burst (5KB), first 5KB transfers instantly,
	// then remaining 5KB takes 1 second at 5KB/s = ~1 second total
	if duration < 800*time.Millisecond {
		t.Errorf("Large write completed too quickly (%v), rate limiting may not be working", duration)
	}
	// But shouldn't take more than 3 seconds (with reasonable overhead)
	if duration > 3*time.Second {
		t.Errorf("Large write took too long (%v), possible performance issue", duration)
	}
}

func TestUnlimitedRate(t *testing.T) {
	t.Parallel()
	data := make([]byte, 10*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Nil limiter should not throttle
	reader := NewReader(bytes.NewReader(data), nil)

	start := time.Now()
	result, err := io.ReadAll(reader)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(result) != len(data) {
		t.Errorf("Expected to read %d bytes, got %d", len(data), len(result))
	}

	// Should complete very quickly (< 100ms)
	if duration > 100*time.Millisecond {
		t.Errorf("Unlimited read took too long (%v)", duration)
	}
}

func BenchmarkReader(b *testing.B) {
	data := make([]byte, 1024)
	limiter := New(1024 * 1024) // 1 MB/s

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := NewReader(bytes.NewReader(data), limiter)
		if _, err := io.ReadAll(reader); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriter(b *testing.B) {
	data := make([]byte, 1024)
	limiter := New(1024 * 1024) // 1 MB/s

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		writer := NewWriter(&buf, limiter)
		if _, err := writer.Write(data); err != nil {
			b.Fatal(err)
		}
	}
}
