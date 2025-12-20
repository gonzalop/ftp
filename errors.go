package ftp

import "fmt"

// ProtocolError represents an FTP protocol error with full context of the
// command/response conversation. This provides detailed debugging information
// beyond simple error messages.
type ProtocolError struct {
	// Command is the FTP command that was sent (e.g., "STOR file.txt")
	Command string

	// Response is the raw response received from the server (e.g., "550 Permission denied")
	Response string

	// Code is the numeric FTP response code (e.g., 550)
	Code int
}

// Error implements the error interface.
func (e *ProtocolError) Error() string {
	return fmt.Sprintf("ftp: %s failed: %s (code %d)", e.Command, e.Response, e.Code)
}

// Is2xx returns true if the error code is in the 2xx range (success).
func (e *ProtocolError) Is2xx() bool {
	return e.Code >= 200 && e.Code < 300
}

// Is3xx returns true if the error code is in the 3xx range (intermediate).
func (e *ProtocolError) Is3xx() bool {
	return e.Code >= 300 && e.Code < 400
}

// Is4xx returns true if the error code is in the 4xx range (temporary failure).
func (e *ProtocolError) Is4xx() bool {
	return e.Code >= 400 && e.Code < 500
}

// Is5xx returns true if the error code is in the 5xx range (permanent failure).
func (e *ProtocolError) Is5xx() bool {
	return e.Code >= 500 && e.Code < 600
}

// IsTemporary returns true if the error is a temporary failure (4xx).
// This can be used to implement retry logic.
func (e *ProtocolError) IsTemporary() bool {
	return e.Is4xx()
}

// IsPermanent returns true if the error is a permanent failure (5xx).
func (e *ProtocolError) IsPermanent() bool {
	return e.Is5xx()
}
