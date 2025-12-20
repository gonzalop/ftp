package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gonzalop/ftp"
)

func main() {
	// Example 1: Connect to a public FTP server (GNU FTP)
	fmt.Println("=== Example 1: Plain FTP Connection ===")
	plainFTPExample()

	fmt.Println("\n=== Example 2: Explicit TLS Connection ===")
	fmt.Println("(Requires a server with TLS support)")
	// Uncomment to test with your TLS-enabled server:
	// explicitTLSExample()

	fmt.Println("\n=== Example 3: File Operations ===")
	fmt.Println("(Requires write access to a server)")
	// Uncomment to test file operations:
	// fileOperationsExample()
}

func plainFTPExample() {
	// Connect to a public FTP server
	client, err := ftp.Dial("ftp.gnu.org:21",
		ftp.WithTimeout(10*time.Second),
	)
	if err != nil {
		log.Printf("Failed to connect: %v", err)
		return
	}
	defer client.Quit()

	// Login anonymously
	if err := client.Login("anonymous", "anonymous@example.com"); err != nil {
		log.Printf("Failed to login: %v", err)
		return
	}

	fmt.Println("✓ Connected successfully")

	// Get current directory
	dir, err := client.CurrentDir()
	if err != nil {
		log.Printf("Failed to get current directory: %v", err)
		return
	}
	fmt.Printf("✓ Current directory: %s\n", dir)

	// List directory contents
	entries, err := client.List("/gnu")
	if err != nil {
		log.Printf("Failed to list directory: %v", err)
		return
	}

	fmt.Printf("✓ Found %d entries in /gnu:\n", len(entries))
	for i, entry := range entries {
		if i >= 5 {
			fmt.Printf("  ... and %d more\n", len(entries)-5)
			break
		}
		fmt.Printf("  - %s (%s)\n", entry.Name, entry.Type)
	}
}

func explicitTLSExample() {
	// Connect with explicit TLS
	client, err := ftp.Dial("ftp.example.com:21",
		ftp.WithExplicitTLS(&tls.Config{
			ServerName: "ftp.example.com",
			// For testing with self-signed certificates:
			// InsecureSkipVerify: true,
		}),
		ftp.WithTimeout(10*time.Second),
	)
	if err != nil {
		log.Printf("Failed to connect: %v", err)
		return
	}
	defer client.Quit()

	if err := client.Login("username", "password"); err != nil {
		log.Printf("Failed to login: %v", err)
		return
	}

	fmt.Println("✓ Connected with TLS")
}

func fileOperationsExample() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Quit()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	testFile := "test-upload.txt"

	// Upload with progress tracking
	fmt.Println("Uploading file...")
	pr := &ftp.ProgressReader{
		Reader: os.Stdin, // Replace with actual file
		Callback: func(bytesTransferred int64) {
			fmt.Printf("\rProgress: %d bytes", bytesTransferred)
		},
	}

	if err := client.Store(testFile, pr); err != nil {
		log.Printf("Upload failed: %v", err)
		return
	}
	fmt.Println("\n✓ Upload complete")

	// Get file size
	size, err := client.Size(testFile)
	if err != nil {
		log.Printf("Failed to get size: %v", err)
	} else {
		fmt.Printf("✓ File size: %d bytes\n", size)
	}

	// Download the file
	fmt.Println("Downloading file...")
	file, err := os.Create("downloaded-" + testFile)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	if err := client.Retrieve(testFile, file); err != nil {
		log.Printf("Download failed: %v", err)
		return
	}
	fmt.Println("✓ Download complete")

	// Clean up
	if err := client.Delete(testFile); err != nil {
		log.Printf("Failed to delete: %v", err)
	} else {
		fmt.Println("✓ File deleted")
	}
}
