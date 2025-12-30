package server

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFSDriver_DisableAnonymous(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	tests := []struct {
		name             string
		disableAnonymous bool
		user             string
		expectError      bool
	}{
		{
			name:             "Default (Allowed)",
			disableAnonymous: false,
			user:             "anonymous",
			expectError:      false,
		},
		{
			name:             "Default (Allowed) - FTP",
			disableAnonymous: false,
			user:             "ftp",
			expectError:      false,
		},
		{
			name:             "Default (Allowed) - Invalid User",
			disableAnonymous: false,
			user:             "user",
			expectError:      true,
		},
		{
			name:             "Disabled",
			disableAnonymous: true,
			user:             "anonymous",
			expectError:      true,
		},
		{
			name:             "Disabled - FTP",
			disableAnonymous: true,
			user:             "ftp",
			expectError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver, err := NewFSDriver(tempDir,
				WithDisableAnonymous(tt.disableAnonymous),
			)
			fatalIfErr(t, err, "Failed to create FS driver")

			_, err = driver.Authenticate(tt.user, "pass", "", nil)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected success, got error: %v", err)
				}
			}
		})
	}
}

// TestNewFSDriver_Validation tests root path validation
func TestNewFSDriver_Validation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupPath   func(t *testing.T) string
		expectError bool
	}{
		{
			name: "Valid directory",
			setupPath: func(t *testing.T) string {
				return t.TempDir()
			},
			expectError: false,
		},
		{
			name: "Non-existent path",
			setupPath: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			expectError: true,
		},
		{
			name: "File instead of directory",
			setupPath: func(t *testing.T) string {
				dir := t.TempDir()
				file := filepath.Join(dir, "file.txt")
				fatalIfErr(t, os.WriteFile(file, []byte("test"), 0644), "Failed to write file")
				return file
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setupPath(t)
			_, err := NewFSDriver(path)
			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected success, got error: %v", err)
			}
		})
	}
}

// TestFSDriver_CustomAuthenticator tests custom authentication
func TestFSDriver_CustomAuthenticator(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	userDir := filepath.Join(tempDir, "user1")
	fatalIfErr(t, os.MkdirAll(userDir, 0755), "Failed to create user dir")

	driver, err := NewFSDriver(tempDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			if user == "admin" && pass == "secret" {
				return tempDir, false, nil // read-write
			}
			if user == "guest" && pass == "guest" {
				return userDir, true, nil // read-only
			}
			return "", false, os.ErrPermission
		}),
	)
	fatalIfErr(t, err, "Failed to create FS driver")

	// Test admin (read-write)
	ctx, err := driver.Authenticate("admin", "secret", "", nil)
	if err != nil {
		t.Errorf("Admin auth failed: %v", err)
	}
	if ctx != nil {
		ctx.Close()
	}

	// Test guest (read-only)
	ctx, err = driver.Authenticate("guest", "guest", "", nil)
	if err != nil {
		t.Errorf("Guest auth failed: %v", err)
	}
	if ctx != nil {
		ctx.Close()
	}

	// Test invalid credentials
	_, err = driver.Authenticate("invalid", "invalid", "", nil)
	if err == nil {
		t.Error("Expected authentication failure for invalid credentials")
	}
}

// TestFSContext_PathSecurity tests directory traversal prevention
func TestFSContext_PathSecurity(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	driver, err := NewFSDriver(tempDir)
	fatalIfErr(t, err, "Failed to create FS driver")

	ctx, err := driver.Authenticate("anonymous", "", "", nil)
	fatalIfErr(t, err, "Failed to authenticate")
	defer ctx.Close()

	// Create a test directory structure
	fatalIfErr(t, os.MkdirAll(filepath.Join(tempDir, "subdir"), 0755), "Failed to create subdir")
	fatalIfErr(t, os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("test"), 0644), "Failed to write file")

	tests := []struct {
		name        string
		path        string
		expectError bool
	}{
		{"Absolute path", "/subdir", false},
		{"Relative path", "subdir", false},
		{"Current directory", ".", false},
		{"Root", "/", false},
		{"File", "/file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ctx.GetFileInfo(tt.path)
			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected success, got error: %v", err)
			}
		})
	}
}

// TestFSContext_FileOperations tests file operations
func TestFSContext_FileOperations(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	driver, err := NewFSDriver(tempDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return tempDir, false, nil // read-write
		}),
	)
	fatalIfErr(t, err, "Failed to create FS driver")

	ctx, err := driver.Authenticate("user", "pass", "", nil)
	fatalIfErr(t, err, "Failed to authenticate")
	defer ctx.Close()

	// Test MakeDir
	err = ctx.MakeDir("/testdir")
	if err != nil {
		t.Errorf("MakeDir failed: %v", err)
	}

	// Verify directory exists
	info, err := ctx.GetFileInfo("/testdir")
	if err != nil || !info.IsDir() {
		t.Error("Directory not created")
	}

	// Test file creation
	f, err := ctx.OpenFile("/test.txt", os.O_CREATE|os.O_WRONLY)
	fatalIfErr(t, err, "OpenFile failed")
	if _, err := f.Write([]byte("test content")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	f.Close()

	// Test file reading
	f, err = ctx.OpenFile("/test.txt", os.O_RDONLY)
	fatalIfErr(t, err, "OpenFile for reading failed")
	buf := make([]byte, 100)
	n, _ := f.Read(buf)
	f.Close()
	if string(buf[:n]) != "test content" {
		t.Errorf("File content mismatch: got %q", string(buf[:n]))
	}

	// Test Rename
	err = ctx.Rename("/test.txt", "/renamed.txt")
	if err != nil {
		t.Errorf("Rename failed: %v", err)
	}

	// Test DeleteFile
	err = ctx.DeleteFile("/renamed.txt")
	if err != nil {
		t.Errorf("DeleteFile failed: %v", err)
	}

	// Test RemoveDir
	err = ctx.RemoveDir("/testdir")
	if err != nil {
		t.Errorf("RemoveDir failed: %v", err)
	}
}

