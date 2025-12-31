package ftp

import (
	"context"
	"net"
	"testing"
)

// mockDialer is a test implementation of the Dialer interface
type mockDialer struct {
	dialFunc func(ctx context.Context, network, address string) (net.Conn, error)
}

func (m *mockDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if m.dialFunc != nil {
		return m.dialFunc(ctx, network, address)
	}
	// Default: use standard dialer
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

// TestWithCustomDialer tests that the WithCustomDialer option is accepted
func TestWithCustomDialer(t *testing.T) {
	// Create a mock dialer
	mockD := &mockDialer{}

	// Create a client with the custom dialer option
	// We can't actually connect without a server, but we can verify the option is accepted
	c := &Client{}
	opt := WithCustomDialer(mockD)

	if err := opt(c); err != nil {
		t.Fatalf("WithCustomDialer option failed: %v", err)
	}

	if c.customDialer != mockD {
		t.Error("customDialer was not set correctly")
	}
}
