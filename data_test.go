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

func TestFormatEPRT(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		want    string
		wantErr bool
	}{
		{
			name: "IPv4",
			addr: "127.0.0.1:12345",
			want: "|1|127.0.0.1|12345|",
		},
		{
			name: "IPv6",
			addr: "[::1]:12345",
			want: "|2|::1|12345|",
		},
		{
			name:    "Invalid",
			addr:    "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatEPRT(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("formatEPRT() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("formatEPRT() = %v, want %v", got, tt.want)
			}
		})
	}
}
