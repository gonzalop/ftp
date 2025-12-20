package ftp

import (
	"testing"
)

func TestResolveDataAddr(t *testing.T) {
	tests := []struct {
		name        string
		pasvAddr    string
		controlHost string
		wantAddr    string
	}{
		{
			name:        "normal address",
			pasvAddr:    "192.168.1.5:12345",
			controlHost: "10.0.0.1",
			wantAddr:    "192.168.1.5:12345",
		},
		{
			name:        "zero address",
			pasvAddr:    "0.0.0.0:12345",
			controlHost: "10.0.0.1",
			wantAddr:    "10.0.0.1:12345",
		},
		{
			name:        "invalid address",
			pasvAddr:    "invalid",
			controlHost: "10.0.0.1",
			wantAddr:    "invalid", // Or handle error? The split might fail.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDataAddr(tt.pasvAddr, tt.controlHost)
			if got != tt.wantAddr {
				t.Errorf("resolveDataAddr() = %v, want %v", got, tt.wantAddr)
			}
		})
	}
}
