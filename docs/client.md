# FTP Client Library for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/gonzalop/ftp.svg)](https://pkg.go.dev/github.com/gonzalop/ftp)
[![Tests](https://github.com/gonzalop/ftp/workflows/Tests/badge.svg)](https://github.com/gonzalop/ftp/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/gonzalop/ftp)](https://goreportcard.com/report/github.com/gonzalop/ftp)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> 📖 **Navigation:** [← Main](../README.md) | [Server →](server.md) | [Security →](security.md) | [Performance →](performance.md) | [Examples →](../examples/) | [Compliance →](client-compliance.md)

A production-ready FTP client library for Go with comprehensive TLS support, progress tracking, and a developer-friendly API.

## Features

- **Plain FTP** - Standard FTP connections
- **Explicit TLS (FTPS)** - Secure connections using AUTH TLS (recommended)
- **Implicit TLS** - Legacy FTPS on port 990
- **TLS Session Reuse** - Automatic session reuse for data connections (required by modern servers)
- **Bandwidth Limiting** - Control upload/download speeds with configurable rate limits
- **Progress Tracking** - Built-in progress callbacks via io.Reader/Writer wrappers
- **Rich Error Context** - Detailed protocol errors with command/response information
- **Directory Operations** - Full support for listing, creating, deleting directories
- **File Operations** - Upload, download, append, store unique (STOU), delete, rename files
- **Feature Negotiation (FEAT)** - Query server capabilities (RFC 2389)
- **File Metadata (MDTM)** - Get file modification times (RFC 3659)
- **Resume Support (REST)** - Resume interrupted transfers (RFC 3659)
- **Machine-Readable Listings (MLST/MLSD)** - Structured directory listings (RFC 3659)
- **Protocol Commands** - Support for `SYST` (System type), `ABOR` (Abort transfer)
- **Automatic EPSV Fallback** - Automatically disables EPSV if server returns 502, falling back to PASV
- **Virtual Hosting (HOST)** - Support for virtual hosting (RFC 7151)
- **Recursive Operations** - Walk, UploadDir, DownloadDir, RemoveDirRecursive helpers
- **Keep-Alive (NOOP)** - Manual and automatic keep-alive support

## RFC Compliance

This client implements the following RFCs:

- **RFC 959** (File Transfer Protocol): Base FTP protocol
- **RFC 2389** (Feature Negotiation): `FEAT`, `OPTS`
- **RFC 2428** (IPv6 Support): `EPSV`, `EPRT`
- **RFC 3659** (Extensions): `MLST/MLSD`, `SIZE`, `MDTM`, `REST`
- **RFC 4217** (Securing FTP with TLS): `AUTH TLS`, `PBSZ`, `PROT`
- **RFC 5797** (FTP Command Registry)
- **RFC 7151** (HOST Command): Virtual hosting support

📋 **[Detailed Compliance Matrix](client-compliance.md)** - Detailed tables of all FTP commands and their implementation status

## Installation

```bash
go get github.com/gonzalop/ftp
```

## Usage
 
 ### Quick Start
 
 Use the `Connect` helper to connect in one line using a URL. It supports `ftp://`, `ftps://` (implicit), and `ftp+explicit://`.
 
 ```go
 // Connect to a public FTP server (anonymous login default)
 client, err := ftp.Connect("ftp://ftp.example.com")
 if err != nil {
     log.Fatal(err)
 }
 defer client.Quit()
 
 // Upload a file
 client.UploadFile("local.txt", "remote.txt")
 
 // Download a file
 client.DownloadFile("remote.txt", "download.txt")
 ```
 
 ### Manual Setup

### Plain FTP Connection

```go
package main

import (
    "log"
    "github.com/gonzalop/ftp"
)

func main() {
    client, err := ftp.Dial("ftp.example.com:21")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Quit()

    if err := client.Login("username", "password"); err != nil {
        log.Fatal(err)
    }

    // List directory
    entries, err := client.List("/")
    if err != nil {
        log.Fatal(err)
    }

    for _, entry := range entries {
        log.Printf("%s (%s)\n", entry.Name, entry.Type)
    }
}
```

### Virtual Hosting (HOST)

For servers that support multiple domains (virtual hosts) on the same IP, use the `Host` command (RFC 7151) before logging in:

```go
client, err := ftp.Dial("ftp.example.com:21")
if err != nil {
    log.Fatal(err)
}
defer client.Quit()

// Specify target virtual host
if err := client.Host("ftp.example.com"); err != nil {
    log.Fatal(err)
}

if err := client.Login("username", "password"); err != nil {
    log.Fatal(err)
}
```

### Explicit TLS (Recommended)

```go
client, err := ftp.Dial("ftp.example.com:21",
    ftp.WithExplicitTLS(&tls.Config{
        ServerName: "ftp.example.com",
    }),
    ftp.WithTimeout(10*time.Second),
)
```

### Client Certificates & mTLS
 
 To authenticate with a client certificate (mutual TLS), provide a custom `tls.Config` with the `Certificates` field set:
 
 ```go
 // Load cert/key
 cert, err := tls.LoadX509KeyPair("client.crt", "client.key")
 if err != nil {
     log.Fatal(err)
 }
 
 client, err := ftp.Dial("ftp.example.com:21",
     ftp.WithExplicitTLS(&tls.Config{
         ServerName:   "ftp.example.com",
         Certificates: []tls.Certificate{cert},
     }),
     ftp.WithTimeout(10*time.Second),
 )
 ```



### Upload with Progress Tracking

```go
file, err := os.Open("large-file.bin")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

// Wrap reader with progress tracking
pr := &ftp.ProgressReader{
    Reader: file,
    Callback: func(bytesTransferred int64) {
        fmt.Printf("\rUploaded: %d bytes", bytesTransferred)
    },
}

err = client.Store("remote-file.bin", pr)
```

### Store Unique Filename (STOU)

Ask the server to generate a unique filename for your upload:

```go
file, _ := os.Open("data.txt")
defer file.Close()

// Returns the name generated by the server (e.g., "ftp-1735084800")
name, err := client.StoreUnique(file)
if err == nil {
    fmt.Printf("File stored as: %s\n", name)
}
```

### Download File

```go
file, err := os.Create("local-file.txt")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

err = client.Retrieve("remote-file.txt", file)
```

### Query Server Features

```go
features, err := client.Features()
if err != nil {
    log.Fatal(err)
}

if client.HasFeature("MDTM") {
    modTime, err := client.ModTime("file.txt")
    fmt.Printf("Last modified: %s\n", modTime)
}

if client.HasFeature("MLST") {
    // Use modern machine-readable listings
    entries, err := client.MLList("/")
    for _, entry := range entries {
        fmt.Printf("%s: %d bytes, modified %s\n",
            entry.Name, entry.Size, entry.ModTime)
    }
}
```

### Resume Interrupted Downloads

```go
// Resume a download from where it left off
file, err := os.OpenFile("large.bin", os.O_WRONLY|os.O_APPEND, 0644)
if err != nil {
    log.Fatal(err)
}
defer file.Close()

info, _ := file.Stat()
err = client.RetrieveFrom("large.bin", file, info.Size())
```

### File Hashing

```go
// Set hash algorithm (optional, defaults to server preference)
err := client.SetHashAlgo("SHA-256")

// Get file hash
hash, err := client.Hash("file.iso")
fmt.Printf("SHA-256 Hash: %s\n", hash)
```

### File Permissions (Chmod)

```go
// Change file permissions (SITE CHMOD)
err := client.Chmod("script.sh", 0755)
```

### System Information (SYST)

```go
// Get server system type
sys, err := client.Syst()
fmt.Printf("Remote system: %s\n", sys)
```

### Aborting Transfers (ABOR)

```go
// Abort an active transfer. This is typically called from a separate 
// goroutine during a long-running Store or Retrieve operation.
err := client.Abort()
```

Note: Calling `Quit()` will also actively abort any in-progress transfer by closing the data connection before the control connection.

### Raw Commands (Quote)

```go
// Send a raw command to the server (e.g., custom site commands)
resp, err := client.Quote("SITE", "UTIME", "file.txt")
fmt.Printf("Response: %s\n", resp.Message)
```

### Recursive Operations

The library provides high-level helpers for recursive file management:

#### Walk Remote Directory

Recurse through a remote directory tree, similar to `filepath.Walk`:

```go
err := client.Walk("/remote/dir", func(path string, info *ftp.Entry, err error) error {
    if err != nil {
        return err // Handle error
    }
    fmt.Printf("Visited: %s\n", path)
    if info.Type == "dir" {
        fmt.Println("Is Directory")
    }
    return nil
})
```

#### Upload Directory

Recursively upload a local directory to the server:

```go
// Upload "local_data" to "/remote/backup"
err := client.UploadDir("local_data", "/remote/backup")
```

#### Download Directory

Recursively download a remote directory to the local filesystem:

```go
// Download "/remote/logs" to "local_logs"
err := client.DownloadDir("/remote/logs", "local_logs")
```

#### Remove Directory Recursively

Recursively delete a remote directory and all its contents:

```go
// Remove directory and all files/subdirectories
err := client.RemoveDirRecursive("/old/project")
```

### Keep-Alive (NOOP)

Send a NOOP command to keep the connection alive during long operations:

```go
// Manual keep-alive
err := client.NoOp()
```

**Note:** If you use `WithIdleTimeout` when creating the client, automatic keep-alive is handled for you. The `NoOp()` method is for manual control when needed.

### Alternative Transports

The client supports custom transports (QUIC, Unix sockets, etc.) through the `WithCustomDialer` option:

```go
// Implement the Dialer interface
type QuicDialer struct {
    quicConn quic.Connection
}

func (d *QuicDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
    stream, _ := d.quicConn.OpenStreamSync(ctx)
    return quicconn.NewQuicConn(stream, d.quicConn), nil
}

// Use with FTP client
client, _ := ftp.Dial("server:21", ftp.WithCustomDialer(&QuicDialer{quicConn: conn}))
```

The custom dialer is used for passive mode data connections. See [ALTERNATIVE_TRANSPORTS.md](../ALTERNATIVE_TRANSPORTS.md) for details.

## API Reference

For complete API documentation, see [![Go Reference](https://pkg.go.dev/badge/github.com/gonzalop/ftp.svg)](https://pkg.go.dev/github.com/gonzalop/ftp)

### Supported LIST Formats

The `List()` command supports multiple directory listing formats for maximum compatibility:

**Unix-style** (most common):
- Standard 9-field: `perms links owner group size month day time/year name`
- 8-field (no group): `perms links owner size month day time/year name`
- Numeric permissions: `644 links owner group size month day time/year name`
- Symlinks: `lrwxrwxrwx ... name -> target`

**DOS/Windows-style**:
- Files: `MM-DD-YY HH:MMAM/PM size filename`
- Directories: `MM-DD-YY HH:MMAM/PM <DIR> dirname`
- Supports both `-` and `/` date separators
- Supports 2-digit and 4-digit years

**EPLF** (Easily Parsed LIST Format):
- Files: `+s<size>,<facts> filename`
- Directories: `+/,<facts> dirname`

For standardized, machine-readable listings, use `MLList()` instead (requires server support for MLSD).

### Custom Listing Parsers

For non-standard listing formats, you can implement a custom parser and register it with `Dial`:

```go
// 1. Implement ListingParser interface
type MyParser struct{}

func (p *MyParser) Parse(line string) (*ftp.Entry, bool) {
    if isMyFormat(line) {
        return parseMyFormat(line), true
    }
    return nil, false
}

// 2. Register with Dial
client, err := ftp.Dial("ftp.example.com:21",
    ftp.WithCustomListParser(&MyParser{}),
)
```

## TLS Session Reuse

Many modern FTP servers (vsftpd, ProFTPD) require TLS session reuse between control and data connections for security. This library automatically handles session reuse using a shared `tls.ClientSessionCache`. No additional configuration is required.

When TLS is enabled, the library automatically enables data channel protection (PROT P) for all data connections, ensuring that file transfers and listings are encrypted.

## Error Handling

The library provides rich error context through the `ProtocolError` type:

```go
if err := client.Store("file.txt", reader); err != nil {
    if pe, ok := err.(*ftp.ProtocolError); ok {
        fmt.Printf("Command: %s\n", pe.Command)
        fmt.Printf("Response: %s\n", pe.Response)
        fmt.Printf("Code: %d\n", pe.Code)

        if pe.IsTemporary() {
            // Retry logic
        }
    }
}
```

## Testing

Run the unit tests:

```bash
go test -v
```

All tests should pass:
```
=== RUN   TestReadResponse_SingleLine
=== RUN   TestReadResponse_MultiLine
=== RUN   TestParsePASV
=== RUN   TestParseEPSV
=== RUN   TestResponse_CodeChecks
=== RUN   TestProtocolError
PASS
```

## Implementation Details

### Response Parser

The library implements a robust multi-line response parser that handles:

- Single-line responses: `220 Welcome\r\n`
- Multi-line responses: `220-Line 1\r\n220-Line 2\r\n220 Done\r\n`
- Edge cases and malformed responses

### Data Connections

Data connections use PASV (IPv4) or EPSV (IPv6) modes. The library:

1. Sends EPSV first (for IPv6 support)
2. Falls back to PASV if EPSV is not supported
3. Automatically wraps data connections in TLS when enabled
4. Reuses TLS sessions from the control connection

### Binary Mode

All file transfers default to binary mode (TYPE I) for reliability.

## License

MIT

## Contributing

Contributions are welcome! Please ensure all tests pass before submitting a PR.
