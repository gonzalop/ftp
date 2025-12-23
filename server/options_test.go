package server

import (
	"crypto/tls"
	"log/slog"
	"os"
	"testing"
	"time"
)

// TestWithDriver tests the WithDriver option
func TestWithDriver(t *testing.T) {
	tempDir := t.TempDir()
	driver, err := NewFSDriver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Test successful driver setting
	s, err := NewServer(":0", WithDriver(driver))
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	if s.driver == nil {
		t.Error("Driver not set")
	}

	// Test duplicate driver setting
	_, err = NewServer(":0",
		WithDriver(driver),
		WithDriver(driver), // Should error
	)
	if err == nil {
		t.Error("Expected error when setting driver twice")
	}
}

// TestWithTLS tests the WithTLS option
func TestWithTLS(t *testing.T) {
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	s, err := NewServer(":0",
		WithDriver(driver),
		WithTLS(tlsConfig),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.tlsConfig == nil {
		t.Error("TLS config not set")
	}
	if s.tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Error("TLS config not applied correctly")
	}
}

// TestWithLogger tests the WithLogger option
func TestWithLogger(t *testing.T) {
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)

	customLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	s, err := NewServer(":0",
		WithDriver(driver),
		WithLogger(customLogger),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.logger != customLogger {
		t.Error("Custom logger not set")
	}
}

// TestWithMaxIdleTime tests the WithMaxIdleTime option
func TestWithMaxIdleTime(t *testing.T) {
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)

	customTimeout := 10 * time.Minute

	s, err := NewServer(":0",
		WithDriver(driver),
		WithMaxIdleTime(customTimeout),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.maxIdleTime != customTimeout {
		t.Errorf("Expected timeout %v, got %v", customTimeout, s.maxIdleTime)
	}
}

// TestWithMaxConnections tests the WithMaxConnections option
func TestWithMaxConnections(t *testing.T) {
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)

	maxConns := 50
	maxPerIP := 10

	s, err := NewServer(":0",
		WithDriver(driver),
		WithMaxConnections(maxConns, maxPerIP),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.maxConnections != maxConns {
		t.Errorf("Expected max connections %d, got %d", maxConns, s.maxConnections)
	}
	if s.maxConnectionsPerIP != maxPerIP {
		t.Errorf("Expected max connections per IP %d, got %d", maxPerIP, s.maxConnectionsPerIP)
	}

	// Test with zero values (no limits)
	s2, err := NewServer(":0",
		WithDriver(driver),
		WithMaxConnections(0, 0),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	if s2.maxConnections != 0 {
		t.Errorf("Expected max connections 0, got %d", s2.maxConnections)
	}
	if s2.maxConnectionsPerIP != 0 {
		t.Errorf("Expected max connections per IP 0, got %d", s2.maxConnectionsPerIP)
	}
}

// TestWithDisableMLSD tests the WithDisableMLSD option
func TestWithDisableMLSD(t *testing.T) {
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)

	s, err := NewServer(":0",
		WithDriver(driver),
		WithDisableMLSD(true),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if !s.disableMLSD {
		t.Error("MLSD should be disabled")
	}
}

// TestNewServer_RequiresDriver tests that NewServer requires a driver
func TestNewServer_RequiresDriver(t *testing.T) {
	_, err := NewServer(":0")
	if err == nil {
		t.Error("Expected error when driver is not provided")
	}
}

// TestNewServer_Defaults tests default values
func TestNewServer_Defaults(t *testing.T) {
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)

	s, err := NewServer(":0", WithDriver(driver))
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	// Check defaults
	if s.logger == nil {
		t.Error("Default logger not set")
	}
	if s.maxIdleTime != 5*time.Minute {
		t.Errorf("Expected default idle time 5m, got %v", s.maxIdleTime)
	}
	if s.maxConnections != 0 {
		t.Errorf("Expected default max connections 0, got %d", s.maxConnections)
	}
	if s.tlsConfig != nil {
		t.Error("TLS should be disabled by default")
	}
	if s.disableMLSD {
		t.Error("MLSD should be enabled by default")
	}
	if s.welcomeMessage != "220 FTP Server Ready" {
		t.Errorf("Expected default welcome message '220 FTP Server Ready', got %q", s.welcomeMessage)
	}
	if s.serverName != "UNIX Type: L8" {
		t.Errorf("Expected default server name 'UNIX Type: L8', got %q", s.serverName)
	}
	if s.readTimeout != 0 {
		t.Errorf("Expected default read timeout 0, got %v", s.readTimeout)
	}
	if s.writeTimeout != 0 {
		t.Errorf("Expected default write timeout 0, got %v", s.writeTimeout)
	}
}

// TestWithWelcomeMessage tests the WithWelcomeMessage option
func TestWithWelcomeMessage(t *testing.T) {
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)

	customMessage := "220 Welcome to My FTP Server"

	s, err := NewServer(":0",
		WithDriver(driver),
		WithWelcomeMessage(customMessage),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.welcomeMessage != customMessage {
		t.Errorf("Expected welcome message %q, got %q", customMessage, s.welcomeMessage)
	}
}

// TestWithServerName tests the WithServerName option
func TestWithServerName(t *testing.T) {
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)

	customName := "Windows_NT"

	s, err := NewServer(":0",
		WithDriver(driver),
		WithServerName(customName),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.serverName != customName {
		t.Errorf("Expected server name %q, got %q", customName, s.serverName)
	}
}

// TestWithReadTimeout tests the WithReadTimeout option
func TestWithReadTimeout(t *testing.T) {
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)

	customTimeout := 30 * time.Second

	s, err := NewServer(":0",
		WithDriver(driver),
		WithReadTimeout(customTimeout),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.readTimeout != customTimeout {
		t.Errorf("Expected read timeout %v, got %v", customTimeout, s.readTimeout)
	}
}

// TestWithWriteTimeout tests the WithWriteTimeout option
func TestWithWriteTimeout(t *testing.T) {
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)

	customTimeout := 30 * time.Second

	s, err := NewServer(":0",
		WithDriver(driver),
		WithWriteTimeout(customTimeout),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.writeTimeout != customTimeout {
		t.Errorf("Expected write timeout %v, got %v", customTimeout, s.writeTimeout)
	}
}
