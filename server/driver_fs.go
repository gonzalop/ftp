package server

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FSDriver implements Driver using the local filesystem.
//
// Security Model:
//   - All file operations are confined to the root path using os.Root
//   - Path traversal attacks (../) are prevented by path validation
//   - Read-only mode is enforced at the operation level
//   - Each user session gets an isolated ClientContext
//
// The driver uses os.Root (Go 1.24+) to safely jail file operations within
// the root directory. This provides kernel-level protection against directory
// traversal attacks.
//
// Default behavior (no options):
//   - Allows anonymous login ("ftp" or "anonymous" users only)
//   - Anonymous users have read-only access
//   - All operations are confined to the root path
type FSDriver struct {
	rootPath string // Root directory for the server

	// authenticator is an optional hook to validate credentials and return the
	// root path for the user. If nil, defaults to strict anonymous-only, read-only access,
	// unless disableAnonymous is true.
	// Arguments: user, pass, host
	// Returns: rootPath, readOnly, error
	authenticator func(user, pass, host string) (string, bool, error)

	// disableAnonymous, if true, prevents the default behavior of allowing anonymous
	// logins when no authenticator is provided.
	//
	// Note that this setting is only effective when authenticator is nil. If a custom
	// authenticator is defined, it assumes full control over the authentication
	// process, including anonymous access.
	disableAnonymous bool

	// enableAnonWrite, if true, allows anonymous users to perform write operations
	// (upload, mkdir, delete, etc.).
	// Default is false (read-only).
	enableAnonWrite bool

	settings *Settings // Optional server settings
}

// FSDriverOption is a functional option for configuring an FSDriver.
type FSDriverOption func(*FSDriver)

// NewFSDriver creates a new filesystem driver with the given root path and options.
// The root path is the directory that will serve as the root for all FTP operations.
// Returns an error if the root path does not exist or is not a directory.
//
// Default behavior:
//   - Allows anonymous login ("ftp" or "anonymous" users)
//   - Anonymous users have read-only access (unless WithAnonWrite(true) is used)
//   - All operations are confined to the root path
//
// Basic usage:
//
//	driver, err := server.NewFSDriver("/tmp/ftp")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// With custom authentication:
//
//	driver, err := server.NewFSDriver("/tmp/ftp",
//	    server.WithAuthenticator(func(user, pass, host string) (string, bool, error) {
//	        if user == "admin" && pass == "secret" {
//	            return "/tmp/ftp", false, nil // read-write access
//	        }
//	        if user == "guest" && pass == "guest" {
//	            return "/tmp/ftp/public", true, nil // read-only, restricted dir
//	        }
//	        return "", false, os.ErrPermission
//	    }))
//
// With per-user directories:
//
//	driver, err := server.NewFSDriver("/home",
//	    server.WithAuthenticator(func(user, pass, host string) (string, bool, error) {
//	        if validateUser(user, pass) {
//	            userDir := filepath.Join("/home", user)
//	            return userDir, false, nil
//	        }
//	        return "", false, os.ErrPermission
//	    }))
func NewFSDriver(rootPath string, options ...FSDriverOption) (*FSDriver, error) {
	// Validate that rootPath exists and is a directory
	info, err := os.Stat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("root path validation failed: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root path is not a directory: %s", rootPath)
	}

	// Canonicalize the root path to ensure we can safely compare it later
	rootPath, err = filepath.EvalSymlinks(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve root path: %w", err)
	}

	d := &FSDriver{
		rootPath: rootPath,
	}

	// Apply options
	for _, opt := range options {
		opt(d)
	}

	return d, nil
}

