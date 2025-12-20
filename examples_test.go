package ftp_test

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gonzalop/ftp"
)

// ExampleDial demonstrates connecting to a plain FTP server.
func ExampleDial() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected successfully")
}

// ExampleDial_explicitTLS demonstrates connecting with explicit TLS.
func ExampleDial_explicitTLS() {
	client, err := ftp.Dial("ftp.example.com:21",
		ftp.WithExplicitTLS(&tls.Config{
			ServerName: "ftp.example.com",
		}),
		ftp.WithTimeout(10*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected with TLS")
}

// ExampleDial_implicitTLS demonstrates connecting with implicit TLS.
func ExampleDial_implicitTLS() {
	client, err := ftp.Dial("ftp.example.com:990",
		ftp.WithImplicitTLS(&tls.Config{
			ServerName: "ftp.example.com",
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected with implicit TLS")
}

// ExampleClient_Store demonstrates uploading a file.
func ExampleClient_Store() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	file, err := os.Open("local.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	if err := client.Store("remote.txt", file); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Upload complete")
}

// ExampleClient_Retrieve demonstrates downloading a file with progress tracking.
func ExampleClient_Retrieve() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	file, err := os.Create("local.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// Wrap the writer with progress tracking
	pw := &ftp.ProgressWriter{
		Writer: file,
		Callback: func(bytesTransferred int64) {
			fmt.Printf("Downloaded: %d bytes\n", bytesTransferred)
		},
	}

	if err := client.Retrieve("remote.txt", pw); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Download complete")
}

// ExampleClient_List demonstrates listing directory contents.
func ExampleClient_List() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	entries, err := client.List("/pub")
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range entries {
		fmt.Printf("%s (%s)\n", entry.Name, entry.Type)
	}
}

// ExampleClient_MakeDir demonstrates creating a directory.
func ExampleClient_MakeDir() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	if err := client.MakeDir("newdir"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Directory created")
}

// ExampleClient_Features demonstrates querying server capabilities.
func ExampleClient_Features() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	features, err := client.Features()
	if err != nil {
		log.Fatal(err)
	}

	for feat, params := range features {
		if params != "" {
			fmt.Printf("%s: %s\n", feat, params)
		} else {
			fmt.Println(feat)
		}
	}
}

// ExampleClient_HasFeature demonstrates checking for specific features.
func ExampleClient_HasFeature() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	if client.HasFeature("MDTM") {
		fmt.Println("Server supports file modification times")
	}

	if client.HasFeature("MLST") {
		fmt.Println("Server supports machine-readable listings")
	}
}

// ExampleClient_ModTime demonstrates getting file modification time.
func ExampleClient_ModTime() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	modTime, err := client.ModTime("file.txt")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Last modified: %s\n", modTime)
}

// ExampleClient_MLList demonstrates machine-readable directory listing.
func ExampleClient_MLList() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	entries, err := client.MLList("/pub")
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range entries {
		fmt.Printf("%s: %d bytes, modified %s\n",
			entry.Name, entry.Size, entry.ModTime)
	}
}

// ExampleClient_RetrieveFrom demonstrates resuming a download.
func ExampleClient_RetrieveFrom() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	// Open file in append mode
	file, err := os.OpenFile("large.bin", os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// Get current file size to resume from
	info, err := file.Stat()
	if err != nil {
		log.Fatal(err)
	}

	// Resume download from current position
	if err := client.RetrieveFrom("large.bin", file, info.Size()); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Download resumed and completed")
}

// ExampleClient_SetOption demonstrates setting server options.
func ExampleClient_SetOption() {
	client, err := ftp.Dial("ftp.example.com:21")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login("username", "password"); err != nil {
		log.Fatal(err)
	}

	// Enable UTF8 mode if supported
	if client.HasFeature("UTF8") {
		if err := client.SetOption("UTF8", "ON"); err != nil {
			log.Printf("Failed to enable UTF8: %v", err)
		} else {
			fmt.Println("UTF8 mode enabled")
		}
	}
}
