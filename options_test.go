package ftp

import (
	"strings"
	"testing"
	"time"
)

func TestDial_ExclusiveTLS(t *testing.T) {
	// Test that Explicit + Implicit fails
	_, err := Dial("ftp.example.com:21", WithExplicitTLS(nil), WithImplicitTLS(nil))
	if err == nil {
		t.Error("Expected error when combining Explicit and Implicit TLS, got nil")
	} else if !strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("Expected 'cannot be combined' error, got: %v", err)
	}

	// Test that Implicit + Explicit fails
	_, err = Dial("ftp.example.com:21", WithImplicitTLS(nil), WithExplicitTLS(nil))
	if err == nil {
		t.Error("Expected error when combining Implicit and Explicit TLS, got nil")
	} else if !strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("Expected 'cannot be combined' error, got: %v", err)
	}
}

func TestDial_MultipleSameTLS(t *testing.T) {
	// Test that multiple Explicit checks are fine (last one wins, no error)
	// Note: Verify logic allows this? Yes, explicit doesn't check against explicit.
	_, err := Dial("ftp.example.com:21", WithExplicitTLS(nil), WithExplicitTLS(nil))
	// Dial will likely fail to connect to example.com, but it shouldn't be the option error
	if err != nil && strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("Did not expect conflict error for multiple Explicit TLS: %v", err)
	}

	// Same for Implicit
	_, err = Dial("ftp.example.com:21", WithImplicitTLS(nil), WithImplicitTLS(nil))
	if err != nil && strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("Did not expect conflict error for multiple Implicit TLS: %v", err)
	}
}

func TestWithIdleTimeout(t *testing.T) {
	// Test that idle timeout is set correctly
	// We can't fully test the functionality without a real server,
	// but we can verify the option sets the field
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{"5 minutes", 5 * time.Minute},
		{"30 seconds", 30 * time.Second},
		{"disabled", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a client with the option (will fail to connect, but that's ok)
			c := &Client{}
			opt := WithIdleTimeout(tt.timeout)
			if err := opt(c); err != nil {
				t.Fatalf("WithIdleTimeout failed: %v", err)
			}

			if c.idleTimeout != tt.timeout {
				t.Errorf("Expected idleTimeout %v, got %v", tt.timeout, c.idleTimeout)
			}
		})
	}
}
