package server

import (
	"strings"
	"testing"
)

func TestRedactPath(t *testing.T) {
	t.Parallel()
	// Helper function for standard middle component redaction
	redactMiddle := func(path string) string {
		if path == "" {
			return path
		}
		parts := strings.Split(path, "/")
		if len(parts) <= 3 {
			return path
		}
		for i := 2; i < len(parts)-1; i++ {
			if parts[i] != "" {
				parts[i] = "*"
			}
		}
		return strings.Join(parts, "/")
	}

	tests := []struct {
		name     string
		redactor PathRedactor
		input    string
		expected string
	}{
		{"Disabled", nil, "/home/user/documents/file.txt", "/home/user/documents/file.txt"},
		{"Enabled_LongPath", redactMiddle, "/home/user/documents/file.txt", "/home/*/*/file.txt"},
		{"Enabled_ShortPath", redactMiddle, "/home/file.txt", "/home/file.txt"}, // Too short to redact
		{"Enabled_VeryShortPath", redactMiddle, "/file.txt", "/file.txt"},       // Too short to redact
		{"Empty", redactMiddle, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{pathRedactor: tt.redactor}
			result := s.redactPath(tt.input)
			if result != tt.expected {
				t.Errorf("redactPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRedactIP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		enabled  bool
		input    string
		expected string
	}{
		{"Disabled_IPv4", false, "192.168.1.100", "192.168.1.100"},
		{"Enabled_IPv4", true, "192.168.1.100", "192.168.1.xxx"},
		{"Enabled_IPv6", true, "2001:db8::1", "2001:db8::xxx"},
		{"Enabled_IPv6_Long", true, "2001:0db8:85a3:0000:0000:8a2e:0370:7334", "2001:0db8:85a3:0000:0000:8a2e:0370:xxx"},
		{"Empty", true, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{redactIPs: tt.enabled}
			result := s.redactIP(tt.input)
			if result != tt.expected {
				t.Errorf("redactIP(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestWithPathRedactor(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)

	redactor := func(path string) string {
		return "/redacted/" + path
	}

	s, err := NewServer(":0",
		WithDriver(driver),
		WithPathRedactor(redactor),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.pathRedactor == nil {
		t.Error("Expected pathRedactor to be set")
	}
}

func TestWithRedactIPs(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)

	s, err := NewServer(":0",
		WithDriver(driver),
		WithRedactIPs(true),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if !s.redactIPs {
		t.Error("Expected redactIPs to be true")
	}
}
