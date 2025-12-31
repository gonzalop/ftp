package server

import (
	"net"
	"testing"
)

// mockListenerFactory is a test implementation of the ListenerFactory interface
type mockListenerFactory struct {
	listenFunc func(network, address string) (net.Listener, error)
}

func (m *mockListenerFactory) Listen(network, address string) (net.Listener, error) {
	if m.listenFunc != nil {
		return m.listenFunc(network, address)
	}
	// Default: use standard net.Listen
	return net.Listen(network, address)
}

// TestWithListenerFactory tests that the WithListenerFactory option is accepted
func TestWithListenerFactory(t *testing.T) {
	// Create a mock listener factory
	mockFactory := &mockListenerFactory{}

	// Create a server with the custom listener factory option
	driver, err := NewFSDriver(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}

	s, err := NewServer(":0",
		WithDriver(driver),
		WithListenerFactory(mockFactory),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.listenerFactory != mockFactory {
		t.Error("listenerFactory was not set correctly")
	}
}

// TestDefaultListenerFactory tests that the default listener factory is set
func TestDefaultListenerFactory(t *testing.T) {
	driver, err := NewFSDriver(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}

	s, err := NewServer(":0", WithDriver(driver))
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.listenerFactory == nil {
		t.Error("listenerFactory should be set to default")
	}

	if _, ok := s.listenerFactory.(*DefaultListenerFactory); !ok {
		t.Error("listenerFactory should be DefaultListenerFactory")
	}
}

// TestWithDisableCommands tests that commands can be disabled
func TestWithDisableCommands(t *testing.T) {
	driver, err := NewFSDriver(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}

	s, err := NewServer(":0",
		WithDriver(driver),
		WithDisableCommands("PORT", "EPRT"),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if !s.disabledCommands["PORT"] {
		t.Error("PORT command should be disabled")
	}

	if !s.disabledCommands["EPRT"] {
		t.Error("EPRT command should be disabled")
	}

	// Test case insensitivity
	if !s.disabledCommands["PORT"] {
		t.Error("PORT command should be disabled (case insensitive)")
	}
}

// TestDisabledCommandsNil tests that disabledCommands is nil by default
func TestDisabledCommandsNil(t *testing.T) {
	driver, err := NewFSDriver(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}

	s, err := NewServer(":0", WithDriver(driver))
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.disabledCommands != nil {
		t.Error("disabledCommands should be nil by default")
	}
}

// TestPredefinedCommandGroups tests that predefined command groups are properly defined
func TestPredefinedCommandGroups(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		minLen   int
	}{
		{"LegacyCommands", LegacyCommands, 5},
		{"ActiveModeCommands", ActiveModeCommands, 2},
		{"WriteCommands", WriteCommands, 8},
		{"SiteCommands", SiteCommands, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.commands) < tt.minLen {
				t.Errorf("%s should have at least %d commands, got %d", tt.name, tt.minLen, len(tt.commands))
			}
		})
	}
}

// TestWithDisableCommandsUsingGroups tests using predefined command groups
func TestWithDisableCommandsUsingGroups(t *testing.T) {
	driver, err := NewFSDriver(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}

	// Test with ActiveModeCommands
	s, err := NewServer(":0",
		WithDriver(driver),
		WithDisableCommands(ActiveModeCommands...),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if !s.disabledCommands["PORT"] {
		t.Error("PORT should be disabled")
	}
	if !s.disabledCommands["EPRT"] {
		t.Error("EPRT should be disabled")
	}

	// Test with WriteCommands
	s2, err := NewServer(":0",
		WithDriver(driver),
		WithDisableCommands(WriteCommands...),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if !s2.disabledCommands["STOR"] {
		t.Error("STOR should be disabled")
	}
	if !s2.disabledCommands["DELE"] {
		t.Error("DELE should be disabled")
	}

	// Test with LegacyCommands
	s3, err := NewServer(":0",
		WithDriver(driver),
		WithDisableCommands(LegacyCommands...),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if !s3.disabledCommands["XCWD"] {
		t.Error("XCWD should be disabled")
	}
	if !s3.disabledCommands["XPWD"] {
		t.Error("XPWD should be disabled")
	}
}
