package ftp

import (
	"strings"
	"testing"
	"time"
)

func TestParseMLEntry(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantName  string
		wantType  string
		wantSize  int64
		wantError bool
	}{
		{
			name:     "file entry",
			input:    "type=file;size=1234;modify=20231220143000; example.txt",
			wantName: "example.txt",
			wantType: "file",
			wantSize: 1234,
		},
		{
			name:     "directory entry",
			input:    "type=dir;modify=20231220143000;perm=flcdmpe; mydir",
			wantName: "mydir",
			wantType: "dir",
			wantSize: 0,
		},
		{
			name:     "file with spaces",
			input:    "type=file;size=5678; my file.txt",
			wantName: "my file.txt",
			wantType: "file",
			wantSize: 5678,
		},
		{
			name:      "invalid format",
			input:     "no-space-separator",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := parseMLEntry(tt.input)

			if (err != nil) != tt.wantError {
				t.Errorf("parseMLEntry() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if err == nil {
				if entry.Name != tt.wantName {
					t.Errorf("parseMLEntry() name = %v, want %v", entry.Name, tt.wantName)
				}
				if entry.Type != tt.wantType {
					t.Errorf("parseMLEntry() type = %v, want %v", entry.Type, tt.wantType)
				}
				if entry.Size != tt.wantSize {
					t.Errorf("parseMLEntry() size = %v, want %v", entry.Size, tt.wantSize)
				}
			}
		})
	}
}

func TestParseMLEntry_ModTime(t *testing.T) {
	input := "type=file;modify=20231220143000; test.txt"
	entry, err := parseMLEntry(input)
	if err != nil {
		t.Fatalf("parseMLEntry() error = %v", err)
	}

	expectedTime := time.Date(2023, 12, 20, 14, 30, 0, 0, time.UTC)
	if !entry.ModTime.Equal(expectedTime) {
		t.Errorf("parseMLEntry() modTime = %v, want %v", entry.ModTime, expectedTime)
	}
}

func TestParseMLEntry_Facts(t *testing.T) {
	input := "type=file;size=1234;perm=r;unix.mode=0644; test.txt"
	entry, err := parseMLEntry(input)
	if err != nil {
		t.Fatalf("parseMLEntry() error = %v", err)
	}

	if entry.Perm != "r" {
		t.Errorf("parseMLEntry() perm = %v, want r", entry.Perm)
	}

	if entry.UnixMode != "0644" {
		t.Errorf("parseMLEntry() unixMode = %v, want 0644", entry.UnixMode)
	}

	if len(entry.Facts) == 0 {
		t.Error("parseMLEntry() facts map is empty")
	}
}

func TestParseFEATResponse(t *testing.T) {
	// Simulate FEAT response parsing
	response := `211-Features:
 MDTM
 REST STREAM
 SIZE
 MLST type*;size*;modify*;
 UTF8
211 End`

	lines := strings.Split(response, "\n")
	features := make(map[string]string)

	for _, line := range lines {
		// Skip the first and last lines (211-... and 211 ...)
		if len(line) < 4 {
			continue
		}
		if len(line) >= 4 && (line[3] == '-' || line[3] == ' ') {
			// This is the status line, skip it
			continue
		}

		// Feature lines start with a space
		featureLine := strings.TrimSpace(line)
		if featureLine == "" {
			continue
		}

		// Split feature name and parameters
		parts := strings.SplitN(featureLine, " ", 2)
		featName := strings.ToUpper(parts[0])
		featParams := ""
		if len(parts) > 1 {
			featParams = parts[1]
		}

		features[featName] = featParams
	}

	// Verify parsed features
	if _, ok := features["MDTM"]; !ok {
		t.Error("MDTM feature not found")
	}

	if _, ok := features["SIZE"]; !ok {
		t.Error("SIZE feature not found")
	}

	if params, ok := features["REST"]; !ok || params != "STREAM" {
		t.Errorf("REST feature = %v, want STREAM", params)
	}

	if params, ok := features["MLST"]; !ok || !strings.Contains(params, "type*") {
		t.Errorf("MLST feature = %v, want to contain type*", params)
	}
}