// WithAuthenticator sets a custom authentication function.
//
// The function receives:
//   - user: username from USER command
//   - pass: password from PASS command
//   - host: hostname from HOST command (may be empty)
//
// The function should return:
//   - rootPath: the root directory for this user (must exist)
//   - readOnly: true to restrict user to read-only operations
//   - error: authentication error (use os.ErrPermission for invalid credentials)
//
// Example with database lookup:
//
//	server.WithAuthenticator(func(user, pass, host string) (string, bool, error) {
//	    dbUser, err := db.ValidateUser(user, pass)
//	    if err != nil {
//	        return "", false, os.ErrPermission
//	    }
//	    return dbUser.HomeDir, dbUser.ReadOnly, nil
//	})
func WithAuthenticator(fn func(user, pass, host string) (string, bool, error)) FSDriverOption {
	return func(d *FSDriver) {
		d.authenticator = fn
	}
}

// WithDisableAnonymous disables anonymous login.
// When enabled, only users authenticated via a custom Authenticator are allowed.
//
// This option only affects the default authentication behavior. If you provide
// a custom Authenticator, it has full control over authentication.
//
// Example:
//
//	driver, _ := server.NewFSDriver("/tmp/ftp",
//	    server.WithDisableAnonymous(true),
//	    server.WithAuthenticator(validateUser),
//	)
func WithDisableAnonymous(disable bool) FSDriverOption {
	return func(d *FSDriver) {
		d.disableAnonymous = disable
	}
}

// WithAnonWrite enables write access for anonymous users.
// Default is false (read-only).
// Use this with caution.
func WithAnonWrite(enable bool) FSDriverOption {
	return func(d *FSDriver) {
		d.enableAnonWrite = enable
	}
}

// WithSettings sets server-specific settings for the driver.
// These settings configure passive mode behavior and other server features.
//
// Example:
//
//	settings := &server.Settings{
//	    PublicHost:  "ftp.example.com",
//	    PasvMinPort: 30000,
//	    PasvMaxPort: 30100,
//	}
//	driver, _ := server.NewFSDriver("/tmp/ftp",
//	    server.WithSettings(settings),
//	)
func WithSettings(settings *Settings) FSDriverOption {
	return func(d *FSDriver) {
		d.settings = settings
	}
}

// Authenticate returns a new FSContext for the user.
// It uses the authenticator hook if provided. Otherwise, it enforces strict
// anonymous-only, read-only access rooted at the root path.
func (d *FSDriver) Authenticate(user, pass, host string) (ClientContext, error) {
	rootPath := d.rootPath
	readOnly := false

	if d.authenticator != nil {
		var err error
		rootPath, readOnly, err = d.authenticator(user, pass, host)
		if err != nil {
			return nil, err
		}
	} else {
		if d.disableAnonymous {
			return nil, errors.New("anonymous login disabled")
		}
		// Default behavior: Strict Anonymous
		if user != "ftp" && user != "anonymous" {
			return nil, errors.New("only anonymous login allowed")
		}
		// Anonymous access: read-only unless explicitly enabled
		readOnly = !d.enableAnonWrite
	}

	// Open the root directory safely
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}

	return &fsContext{
		rootHandle: root,
		rootPath:   rootPath,
		cwd:        "/",
		readOnly:   readOnly,
		settings:   d.settings,
	}, nil
}

// fsContext implements ClientContext for the local filesystem.
// It tracks the current working directory and ensures all operations
// are jailed within the root handle.
type fsContext struct {
	rootHandle *os.Root
	rootPath   string
	cwd        string
	readOnly   bool
	settings   *Settings
}

// Close closes the underlying root directory handle.
// This is essential to release file descriptors.
func (c *fsContext) Close() error {
	return c.rootHandle.Close()
}

// resolve returns the path relative to the root handle.
// It ensures the path does not escape the root.
func (c *fsContext) resolve(path string) (string, error) {
	// 1. Handle absolute paths (virtual root /)
	if strings.HasPrefix(path, "/") {
		// path is absolute in virtual fs
	} else {
		// path is relative to cwd
		path = filepath.Join(c.cwd, path)
	}

	// 2. Clean the path
	path = filepath.Clean(path)

	// 3. Ensure it starts with / and strip it for relative usage
	if !strings.HasPrefix(path, "/") {
		return "", errors.New("invalid path")
	}

	// 4. Strip leading slash to get path relative to root handle
	// e.g. "/foo/bar" -> "foo/bar"
	// "/" -> "."
	rel := strings.TrimPrefix(path, "/")
	if rel == "" {
		rel = "."
	}

	return rel, nil
}

