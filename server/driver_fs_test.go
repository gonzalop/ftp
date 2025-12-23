package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFSDriver_DisableAnonymous(t *testing.T) {
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
			if err != nil {
				t.Fatal(err)
			}

			_, err = driver.Authenticate(tt.user, "pass", "")
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
				if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
					t.Fatal(err)
				}
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
	tempDir := t.TempDir()
	userDir := filepath.Join(tempDir, "user1")
	if err := os.MkdirAll(userDir, 0755); err != nil {
		t.Fatal(err)
	}

	driver, err := NewFSDriver(tempDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			if user == "admin" && pass == "secret" {
				return tempDir, false, nil // read-write
			}
			if user == "guest" && pass == "guest" {
				return userDir, true, nil // read-only
			}
			return "", false, os.ErrPermission
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Test admin (read-write)
	ctx, err := driver.Authenticate("admin", "secret", "")
	if err != nil {
		t.Errorf("Admin auth failed: %v", err)
	}
	if ctx != nil {
		ctx.Close()
	}

	// Test guest (read-only)
	ctx, err = driver.Authenticate("guest", "guest", "")
	if err != nil {
		t.Errorf("Guest auth failed: %v", err)
	}
	if ctx != nil {
		ctx.Close()
	}

	// Test invalid credentials
	_, err = driver.Authenticate("invalid", "invalid", "")
	if err == nil {
		t.Error("Expected authentication failure for invalid credentials")
	}
}

// TestFSContext_PathSecurity tests directory traversal prevention
func TestFSContext_PathSecurity(t *testing.T) {
	tempDir := t.TempDir()
	driver, err := NewFSDriver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := driver.Authenticate("anonymous", "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()

	// Create a test directory structure
	if err := os.MkdirAll(filepath.Join(tempDir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

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
	tempDir := t.TempDir()
	driver, err := NewFSDriver(tempDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return tempDir, false, nil // read-write
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := driver.Authenticate("user", "pass", "")
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	if _, err := f.Write([]byte("test content")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	f.Close()

	// Test file reading
	f, err = ctx.OpenFile("/test.txt", os.O_RDONLY)
	if err != nil {
		t.Fatalf("OpenFile for reading failed: %v", err)
	}
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
	tempDir := t.TempDir()
	driver, err := NewFSDriver(tempDir,
		WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return tempDir, true, nil // read-only
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := driver.Authenticate("readonly", "pass", "")
	if err != nil {
		t.Fatal(err)
	}
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

// TestFSContext_GetHash tests hash calculation
func TestFSContext_GetHash(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	driver, err := NewFSDriver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := driver.Authenticate("anonymous", "", "")
	if err != nil {
		t.Fatal(err)
	}
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
