package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"log/slog"
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

	fmt.Println("\n=== Example 3: Non-Mutating Operations (GNU FTP) ===")
	fmt.Println("(Requires an empty directory called 'temp')")
	// testNonMutatingOperations()

	fmt.Println("\n=== Example 4: File Operations ===")
	fmt.Println("(Requires write access to a server)")
	// Uncomment to test file operations:
	// fileOperationsExample()

	fmt.Println("\n=== Example 5: Custom Listing Parser ===")
	fmt.Println("(See the code)")
	// customParserExample()
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
		fmt.Printf("  - %q (%s)\n", entry.Name, entry.Type)
	}
}

// testNonMutatingOperations demonstrates read-only operations on GNU FTP server
func testNonMutatingOperations() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	//s, _ := server.NewServer(":21",
	//server.WithDriver(driver),
	//server.WithLogger(logger),

	// Connect to GNU FTP server
	client, err := ftp.Dial("ftp.gnu.org:21",
		ftp.WithTimeout(10*time.Second),
		ftp.WithLogger(logger),
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
	fmt.Println("✓ Connected to ftp.gnu.org")

	// 1. Discover server features
	fmt.Println("\n--- Server Features ---")
	features, err := client.Features()
	if err != nil {
		log.Printf("Failed to get features: %v", err)
	} else {
		fmt.Printf("✓ Server supports %d features:\n", len(features))
		for feat, params := range features {
			if params != "" {
				fmt.Printf("  - %s: %s\n", feat, params)
			} else {
				fmt.Printf("  - %s\n", feat)
			}
		}
	}

	// 2. Get current directory
	fmt.Println("\n--- Current Directory ---")
	dir, err := client.CurrentDir()
	if err != nil {
		log.Printf("Failed to get current directory: %v", err)
	} else {
		fmt.Printf("✓ Current directory: %s\n", dir)
	}

	// 3. List directory contents
	fmt.Println("\n--- Directory Listing ---")
	entries, err := client.List("/gnu/screen")
	if err != nil {
		log.Printf("Failed to list directory: %v", err)
	} else {
		fmt.Printf("✓ Found %d entries in /gnu:\n", len(entries))
		for i, entry := range entries {
			if i >= 10 {
				fmt.Printf("  ... and %d more\n", len(entries)-10)
				break
			}
			fmt.Printf("  - %s (%s, %d bytes)\n", entry.Name, entry.Type, entry.Size)
		}
	}

	// 4. Test name list
	fmt.Println("\n--- Name List ---")
	names, err := client.NameList("/gnu")
	if err != nil {
		log.Printf("Failed to get name list: %v", err)
	} else {
		fmt.Printf("✓ Found %d names (first 5):\n", len(names))
		for i, name := range names {
			if i >= 5 {
				break
			}
			fmt.Printf("  - %s\n", name)
		}
	}

	// 5. Test file size if we found a file
	if len(entries) > 0 {
		for _, entry := range entries {
			if entry.Type == "file" {
				fmt.Println("\n--- File Size ---")
				size, err := client.Size(entry.Name)
				if err != nil {
					log.Printf("Failed to get size for %s: %v", entry.Name, err)
				} else {
					fmt.Printf("✓ %s: %d bytes\n", entry.Name, size)
				}
				break
			}
		}
	}

	// 6. Test modification time if supported
	if client.HasFeature("MDTM") && len(entries) > 0 {
		for _, entry := range entries {
			if entry.Type == "file" {
				fmt.Println("\n--- File Modification Time ---")
				modTime, err := client.ModTime(entry.Name)
				if err != nil {
					log.Printf("Failed to get mod time for %s: %v", entry.Name, err)
				} else {
					fmt.Printf("✓ %s modified: %s\n", entry.Name, modTime.Format("2006-01-02 15:04:05 MST"))
				}
				break
			}
		}
	}

	// 7. Test NOOP (keep-alive)
	fmt.Println("\n--- Keep-Alive Test ---")
	if err := client.Noop(); err != nil {
		log.Printf("NOOP failed: %v", err)
	} else {
		fmt.Println("✓ NOOP successful (connection alive)")
	}

	fmt.Println("\n✓ All non-mutating operations completed successfully")

	// 8. Download screen
	fmt.Println("\n--- Download screen ---")
	client.DownloadDir("/gnu/screen", "temp")
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

func customParserExample() {
	// Define a custom parser for a hypothetical format
	// Format: "FILENAME|SIZE|TYPE"
	myParser := &MyCustomParser{}

	// Connect with the custom parser
	// Note: We use a dummy address here just to show initialization
	client, err := ftp.Dial("ftp.example.com:21",
		ftp.WithCustomListParser(myParser),
		ftp.WithTimeout(10*time.Second),
	)
	if err != nil {
		fmt.Printf("Note: Dial failed as expected (dummy host): %v\n", err)
		return
	}
	defer client.Quit()

	// In a real scenario, List() would now use myParser
	// entries, _ := client.List("/")
}

// MyCustomParser implements the ListingParser interface
type MyCustomParser struct{}

func (p *MyCustomParser) Parse(line string) (*ftp.Entry, bool) {
	// Simple example parsing logic
	// In reality, you would check if the line matches your custom format
	return nil, false
}
