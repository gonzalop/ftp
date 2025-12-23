package ftp

import (
	"testing"
)

func TestParseListLine(t *testing.T) {
	tests := []struct {
		name           string
		line           string
		expectedName   string
		expectedType   string
		expectedSize   int64
		expectedTarget string
	}{
		// Unix-style tests
		{
			name:         "unix directory entry",
			line:         "drw-rw-rw-   1 root  root         0 Sep 24 2024 logger",
			expectedName: "logger",
			expectedType: "dir",
			expectedSize: 0,
		},
		{
			name:         "unix file with size",
			line:         "-rw-rw-rw-   1 root  root   1037794 Dec 14 12:22 large-document.pdf",
			expectedName: "large-document.pdf",
			expectedType: "file",
			expectedSize: 1037794,
		},
		{
			name:         "unix another file with size",
			line:         "-rw-rw-rw-   1 root  root    616300 Oct 25 01:18 archive-data.zip",
			expectedName: "archive-data.zip",
			expectedType: "file",
			expectedSize: 616300,
		},
		{
			name:         "unix small file",
			line:         "-rw-rw-rw-   1 root  root        16 Dec 15 04:51 verify_job",
			expectedName: "verify_job",
			expectedType: "file",
			expectedSize: 16,
		},
		{
			name:           "unix symlink",
			line:           "lrwxrwxrwx   1 root  root        11 Dec 20 10:30 link -> target.txt",
			expectedName:   "link",
			expectedType:   "link",
			expectedSize:   11,
			expectedTarget: "target.txt",
		},
		{
			name:           "unix symlink with path",
			line:           "lrwxrwxrwx   1 root  root        20 Dec 20 10:30 mylink -> /usr/bin/python3",
			expectedName:   "mylink",
			expectedType:   "link",
			expectedSize:   20,
			expectedTarget: "/usr/bin/python3",
		},
		{
			name:           "unix symlink with spaces in target",
			line:           "lrwxrwxrwx   1 root  root        25 Dec 20 10:30 docs -> /home/user/My Documents",
			expectedName:   "docs",
			expectedType:   "link",
			expectedSize:   25,
			expectedTarget: "/home/user/My Documents",
		},
		// DOS/Windows-style tests
		{
			name:         "dos directory entry",
			line:         "09-24-24  10:30AM       <DIR>          logger",
			expectedName: "logger",
			expectedType: "dir",
			expectedSize: 0,
		},
		{
			name:         "dos file with size",
			line:         "12-14-23  12:22PM           1037794 large-document.pdf",
			expectedName: "large-document.pdf",
			expectedType: "file",
			expectedSize: 1037794,
		},
		{
			name:         "dos another file",
			line:         "10-25-24  01:18AM            616300 archive-data.zip",
			expectedName: "archive-data.zip",
			expectedType: "file",
			expectedSize: 616300,
		},
		{
			name:         "dos small file",
			line:         "12-15-24  04:51AM                16 verify_job",
			expectedName: "verify_job",
			expectedType: "file",
			expectedSize: 16,
		},
		{
			name:         "dos file with spaces in name",
			line:         "12-20-24  03:30PM            123456 my document.txt",
			expectedName: "my document.txt",
			expectedType: "file",
			expectedSize: 123456,
		},
		{
			name:         "dos directory with spaces",
			line:         "11-15-24  09:00AM       <DIR>          My Folder",
			expectedName: "My Folder",
			expectedType: "dir",
			expectedSize: 0,
		},
		// DOS date format variations
		{
			name:         "dos with slash separator",
			line:         "12/14/23  12:22PM           1037794 file.txt",
			expectedName: "file.txt",
			expectedType: "file",
			expectedSize: 1037794,
		},
		{
			name:         "dos with 4-digit year",
			line:         "12-14-2023  12:22PM           1037794 file.txt",
			expectedName: "file.txt",
			expectedType: "file",
			expectedSize: 1037794,
		},
		{
			name:         "dos with slash and 4-digit year",
			line:         "12/14/2023  12:22PM           1037794 file.txt",
			expectedName: "file.txt",
			expectedType: "file",
			expectedSize: 1037794,
		},
		{
			name:         "dos directory with slash separator",
			line:         "09/24/24  10:30AM       <DIR>          logger",
			expectedName: "logger",
			expectedType: "dir",
			expectedSize: 0,
		},
		// Unix format variations
		{
			name:         "unix 8-field format (no group)",
			line:         "-rw-r--r--   1 user     4096 Dec 20 10:30 config.txt",
			expectedName: "config.txt",
			expectedType: "file",
			expectedSize: 4096,
		},
		{
			name:         "unix 8-field directory",
			line:         "drwxr-xr-x   2 user     4096 Dec 20 10:30 mydir",
			expectedName: "mydir",
			expectedType: "dir",
			expectedSize: 4096,
		},
		{
			name:         "unix numeric permissions",
			line:         "644   1 user  group     4096 Dec 20 10:30 file.txt",
			expectedName: "file.txt",
			expectedType: "file",
			expectedSize: 4096,
		},
		{
			name:         "unix with year instead of time",
			line:         "-rw-r--r--   1 user  group     4096 Dec 20  2023 oldfile.txt",
			expectedName: "oldfile.txt",
			expectedType: "file",
			expectedSize: 4096,
		},
		{
			name:         "unix file with special chars in name",
			line:         "-rw-r--r--   1 user  group     1024 Dec 20 10:30 file-with_special.chars.txt",
			expectedName: "file-with_special.chars.txt",
			expectedType: "file",
			expectedSize: 1024,
		},
		// EPLF format tests
		{
			name:         "eplf file with tab separator",
			line:         "+i8388621.48594,m825718503,r,s280,\tdjb.html",
			expectedName: "djb.html",
			expectedType: "file",
			expectedSize: 280,
		},
		{
			name:         "eplf directory",
			line:         "+i8388621.50690,m824255907,/,\tscgi",
			expectedName: "scgi",
			expectedType: "dir",
			expectedSize: 0,
		},
		{
			name:         "eplf file with space separator",
			line:         "+s1024,r readme.txt",
			expectedName: "readme.txt",
			expectedType: "file",
			expectedSize: 1024,
		},
		{
			name:         "eplf file with spaces in name",
			line:         "+s2048,r my document.txt",
			expectedName: "my document.txt",
			expectedType: "file",
			expectedSize: 2048,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := parseListLine(tt.line, nil)
			if entry == nil {
				t.Fatal("parseListLine returned nil")
			}

			if entry.Name != tt.expectedName {
				t.Errorf("Name = %q, want %q", entry.Name, tt.expectedName)
			}

			if entry.Type != tt.expectedType {
				t.Errorf("Type = %q, want %q", entry.Type, tt.expectedType)
			}

			if entry.Size != tt.expectedSize {
				t.Errorf("Size = %d, want %d", entry.Size, tt.expectedSize)
			}

			if tt.expectedTarget != "" && entry.Target != tt.expectedTarget {
				t.Errorf("Target = %q, want %q", entry.Target, tt.expectedTarget)
			}
		})
	}
}

// CustomParser for testing
type CustomParser struct{}

func (p *CustomParser) Parse(line string) (*Entry, bool) {
	if line == "custom-entry" {
		return &Entry{Name: "custom", Type: "file", Size: 999}, true
	}
	return nil, false
}

func TestCustomParser(t *testing.T) {
	custom := &CustomParser{}
	// Pass custom parser
	entry := parseListLine("custom-entry", []ListingParser{custom})
	if entry == nil {
		t.Fatal("Custom parser failed to match")
	}
	if entry.Name != "custom" {
		t.Errorf("Expected custom, got %s", entry.Name)
	}
}