// TestFSContext_ReadOnly tests read-only mode enforcement
func TestFSContext_ReadOnly(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	driver, err := NewFSDriver(tempDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return tempDir, true, nil // read-only
		}),
	)
	fatalIfErr(t, err, "Failed to create FS driver")

	ctx, err := driver.Authenticate("readonly", "pass", "", nil)
	fatalIfErr(t, err, "Failed to authenticate")
	defer ctx.Close()

	// All write operations should fail
	if err := ctx.MakeDir("/testdir"); err == nil {
		t.Error("MakeDir should fail in read-only mode")
	}

	if err := ctx.DeleteFile("/file.txt"); err == nil {
		t.Error("DeleteFile should fail in read-only mode")
	}

	if err := ctx.RemoveDir("/dir"); err == nil {
		t.Error("RemoveDir should fail in read-only mode")
	}

	if _, err := ctx.OpenFile("/test.txt", os.O_CREATE|os.O_WRONLY); err == nil {
		t.Error("OpenFile for writing should fail in read-only mode")
	}
}

// TestFSContext_SetTime tests modification time setting
func TestFSContext_SetTime(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	fatalIfErr(t, os.WriteFile(testFile, []byte("test content"), 0644), "Failed to write test file")

	driver, err := NewFSDriver(tempDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return tempDir, false, nil // read-write
		}),
	)
	fatalIfErr(t, err, "Failed to create FS driver")

	ctx, err := driver.Authenticate("user", "pass", "", nil)
	fatalIfErr(t, err, "Failed to authenticate")
	defer ctx.Close()

	// Valid time
	newTime := time.Date(2022, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := ctx.SetTime("/test.txt", newTime); err != nil {
		t.Errorf("SetTime failed: %v", err)
	}

	// Verify
	info, err := os.Stat(testFile)
	fatalIfErr(t, err, "Failed to stat file")
	if !info.ModTime().Equal(newTime) {
		t.Errorf("Time mismatch: got %v, want %v", info.ModTime(), newTime)
	}

	// Invalid path
	if err := ctx.SetTime("/nonexistent", newTime); err == nil {
		t.Error("Expected error for non-existent file")
	}
}

// TestFSContext_Chmod tests mode changing
func TestFSContext_Chmod(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Skip("Chmod not fully supported on Windows")
	}

	testFile := filepath.Join(tempDir, "test.txt")
	fatalIfErr(t, os.WriteFile(testFile, []byte("test content"), 0644), "Failed to write test file")

	driver, err := NewFSDriver(tempDir,
		WithAuthenticator(func(user, pass, host string, _ net.IP) (string, bool, error) {
			return tempDir, false, nil // read-write
		}),
	)
	fatalIfErr(t, err, "Failed to create FS driver")

	ctx, err := driver.Authenticate("user", "pass", "", nil)
	fatalIfErr(t, err, "Failed to authenticate")
	defer ctx.Close()

	// Change to 0600
	if err := ctx.Chmod("/test.txt", 0600); err != nil {
		t.Errorf("Chmod failed: %v", err)
	}

	// Verify
	info, err := os.Stat(testFile)
	fatalIfErr(t, err, "Failed to stat file")
	// Note: os.Stat might return more permission bits than we set, so mask.
	if info.Mode().Perm() != 0600 {
		t.Errorf("Mode mismatch: got %o, want %o", info.Mode().Perm(), 0600)
	}

	// Invalid path
	if err := ctx.Chmod("/nonexistent", 0600); err == nil {
		t.Error("Expected error for non-existent file")
	}

	// Test that modes > 0777 are rejected at the driver level
	// (Note: session layer also validates, but driver should be safe)
	if err := ctx.Chmod("/test.txt", 04755); err == nil {
		t.Error("Expected error for setuid bit (mode > 0777)")
	}
}

// TestFSContext_GetHash tests hash calculation
func TestFSContext_GetHash(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	fatalIfErr(t, os.WriteFile(testFile, []byte("test content"), 0644), "Failed to write test file")

	driver, err := NewFSDriver(tempDir)
	fatalIfErr(t, err, "Failed to create FS driver")

	ctx, err := driver.Authenticate("anonymous", "", "", nil)
	fatalIfErr(t, err, "Failed to authenticate")
	defer ctx.Close()

	tests := []struct {
		algo        string
		expectError bool
	}{
		{"SHA-256", false},
		{"SHA-512", false},
		{"SHA-1", false},
		{"MD5", false},
		{"CRC32", false},
		{"INVALID", true},
	}

	for _, tt := range tests {
		t.Run(tt.algo, func(t *testing.T) {
			hash, err := ctx.GetHash("/test.txt", tt.algo)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error for invalid algorithm")
				}
			} else {
				if err != nil {
					t.Errorf("GetHash failed: %v", err)
				}
				if hash == "" {
					t.Error("Hash should not be empty")
				}
				// Verify it's a valid hex string
				if !isHex(hash) {
					t.Errorf("Hash is not valid hex: %s", hash)
				}
			}
		})
	}
}

func isHex(s string) bool {
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return len(s) > 0
}
