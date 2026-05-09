package ftp

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadResponse_Limits(t *testing.T) {
	t.Parallel()
	t.Run("line too long", func(t *testing.T) {
		// Create a line longer than 4096 bytes
		longLine := "220 " + strings.Repeat("a", 4100) + "\r\n"
		reader := bufio.NewReader(strings.NewReader(longLine))
		_, err := readResponse(reader)
		if err == nil {
			t.Error("expected error for line too long, got nil")
		} else if !strings.Contains(err.Error(), "line too long") {
			t.Errorf("expected 'line too long' error, got: %v", err)
		}
	})

	t.Run("too many lines", func(t *testing.T) {
		// Create a multi-line response with > 1000 lines
		var builder strings.Builder
		builder.WriteString("220-Start\r\n")
		for i := 0; i < 1001; i++ {
			builder.WriteString("220-Line\r\n")
		}
		builder.WriteString("220 End\r\n")
		reader := bufio.NewReader(strings.NewReader(builder.String()))
		_, err := readResponse(reader)
		if err == nil {
			t.Error("expected error for too many lines, got nil")
		} else if !strings.Contains(err.Error(), "too many response lines") {
			t.Errorf("expected 'too many response lines' error, got: %v", err)
		}
	})
}
