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

	s, err := NewServer(":0",
		WithDriver(driver),
		WithMaxConnections(maxConns),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.maxConnections != maxConns {
		t.Errorf("Expected max connections %d, got %d", maxConns, s.maxConnections)
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
}
