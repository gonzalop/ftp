package server

import (
	"testing"
	"time"
)

func TestListenAndServe(t *testing.T) {
	// Use a random port
	addr := "127.0.0.1:0"
	rootDir := t.TempDir()

	// Start server in a goroutine
	// Since ListenAndServe blocks, we run it async.
	// However, we can't easily shut it down remotely with this simple helper unless we close the listener?
	// The helper doesn't return the server instance, which makes testing graceful shutdown hard.
	// But for this test, we just want to see if it starts up correctly.
	// We'll trust that if it returns an error immediately, it failed.

	errChan := make(chan error, 1)
	go func() {
		errChan <- ListenAndServe(addr, rootDir)
	}()

	select {
	case err := <-errChan:
		t.Fatalf("ListenAndServe failed immediately: %v", err)
	case <-time.After(200 * time.Millisecond):
		// Assume it started successfully if it hasn't returned in 200ms
	}
}
