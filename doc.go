// Package ftp implements an FTP client with support for both plain and secure (FTPS) connections.
//
// # Overview
//
// This package provides a developer-friendly FTP client that supports:
//   - Plain FTP connections
//   - Explicit TLS (FTPS with AUTH TLS)
//   - Implicit TLS (FTPS on port 990)
//   - Automatic TLS session reuse for data connections
//   - Progress tracking via io.Reader/Writer wrappers
//   - Robust error handling with detailed protocol context
//
// # Standards Compliance
//
// This library strictly adheres to FTP RFC specifications. For a detailed
// breakdown of supported commands, see the RFC 5797 Compliance Matrix at
// https://github.com/gonzalop/ftp/blob/main/RFC5797-compliance.md.
//
// # Basic Usage
//
// Connect to a plain FTP server:
//
//	client, err := ftp.Dial("ftp.example.com:21")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Quit()
//
//	if err := client.Login("username", "password"); err != nil {
//	    log.Fatal(err)
//	}
//
// # TLS Support
//
// There are two modes of FTPS:
//
// Explicit TLS (recommended): The client connects on port 21 and upgrades to TLS
// using the AUTH TLS command. This is the most common and recommended approach:
//
//	client, err := ftp.Dial("ftp.example.com:21",
//	    ftp.WithExplicitTLS(&tls.Config{
//	        ServerName: "ftp.example.com",
//	    }),
//	)
//
// Implicit TLS: The client connects directly with TLS on port 990. This is a
// legacy mode but still used by some servers:
//
//	client, err := ftp.Dial("ftp.example.com:990",
//	    ftp.WithImplicitTLS(&tls.Config{
//	        ServerName: "ftp.example.com",
//	    }),
//	)
//
// # TLS Session Reuse
//
// Many modern FTP servers (vsftpd, ProFTPD) require TLS session reuse between
// the control and data connections for security. This library automatically
// handles session reuse by maintaining a shared TLS session cache. No additional
// configuration is required.
//
// # File Transfers
//
// Upload a file:
//
//	file, err := os.Open("local.txt")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer file.Close()
//
//	if err := client.Store("remote.txt", file); err != nil {
//	    log.Fatal(err)
//	}
//
// Download a file:
//
//	file, err := os.Create("local.txt")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer file.Close()
//
//	if err := client.Retrieve("remote.txt", file); err != nil {
//	    log.Fatal(err)
//	}
//
// # Progress Tracking
//
// Progress tracking is implemented using the io.Reader/Writer pattern. Wrap your
// reader or writer with a progress callback:
//
//	pr := &ftp.ProgressReader{
//	    Reader: file,
//	    Callback: func(bytesTransferred int64) {
//	        fmt.Printf("Uploaded: %d bytes\n", bytesTransferred)
//	    },
//	}
//	err := client.Store("remote.txt", pr)
//
// # Error Handling
//
// Errors returned by this package include detailed protocol context. Use type
// assertion to access the full error details:
//
//	if err := client.Store("file.txt", reader); err != nil {
//	    if pe, ok := err.(*ftp.ProtocolError); ok {
//	        fmt.Printf("Command: %s\n", pe.Command)
//	        fmt.Printf("Response: %s\n", pe.Response)
//	        fmt.Printf("Code: %d\n", pe.Code)
//	    }
//	}
package ftp
