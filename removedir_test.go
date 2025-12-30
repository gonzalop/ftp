package ftp_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gonzalop/ftp"
)

func TestRemoveDirRecursive(t *testing.T) {
	addr, cleanup, rootDir := setupServer(t)
	defer cleanup()

	c, err := ftp.Dial(addr)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Create a nested directory structure
	// test_dir/
	//   file1.txt
	//   subdir1/
	//     file2.txt
	//     subdir2/
	//       file3.txt
	//   subdir3/
	//     file4.txt

	if err := c.MakeDir("test_dir"); err != nil {
		t.Fatal(err)
	}

	if err := c.Store("test_dir/file1.txt", bytes.NewBufferString("content1")); err != nil {
		t.Fatal(err)
	}

	if err := c.MakeDir("test_dir/subdir1"); err != nil {
		t.Fatal(err)
	}

	if err := c.Store("test_dir/subdir1/file2.txt", bytes.NewBufferString("content2")); err != nil {
		t.Fatal(err)
	}

	if err := c.MakeDir("test_dir/subdir1/subdir2"); err != nil {
		t.Fatal(err)
	}

	if err := c.Store("test_dir/subdir1/subdir2/file3.txt", bytes.NewBufferString("content3")); err != nil {
		t.Fatal(err)
	}

	if err := c.MakeDir("test_dir/subdir3"); err != nil {
		t.Fatal(err)
	}

	if err := c.Store("test_dir/subdir3/file4.txt", bytes.NewBufferString("content4")); err != nil {
		t.Fatal(err)
	}

	// Verify the structure exists
	entries, err := c.List("test_dir")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 { // file1.txt, subdir1, subdir3
		t.Errorf("Expected 3 entries in test_dir, got %d", len(entries))
	}

	// Remove the entire directory recursively
	if err := c.RemoveDirRecursive("test_dir"); err != nil {
		t.Fatalf("RemoveDirRecursive failed: %v", err)
	}

	// Verify the directory is gone
	entries, err = c.List(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name == "test_dir" {
			t.Error("test_dir should have been deleted")
		}
	}

	// Verify on disk that the directory is gone
	testDirPath := filepath.Join(rootDir, "test_dir")
	if _, err := os.Stat(testDirPath); !os.IsNotExist(err) {
		t.Errorf("test_dir should not exist on disk: %s", testDirPath)
	}
}

func TestRemoveDirRecursive_EmptyDir(t *testing.T) {
	addr, cleanup, _ := setupServer(t)
	defer cleanup()

	c, err := ftp.Dial(addr)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Create an empty directory
	if err := c.MakeDir("empty_dir"); err != nil {
		t.Fatal(err)
	}

	// Remove it recursively
	if err := c.RemoveDirRecursive("empty_dir"); err != nil {
		t.Fatalf("RemoveDirRecursive on empty dir failed: %v", err)
	}

	// Verify it's gone
	entries, err := c.List(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name == "empty_dir" {
			t.Error("empty_dir should have been deleted")
		}
	}
}

func TestRemoveDirRecursive_NonExistent(t *testing.T) {
	addr, cleanup, _ := setupServer(t)
	defer cleanup()

	c, err := ftp.Dial(addr)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Try to remove a non-existent directory
	err = c.RemoveDirRecursive("nonexistent_dir")
	if err == nil {
		t.Error("RemoveDirRecursive should fail on non-existent directory")
	}
}
