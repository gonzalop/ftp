package main

import (
	"log"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	"github.com/gonzalop/ftp/server"
)

func main() {
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	logger := slog.New(slog.NewTextHandler(os.Stderr, opts))
	slog.SetDefault(logger)
	// 1. Setup a root directory for the FTP server
	rootPath := filepath.Join(os.TempDir(), "ftp-server-example")
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		log.Fatalf("Failed to create root directory: %v", err)
	}
	log.Printf("Serving files from: %s", rootPath)

	// Create a dummy file so there is something to list
	_ = os.WriteFile(filepath.Join(rootPath, "hello.txt"), []byte("Hello, FTP World!"), 0644)

	// 2. Create the Filesystem Driver
	// This driver handles the filesystem operations for the server.
	driver, err := server.NewFSDriver(rootPath,
		// Optional: Implement a custom Authenticator
		server.WithAuthenticator(func(user, pass, host string, remoteIP net.IP) (string, bool, error) {
			// For this example, we accept any user "user" with password "pass"
			// AND anonymous access.
			if user == "user" && pass == "pass" {
				// rootPath, readOnly, error
				return rootPath, false, nil
			}
			if user == "anonymous" || user == "ftp" {
				return rootPath, true, nil // Read-only for anonymous
			}
			// Return auth failure
			// Note: strict return for failure is typically (string, bool, error)
			// Returning error ensures the server knows it failed.
			return "", false, os.ErrPermission
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	// 3. Create and Start the Server
	srv, err := server.NewServer(":2121", server.WithDriver(driver))
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Starting FTP server on :2121")
	log.Println("You can connect using: ftp -P 2121 localhost")
	log.Println("  User: 'user', Pass: 'pass' (Read/Write)")
	log.Println("  User: 'anonymous' (Read-Only)")

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
