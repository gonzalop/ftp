package ftp_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/gonzalop/ftp"
	"github.com/gonzalop/ftp/server"
)

func TestRecursiveHelpers(t *testing.T) {
	// Start server
	addr, s, rootDir := startServer(t)
	defer func() {
		if err := s.Shutdown(context.Background()); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Connect
	c, err := ftp.Dial(addr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit error: %v", err)
		}
	}()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// 1. Test UploadDir
	t.Run("UploadDir", func(t *testing.T) {
		// Create local source dir
		srcDir := t.TempDir()
		createTestStructure(t, srcDir)

		// Create a symlink that should be ignored
		// Pointing to a file outside the upload Root would be the security concern,
		// but even internal ones should be skipped by default.
		secretFile := filepath.Join(t.TempDir(), "secret.txt")
		if err := os.WriteFile(secretFile, []byte("secret"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(secretFile, filepath.Join(srcDir, "ignore_me.link")); err != nil {
			t.Fatal(err)
		}

		// Upload to remote
		remoteDest := "/uploaded"
		if err := c.UploadDir(srcDir, remoteDest); err != nil {
			t.Fatalf("UploadDir failed: %v", err)
		}

		// Verify on server disk
		localDest := filepath.Join(rootDir, "uploaded")
		verifyStructure(t, srcDir, localDest)

		// Ensure symlink was NOT uploaded
		if _, err := os.Stat(filepath.Join(localDest, "ignore_me.link")); err == nil {
			t.Error("Symlink was uploaded but should have been skipped")
		}
	})

	// 2. Test Walk
	t.Run("Walk", func(t *testing.T) {
		// We already have /uploaded structure on server.
		// Let's walk it.

		expectedPaths := []string{
			"/uploaded",
			"/uploaded/file1.txt",
			"/uploaded/subdir",
			"/uploaded/subdir/file2.txt",
			"/uploaded/subdir/nested",
			"/uploaded/subdir/nested/file3.txt",
		}
		sort.Strings(expectedPaths)

		var visited []string
		err := c.Walk("/uploaded", func(path string, info *ftp.Entry, err error) error {
			if err != nil {
				return err
			}
			visited = append(visited, path)
			return nil
		})

		if err != nil {
			t.Fatalf("Walk failed: %v", err)
		}

		sort.Strings(visited)

		if len(visited) != len(expectedPaths) {
			t.Fatalf("Verify visited count: got %d, want %d\nGot: %v\nWant: %v", len(visited), len(expectedPaths), visited, expectedPaths)
		}

		for i, p := range visited {
			if p != expectedPaths[i] {
				t.Errorf("Path mismatch at %d: got %s, want %s", i, p, expectedPaths[i])
			}
		}
	})

	// 3. Test DownloadDir
	t.Run("DownloadDir", func(t *testing.T) {
		destDir := t.TempDir()

		if err := c.DownloadDir("/uploaded", destDir); err != nil {
			t.Fatalf("DownloadDir failed: %v", err)
		}

		// Verify local disk matches server disk
		serverPath := filepath.Join(rootDir, "uploaded")
		verifyStructure(t, serverPath, destDir)
	})
}

func startServer(t *testing.T) (string, *server.Server, string) {
	rootDir := t.TempDir()

	driver, err := server.NewFSDriver(rootDir,
		server.WithAuthenticator(func(user, pass, host string) (string, bool, error) {
			return rootDir, false, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	s, err := server.NewServer(ln.Addr().String(), server.WithDriver(driver))
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		if err := s.Serve(ln); err != nil {
			// Serve returns ErrServerClosed on shutdown, which we can ignore or log.
			// But since we don't have access to ErrServerClosed easily without import,
			// and this is a test helper, simple logging is fine.
			// Ideally we would check for the specific error.
			t.Logf("Serve error: %v", err)
		}
	}()

	return ln.Addr().String(), s, rootDir
}

func createTestStructure(t *testing.T, dir string) {
	// file1.txt
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatal(err)
	}
	// subdir/
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	// subdir/file2.txt
	if err := os.WriteFile(filepath.Join(dir, "subdir", "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatal(err)
	}
	// subdir/nested/
	if err := os.Mkdir(filepath.Join(dir, "subdir", "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	// subdir/nested/file3.txt
	if err := os.WriteFile(filepath.Join(dir, "subdir", "nested", "file3.txt"), []byte("content3"), 0644); err != nil {
		t.Fatal(err)
	}
}

func verifyStructure(t *testing.T, srcDir, dstDir string) {
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip symlinks in verification since they are not uploaded
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		rel, _ := filepath.Rel(srcDir, path)
		dstPath := filepath.Join(dstDir, rel)

		dstInfo, err := os.Stat(dstPath)
		if err != nil {
			return fmt.Errorf("missing in dest: %s (%v)", rel, err)
		}

		if info.IsDir() {
			if !dstInfo.IsDir() {
				return fmt.Errorf("expected dir at %s", rel)
			}
		} else {
			if dstInfo.IsDir() {
				return fmt.Errorf("expected file at %s", rel)
			}
			if info.Size() != dstInfo.Size() {
				return fmt.Errorf("size mismatch at %s: %d vs %d", rel, info.Size(), dstInfo.Size())
			}
			// Verify content
			s, _ := os.ReadFile(path)
			d, _ := os.ReadFile(dstPath)
			if !bytes.Equal(s, d) {
				return fmt.Errorf("content mismatch at %s", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Errorf("Verification failed: %v", err)
	}
}
