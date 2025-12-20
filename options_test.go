package ftp

import (
	"strings"
	"testing"
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