// ChangeDir changes the current working directory.
// It verifies the destination exists and is a directory.
func (c *fsContext) ChangeDir(path string) error {
	rel, err := c.resolve(path)
	if err != nil {
		return err
	}

	// Validate it exists and is a directory
	info, err := c.rootHandle.Stat(rel)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("not a directory")
	}

	// Update cwd (virtual path)
	if !strings.HasPrefix(path, "/") {
		path = filepath.Join(c.cwd, path)
	}
	c.cwd = filepath.Clean(path)
	// Ensure it starts with /
	if !strings.HasPrefix(c.cwd, "/") {
		c.cwd = "/" + c.cwd
	}

	return nil
}

// GetWd returns the current working directory.
func (c *fsContext) GetWd() (string, error) {
	return c.cwd, nil
}

// MakeDir creates a new directory with 0755 permissions.
func (c *fsContext) MakeDir(path string) error {
	if c.readOnly {
		return os.ErrPermission
	}
	rel, err := c.resolve(path)
	if err != nil {
		return err
	}
	return c.rootHandle.Mkdir(rel, 0755)
}

// RemoveDir removes a directory and its contents.
func (c *fsContext) RemoveDir(path string) error {
	if c.readOnly {
		return os.ErrPermission
	}
	rel, err := c.resolve(path)
	if err != nil {
		return err
	}
	return c.rootHandle.Remove(rel)
}

// DeleteFile removes a file.
func (c *fsContext) DeleteFile(path string) error {
	if c.readOnly {
		return os.ErrPermission
	}
	rel, err := c.resolve(path)
	if err != nil {
		return err
	}
	return c.rootHandle.Remove(rel)
}

// Rename moves or renames a file or directory.
func (c *fsContext) Rename(fromPath, toPath string) error {
	if c.readOnly {
		return os.ErrPermission
	}
	srcRel, err := c.resolve(fromPath)
	if err != nil {
		return err
	}
	dstRel, err := c.resolve(toPath)
	if err != nil {
		return err
	}

	srcFull := filepath.Join(c.rootPath, srcRel)
	dstFull := filepath.Join(c.rootPath, dstRel)

	// Security check: ensure both source and destination are within the root
	// We use EvalSymlinks to resolve all path components.
	// Note: EvalSymlinks fails if the path doesn't exist, which is fine for src.
	// For dst, we check its parent directory if it doesn't exist yet.
	realSrc, err := filepath.EvalSymlinks(srcFull)
	if err != nil {
		if os.IsNotExist(err) {
			return os.ErrNotExist
		}
		if os.IsPermission(err) {
			return os.ErrPermission
		}
		// Return a generic error to avoid leaking the absolute path
		return errors.New("failed to resolve source path")
	}
	if !strings.HasPrefix(realSrc, c.rootPath) {
		return os.ErrPermission
	}

	// For destination, we check the parent directory
	dstParent := filepath.Dir(dstFull)
	realDstParent, err := filepath.EvalSymlinks(dstParent)
	if err == nil {
		if !strings.HasPrefix(realDstParent, c.rootPath) {
			return os.ErrPermission
		}
	} else if !os.IsNotExist(err) {
		if os.IsPermission(err) {
			return os.ErrPermission
		}
		return errors.New("failed to resolve destination path")
	}

	// Safety check: ensure we can resolve root path
	// os.Root does not support Rename for now.
	if err := os.Rename(srcFull, dstFull); err != nil {
		if os.IsNotExist(err) {
			return os.ErrNotExist
		}
		if os.IsPermission(err) {
			return os.ErrPermission
		}
		// Return a generic error to avoid leaking the absolute path
		return errors.New("rename failed")
	}
	return nil
}

