package ftp

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadResponse_SingleLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantCode int
		wantMsg  string
		wantErr  bool
	}{
		{
			name:     "simple success",
			input:    "220 Welcome\r\n",
			wantCode: 220,
			wantMsg:  "Welcome",
			wantErr:  false,
		},
		{
			name:     "error response",
			input:    "550 File not found\r\n",
			wantCode: 550,
			wantMsg:  "File not found",
			wantErr:  false,
		},
		{
			name:     "code with no message",
			input:    "200 \r\n",
			wantCode: 200,
			wantMsg:  "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			resp, err := readResponse(reader)

			if (err != nil) != tt.wantErr {
				t.Errorf("readResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if resp.Code != tt.wantCode {
					t.Errorf("readResponse() code = %v, want %v", resp.Code, tt.wantCode)
				}
				if resp.Message != tt.wantMsg {
					t.Errorf("readResponse() message = %v, want %v", resp.Message, tt.wantMsg)
				}
			}
		})
	}
}

func TestReadResponse_MultiLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantCode int
		wantMsg  string
		wantErr  bool
	}{
		{
			name: "multi-line response",
			input: "220-Welcome to FTP\r\n" +
				"220-This is line 2\r\n" +
				"220 Ready\r\n",
			wantCode: 220,
			wantMsg:  "Welcome to FTP\nThis is line 2\nReady",
			wantErr:  false,
		},
		{
			name: "transfer complete",
			input: "226-Transfer complete\r\n" +
				"226 Closing data connection\r\n",
			wantCode: 226,
			wantMsg:  "Transfer complete\nClosing data connection",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			resp, err := readResponse(reader)

			if (err != nil) != tt.wantErr {
				t.Errorf("readResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if resp.Code != tt.wantCode {
					t.Errorf("readResponse() code = %v, want %v", resp.Code, tt.wantCode)
				}
				if resp.Message != tt.wantMsg {
					t.Errorf("readResponse() message = %q, want %q", resp.Message, tt.wantMsg)
				}
			}
		})
	}
}

func TestParsePASV(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantAddr string
		wantErr  bool
	}{
		{
			name:     "standard PASV response",
			input:    "227 Entering Passive Mode (192,168,1,1,195,149)",
			wantAddr: "192.168.1.1:50069",
			wantErr:  false,
		},
		{
			name:     "PASV with text before",
			input:    "227 Entering Passive Mode (10,0,0,5,78,52)",
			wantAddr: "10.0.0.5:20020",
			wantErr:  false,
		},
		{
			name:     "invalid PASV response",
			input:    "227 Invalid response",
			wantAddr: "",
			wantErr:  true,
		},
		{
			name:     "PASV with invalid IP parts",
			input:    "227 Entering Passive Mode (300,168,1,1,195,149)",
			wantAddr: "",
			wantErr:  true,
		},
		{
			name:     "PASV with 0.0.0.0 IP",
			input:    "227 Entering Passive Mode (0,0,0,0,195,149)",
			wantAddr: "0.0.0.0:50069",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := parsePASV(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("parsePASV() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if addr != tt.wantAddr {
				t.Errorf("parsePASV() = %v, want %v", addr, tt.wantAddr)
			}
		})
	}
}

func TestParseEPSV(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantPort string
		wantErr  bool
	}{
		{
			name:     "standard EPSV response",
			input:    "229 Entering Extended Passive Mode (|||6446|)",
			wantPort: "6446",
			wantErr:  false,
		},
		{
			name:     "EPSV with text",
			input:    "229 Extended Passive Mode OK (|||12345|)",
			wantPort: "12345",
			wantErr:  false,
		},
		{
			name:     "invalid EPSV response",
			input:    "229 Invalid response",
			wantPort: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, err := parseEPSV(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseEPSV() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if port != tt.wantPort {
				t.Errorf("parseEPSV() = %v, want %v", port, tt.wantPort)
			}
		})
	}
}

func TestResponse_CodeChecks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code  int
		is2xx bool
		is3xx bool
		is4xx bool
		is5xx bool
	}{
		{200, true, false, false, false},
		{220, true, false, false, false},
		{331, false, true, false, false},
		{421, false, false, true, false},
		{550, false, false, false, true},
	}

	for _, tt := range tests {
		resp := &Response{Code: tt.code}

		if resp.Is2xx() != tt.is2xx {
			t.Errorf("Response{%d}.Is2xx() = %v, want %v", tt.code, resp.Is2xx(), tt.is2xx)
		}
		if resp.Is3xx() != tt.is3xx {
			t.Errorf("Response{%d}.Is3xx() = %v, want %v", tt.code, resp.Is3xx(), tt.is3xx)
		}
		if resp.Is4xx() != tt.is4xx {
			t.Errorf("Response{%d}.Is4xx() = %v, want %v", tt.code, resp.Is4xx(), tt.is4xx)
		}
		if resp.Is5xx() != tt.is5xx {
			t.Errorf("Response{%d}.Is5xx() = %v, want %v", tt.code, resp.Is5xx(), tt.is5xx)
		}
	}
}

func TestProtocolError(t *testing.T) {
	t.Parallel()
	err := &ProtocolError{
		Command:  "STOR file.txt",
		Response: "Permission denied",
		Code:     550,
	}

	if !err.Is5xx() {
		t.Error("ProtocolError with code 550 should be Is5xx()")
	}

	if !err.IsPermanent() {
		t.Error("ProtocolError with code 550 should be IsPermanent()")
	}

	if err.IsTemporary() {
		t.Error("ProtocolError with code 550 should not be IsTemporary()")
	}

	expectedMsg := "ftp: STOR file.txt failed: Permission denied (code 550)"
	if err.Error() != expectedMsg {
		t.Errorf("ProtocolError.Error() = %q, want %q", err.Error(), expectedMsg)
	}
}

func TestReadResponse_RFC2389(t *testing.T) {
	t.Parallel()
	// Example from RFC 2389 - feature lines start with space
	response := "211-Extensions supported:\r\n" +
		" MLST size*;create;modify*;perm;media-type\r\n" +
		" SIZE\r\n" +
		" COMPRESSION\r\n" +
		" MDTM\r\n" +
		"211 END\r\n"

	reader := bufio.NewReader(strings.NewReader(response))
	resp, err := readResponse(reader)
	if err != nil {
		t.Fatalf("readResponse failed on RFC 2389 payload: %v", err)
	}

	if resp.Code != 211 {
		t.Errorf("expected code 211, got %d", resp.Code)
	}
	if len(resp.Lines) != 6 {
		t.Errorf("expected 6 lines, got %d", len(resp.Lines))
	}
}
