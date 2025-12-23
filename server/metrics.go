package server

import "time"

// PathRedactor is a function type for custom path redaction in logs.
// It takes a file path and returns a redacted version for privacy.
//
// Example implementations:
//
//	// Redact middle components
//	func(path string) string {
//	    parts := strings.Split(path, "/")
//	    if len(parts) > 3 {
//	        for i := 2; i < len(parts)-1; i++ {
//	            parts[i] = "*"
//	        }
//	    }
//	    return strings.Join(parts, "/")
//	}
//
//	// Redact specific patterns
//	func(path string) string {
//	    return regexp.MustCompile(`/users/[^/]+/`).ReplaceAllString(path, "/users/*/")
//	}
type PathRedactor func(path string) string

// MetricsCollector is an optional interface for collecting server metrics.
// Implementations can send metrics to monitoring systems like Prometheus,
// StatsD, DataDog, etc.
//
// All methods are called from various points in the server lifecycle and
// should be non-blocking. If a method takes significant time, it should
// dispatch the work asynchronously.
//
// The server will check if the collector is nil before calling methods,
// so implementations don't need to handle nil receivers.
type MetricsCollector interface {
	// RecordCommand records metrics for an FTP command execution.
	// cmd is the command name (e.g., "RETR", "STOR", "LIST").
	// success indicates whether the command completed successfully.
	// duration is how long the command took to execute.
	RecordCommand(cmd string, success bool, duration time.Duration)

	// RecordTransfer records metrics for a file transfer operation.
	// operation is either "RETR" (download) or "STOR" (upload).
	// bytes is the number of bytes transferred.
	// duration is how long the transfer took.
	RecordTransfer(operation string, bytes int64, duration time.Duration)

	// RecordConnection records metrics for connection attempts.
	// accepted indicates whether the connection was accepted.
	// reason provides context (e.g., "global_limit_reached", "per_ip_limit_reached", "accepted").
	RecordConnection(accepted bool, reason string)

	// RecordAuthentication records metrics for authentication attempts.
	// success indicates whether authentication succeeded.
	// user is the username that attempted to authenticate.
	RecordAuthentication(success bool, user string)
}
