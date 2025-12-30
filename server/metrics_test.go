package server

import (
	"testing"
	"time"
)

// mockMetricsCollector is a simple mock for testing
type mockMetricsCollector struct {
	commands        int
	transfers       int
	connections     int
	authentications int
}

func (m *mockMetricsCollector) RecordCommand(cmd string, success bool, duration time.Duration) {
	m.commands++
}

func (m *mockMetricsCollector) RecordTransfer(operation string, bytes int64, duration time.Duration) {
	m.transfers++
}

func (m *mockMetricsCollector) RecordConnection(accepted bool, reason string) {
	m.connections++
}

func (m *mockMetricsCollector) RecordAuthentication(success bool, user string) {
	m.authentications++
}

func TestWithMetricsCollector(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)
	mock := &mockMetricsCollector{}

	s, err := NewServer(":0",
		WithDriver(driver),
		WithMetricsCollector(mock),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.metricsCollector == nil {
		t.Error("Expected metricsCollector to be set")
	}
}

func TestMetricsCollectorNilSafe(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	driver, _ := NewFSDriver(tempDir)

	// Server without metrics collector should not panic
	s, err := NewServer(":0",
		WithDriver(driver),
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if s.metricsCollector != nil {
		t.Error("Expected metricsCollector to be nil")
	}

	// This should not panic even though collector is nil
	if s.metricsCollector != nil {
		s.metricsCollector.RecordConnection(true, "accepted")
	}
}
