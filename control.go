package ftp

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Response represents an FTP server response.
type Response struct {
	// Code is the three-digit response code (e.g., 220, 550)
	Code int

	// Message is the human-readable message from the server
	Message string

	// Lines contains all lines of the response (for multi-line responses)
	Lines []string
}

// Is2xx returns true if the response code is in the 2xx range (success).
func (r *Response) Is2xx() bool {
	return r.Code >= 200 && r.Code < 300
}

// Is3xx returns true if the response code is in the 3xx range (intermediate).
func (r *Response) Is3xx() bool {
	return r.Code >= 300 && r.Code < 400
}

// Is4xx returns true if the response code is in the 4xx range (temporary failure).
func (r *Response) Is4xx() bool {
	return r.Code >= 400 && r.Code < 500
}

// Is5xx returns true if the response code is in the 5xx range (permanent failure).
func (r *Response) Is5xx() bool {
	return r.Code >= 500 && r.Code < 600
}

// String returns the full response as a string.
func (r *Response) String() string {
	return strings.Join(r.Lines, "\n")
}

// readResponse reads a complete FTP response from the reader.
// It handles both single-line and multi-line responses.
//
// Single-line format: "220 Welcome\r\n"
// Multi-line format:
//
//	"220-Welcome to FTP\r\n"
//	"220-This is line 2\r\n"
//	"220 Ready\r\n"
//
// The response is complete when a line starts with the code followed by a space.
func readResponse(r *bufio.Reader) (*Response, error) {
	var lines []string
	var code int
	var firstLine = true

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF && len(lines) > 0 {
				// Server closed connection after partial response
				return nil, fmt.Errorf("unexpected EOF reading response")
			}
			return nil, err
		}

		// Remove trailing \r\n
		line = strings.TrimRight(line, "\r\n")

		// Parse the code from the first line
		if firstLine {
			if len(line) < 4 {
				return nil, fmt.Errorf("invalid response line: %q", line)
			}
			parsedCode, err := strconv.Atoi(line[0:3])
			if err != nil {
				return nil, fmt.Errorf("invalid response code: %q", line[0:3])
			}
			code = parsedCode
			firstLine = false

			lines = append(lines, line)

			// Check if this is a single-line response (code followed by space)
			if len(line) >= 4 && line[3] == ' ' {
				break
			}

			// Multi-line responses have a dash after the code
			if len(line) >= 4 && line[3] != '-' {
				return nil, fmt.Errorf("invalid response format: %q", line)
			}
		} else {
			// Subsequent lines in multi-line responses can either:
			// 1. Start with the code (e.g., "211-..." or "211 ...")
			// 2. Start with a space (RFC 2389 FEAT format)
			if len(line) > 0 && line[0] == ' ' {
				// RFC 2389 format: content line starting with space
				lines = append(lines, line)
			} else if len(line) >= 4 {
				// Standard format: line starts with response code
				if line[0:3] != fmt.Sprintf("%03d", code) {
					return nil, fmt.Errorf("response code mismatch: expected %d, got %s", code, line[0:3])
				}
				lines = append(lines, line)

				// Check if this is the last line (code followed by space)
				if line[3] == ' ' {
					break
				}

				// Multi-line continuation has a dash after the code
				if line[3] != '-' {
					return nil, fmt.Errorf("invalid response format: %q", line)
				}
			} else {
				return nil, fmt.Errorf("invalid response line: %q", line)
			}
		}
	}

	// Build the message by joining all lines and removing the code prefix
	var messageLines []string
	for _, line := range lines {
		if len(line) > 4 {
			messageLines = append(messageLines, line[4:])
		}
	}
	message := strings.Join(messageLines, "\n")

	return &Response{
		Code:    code,
		Message: message,
		Lines:   lines,
	}, nil
}

// sendCommand sends an FTP command and returns the response.
func (c *Client) sendCommand(command string, args ...string) (*Response, error) {
	// Build the full command
	var cmd string
	if len(args) > 0 {
		cmd = fmt.Sprintf("%s %s", command, strings.Join(args, " "))
	} else {
		cmd = command
	}

	// Log if debug is enabled
	if c.logger != nil {
		c.logger.Debug("ftp command", "cmd", cmd)
	}

	// Set write deadline
	if c.timeout > 0 {
		if err := c.conn.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
			return nil, fmt.Errorf("failed to set write deadline: %w", err)
		}
	}

	// Send the command
	_, err := fmt.Fprintf(c.conn, "%s\r\n", cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	// Set read deadline for response
	// Note: We set it on the underlying connection, not the bufio Reader
	if c.timeout > 0 {
		if err := c.conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
			return nil, fmt.Errorf("failed to set read deadline: %w", err)
		}
	}

	// Read the response
	resp, err := readResponse(c.reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Log the response if debug is enabled
	if c.logger != nil {
		c.logger.Debug("ftp response", "code", resp.Code, "message", resp.Message)
	}

	return resp, nil
}

// expectCode sends a command and verifies the response code matches the expected code.
// Returns an error if the code doesn't match or if the command fails.
func (c *Client) expectCode(expectedCode int, command string, args ...string) (*Response, error) {
	resp, err := c.sendCommand(command, args...)
	if err != nil {
		return nil, err
	}

	if resp.Code != expectedCode {
		return resp, &ProtocolError{
			Command:  command,
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	return resp, nil
}

// expect2xx sends a command and verifies the response is in the 2xx range (success).
func (c *Client) expect2xx(command string, args ...string) (*Response, error) {
	resp, err := c.sendCommand(command, args...)
	if err != nil {
		return nil, err
	}

	if !resp.Is2xx() {
		return resp, &ProtocolError{
			Command:  command,
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	return resp, nil
}