// ListDir returns a list of files in the specified directory.
func (c *fsContext) ListDir(path string) ([]os.FileInfo, error) {
	rel, err := c.resolve(path)
	if err != nil {
		return nil, err
	}

	f, err := c.rootHandle.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}

	infos := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err == nil {
			infos = append(infos, info)
		}
	}
	return infos, nil
}

// OpenFile opens a file for transfer (reading or writing).
func (c *fsContext) OpenFile(path string, flag int) (io.ReadWriteCloser, error) {
	if c.readOnly {
		// Check if any write flags are set
		if flag&os.O_WRONLY != 0 || flag&os.O_RDWR != 0 || flag&os.O_CREATE != 0 || flag&os.O_TRUNC != 0 || flag&os.O_APPEND != 0 {
			return nil, os.ErrPermission
		}
	}
	rel, err := c.resolve(path)
	if err != nil {
		return nil, err
	}

	// os.Root.OpenFile(name, flag, perm)
	return c.rootHandle.OpenFile(rel, flag, 0644)
}

// GetFileInfo returns status information for a file or directory.
func (c *fsContext) GetFileInfo(path string) (os.FileInfo, error) {
	rel, err := c.resolve(path)
	if err != nil {
		return nil, err
	}
	return c.rootHandle.Stat(rel)
}

// GetHash calculates the hash of the file using the specified algorithm.
// Supported algorithms: SHA-256, SHA-512, SHA-1, MD5, CRC32
func (c *fsContext) GetHash(path string, algo string) (string, error) {
	rel, err := c.resolve(path)
	if err != nil {
		return "", err
	}

	f, err := c.rootHandle.Open(rel)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var h interface {
		io.Writer
		Sum(b []byte) []byte
	}

	switch strings.ToUpper(algo) {
	case "SHA-256", "SHA256":
		h = sha256.New()
	case "SHA-512", "SHA512":
		h = sha512.New()
	case "SHA-1", "SHA1":
		h = sha1.New()
	case "MD5":
		h = md5.New()
	case "CRC32":
		h = crc32.NewIEEE()
	default:
		return "", errors.New("unsupported algorithm")
	}

	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// SetTime sets the modification time of a file.
// Used by the MFMT command.
func (c *fsContext) SetTime(path string, t time.Time) error {
	if c.readOnly {
		return os.ErrPermission
	}
	rel, err := c.resolve(path)
	if err != nil {
		return err
	}

	fullPath := filepath.Join(c.rootPath, rel)

	// Security check: ensure the target is within the root
	// This prevents symlink traversal attacks (e.g. symlink in root -> /etc/passwd)
	realPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return os.ErrNotExist
		}
		if os.IsPermission(err) {
			return os.ErrPermission
		}
		return errors.New("failed to resolve path")
	}
	if !strings.HasPrefix(realPath, c.rootPath) {
		return os.ErrPermission
	}

	if err := os.Chtimes(fullPath, t, t); err != nil {
		if os.IsNotExist(err) {
			return os.ErrNotExist
		}
		if os.IsPermission(err) {
			return os.ErrPermission
		}
		return errors.New("failed to set time")
	}
	return nil
}

// Chmod changes the mode of the file.
// Used by the SITE CHMOD command.
func (c *fsContext) Chmod(path string, mode os.FileMode) error {
	if c.readOnly {
		return os.ErrPermission
	}

	// Validate mode: only allow standard permission bits (0-777)
	if mode > 0777 {
		return os.ErrInvalid
	}

	rel, err := c.resolve(path)
	if err != nil {
		return err
	}

	// Open the file through the root handle to enforce the jail
	// We use O_RDONLY because we just need a handle to call Chmod
	f, err := c.rootHandle.OpenFile(rel, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	return f.Chmod(mode)
}

func (c *fsContext) GetSettings() *Settings {
	if c.settings == nil {
		return &Settings{}
	}
	return c.settings
}
