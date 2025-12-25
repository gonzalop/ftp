package server

import (
	"io"
	"os"
	"time"
)

// Driver is the interface that must be implemented by an FTP driver.
// It is responsible for authenticating users and providing a session-specific
// ClientContext for file operations.
//
// Implementations should:
//   - Validate user credentials (user, pass)
//   - Use the host parameter for virtual hosting (optional)
//   - Return a ClientContext that isolates the user's file operations
//   - Return os.ErrPermission or similar errors for authentication failures
//
// Example implementation:
//
//	type MyDriver struct{}
//
//	func (d *MyDriver) Authenticate(user, pass, host string) (ClientContext, error) {
//	    if !validateCredentials(user, pass) {
//	        return nil, os.ErrPermission
//	    }
//	    return newMyContext(user), nil
//	}
//
// To implement a custom backend (e.g., S3, Database, Memory), implement this interface.
type Driver interface {
	// Authenticate validates the user and password.
	// The host parameter contains the value from the HOST command (RFC 7151),
	// which can be used for virtual hosting. It may be empty if not provided.
	//
	// Returns:
	//   - ClientContext: A session-specific context for file operations
	//   - error: Authentication error (use os.ErrPermission for invalid credentials)
	Authenticate(user, pass, host string) (ClientContext, error)
}

// ClientContext is the interface that must be implemented by a driver to handle
// file system operations for a specific client session.
//
// It isolates the operations to the user's view of the filesystem (e.g., handling chroots).
// All paths are relative to the user's root directory and use forward slashes.
//
// Error handling:
//   - Return os.ErrNotExist when files/directories don't exist
//   - Return os.ErrPermission for permission denied errors
//   - Return os.ErrExist when files/directories already exist
//   - The server will translate these to appropriate FTP response codes
//
// Implementations must be safe for concurrent use by a single session.
type ClientContext interface {
	// ChangeDir changes the current working directory.
	// Returns os.ErrNotExist if the directory doesn't exist.
	ChangeDir(path string) error

	// GetWd returns the current working directory.
	GetWd() (string, error)

	// MakeDir creates a new directory.
	// Returns os.ErrExist if the directory already exists.
	MakeDir(path string) error

	// RemoveDir removes a directory and its contents.
	// Returns os.ErrNotExist if the directory doesn't exist.
	RemoveDir(path string) error

	// DeleteFile removes a file.
	// Returns os.ErrNotExist if the file doesn't exist.
	DeleteFile(path string) error

	// Rename moves or renames a file or directory.
	// Returns os.ErrNotExist if the source doesn't exist.
	Rename(fromPath, toPath string) error

	// ListDir returns a list of files in the specified directory.
	// Returns os.ErrNotExist if the directory doesn't exist.
	ListDir(path string) ([]os.FileInfo, error)

	// OpenFile opens a file for reading or writing.
	// The flag parameter uses os.O_* constants (os.O_RDONLY, os.O_WRONLY|os.O_CREATE, etc.).
	// Returns os.ErrNotExist if the file doesn't exist (for reading).
	OpenFile(path string, flag int) (io.ReadWriteCloser, error)

	// GetFileInfo returns file or directory metadata.
	// Returns os.ErrNotExist if the path doesn't exist.
	GetFileInfo(path string) (os.FileInfo, error)

	// GetHash calculates the hash of a file using the specified algorithm.
	// Supported algorithms: "SHA-256", "SHA-512", "SHA-1", "MD5", "CRC32".
	// Returns an error if the algorithm is unsupported or the file doesn't exist.
	GetHash(path string, algo string) (string, error)

	// SetTime sets the modification time of a file.
	// Used by the MFMT command.
	// Returns os.ErrNotExist if the file doesn't exist.
	SetTime(path string, t time.Time) error

	// Chmod changes the mode of the file.
	// Used by the SITE CHMOD command.
	// Returns os.ErrNotExist if the file doesn't exist.
	Chmod(path string, mode os.FileMode) error

	// Close releases any resources associated with this context.
	// Called when the client disconnects.
	Close() error

	// GetSettings returns the session settings for passive mode configuration.
	// May return nil if no special settings are needed.
	GetSettings() *Settings
}

// Settings defines server configuration for passive mode and other features.
//
// These settings are typically configured once and shared across all sessions,
// but can be customized per-user if needed.
type Settings struct {
	// PublicHost is the hostname or IP address advertised in PASV responses.
	// If set to a hostname, the server will resolve it once and use the first
	// IPv4 address found.
	// If empty, the server uses the control connection's local address.
	// Required when behind NAT or in containerized environments.
	PublicHost string

	// PasvMinPort is the minimum port number for passive data connections.
	// If 0, the OS assigns a random port.
	PasvMinPort int

	// PasvMaxPort is the maximum port number for passive data connections.
	// If 0, the OS assigns a random port.
	// Must be >= PasvMinPort if both are set.
	PasvMaxPort int
}
