# FTP Library for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/gonzalop/ftp.svg)](https://pkg.go.dev/github.com/gonzalop/ftp)
[![Tests](https://github.com/gonzalop/ftp/workflows/Tests/badge.svg)](https://github.com/gonzalop/ftp/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/gonzalop/ftp)](https://goreportcard.com/report/github.com/gonzalop/ftp)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A comprehensive, production-ready FTP library providing both **client** and **server** implementations with extensive RFC compliance.

## 📦 Packages

### FTP Client - `github.com/gonzalop/ftp`
Production-ready FTP client with TLS support, progress tracking, and a developer-friendly API.

**Quick Install:** `go get github.com/gonzalop/ftp`

**RFC Compliance:**
- ✅ RFC 959 (Base FTP Protocol)
- ✅ RFC 2389 (Feature Negotiation - FEAT)
- ✅ RFC 2428 (IPv6 Support - EPSV/EPRT)
- ✅ RFC 3659 (Extensions - MLST/MLSD, SIZE, MDTM, REST)
- ✅ RFC 4217 (Securing FTP with TLS)
- ✅ RFC 5797 (FTP Command and Extension Registry) - [Compliance Matrix](RFC5797-compliance.md)

**[📖 Documentation below](#ftp-client-documentation)**

---

### FTP Server - `github.com/gonzalop/ftp/server`
Flexible, embeddable FTP server with pluggable storage backends (filesystem, S3, custom).

**Quick Install:** `go get github.com/gonzalop/ftp/server`

**RFC Compliance:**
- ✅ RFC 959 (Base FTP Protocol)
- ✅ RFC 1123 (Requirements for Internet Hosts)
- ✅ RFC 2389 (Feature Negotiation - FEAT, OPTS)
- ✅ RFC 2428 (IPv6 Support - EPSV/EPRT)
- ✅ RFC 3659 (Extensions - MLST/MLSD, SIZE, MDTM, REST)
- ✅ RFC 4217 (Securing FTP with TLS)
- ✅ RFC 7151 (HOST Command for Virtual Hosting)
- ✅ draft-bryan-ftp-hash (HASH Command - SHA-256, SHA-512, MD5, etc.)

**[📖 Server Documentation →](./server/README.md)** | **[Compliance Details →](./server/RFC5797-compliance.md)**

---

## FTP Client Documentation

A production-ready FTP client library with comprehensive TLS support, progress tracking, and a developer-friendly API.


## Features

- ✅ **Plain FTP** - Standard FTP connections
- ✅ **Explicit TLS (FTPS)** - Secure connections using AUTH TLS (recommended)
- ✅ **Implicit TLS** - Legacy FTPS on port 990
- ✅ **TLS Session Reuse** - Automatic session reuse for data connections (required by modern servers)
- ✅ **Progress Tracking** - Built-in progress callbacks via io.Reader/Writer wrappers
- ✅ **Rich Error Context** - Detailed protocol errors with command/response information
- ✅ **Directory Operations** - Full support for listing, creating, deleting directories
- ✅ **File Operations** - Upload, download, append, delete, rename files
- ✅ **Feature Negotiation (FEAT)** - Query server capabilities (RFC 2389)
- ✅ **File Metadata (MDTM)** - Get file modification times (RFC 3659)
- ✅ **Resume Support (REST)** - Resume interrupted transfers (RFC 3659)
- ✅ **Machine-Readable Listings (MLST/MLSD)** - Structured directory listings (RFC 3659)

📋 **[RFC 5797 Compliance Matrix](RFC5797-compliance.md)** - Detailed command implementation status



## Installation

```bash
go get github.com/gonzalop/ftp
```

## Quick Start

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

### Explicit TLS (Recommended)

```go
client, err := ftp.Dial("ftp.example.com:21",
    ftp.WithExplicitTLS(&tls.Config{
        ServerName: "ftp.example.com",
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

## API Reference

[![Go Reference](https://pkg.go.dev/badge/github.com/gonzalop/ftp.svg)](https://pkg.go.dev/github.com/gonzalop/ftp)

### Connection Options

- `WithTimeout(duration)` - Set connection and operation timeout
- `WithExplicitTLS(config)` - Enable explicit TLS (AUTH TLS)
- `WithImplicitTLS(config)` - Enable implicit TLS (port 990)
- `WithDebug(logger)` - Enable debug logging
- `WithDialer(dialer)` - Use custom net.Dialer
- `WithActiveMode()` - Use active mode (PORT) instead of passive (PASV/EPSV)
- `WithDisableEPSV()` - Disable EPSV command (force PASV)

### File Operations

- `Store(remotePath, reader)` - Upload from io.Reader
- `StoreFrom(remotePath, localPath)` - Upload local file
- `Retrieve(remotePath, writer)` - Download to io.Writer
- `RetrieveTo(remotePath, localPath)` - Download to local file
- `Append(remotePath, reader)` - Append to remote file
- `Delete(path)` - Delete file
- `Rename(from, to)` - Rename file or directory
- `Size(path)` - Get file size
- `ModTime(path)` - Get file modification time (MDTM)
- `RestartAt(offset)` - Set restart marker for next transfer
- `RetrieveFrom(path, writer, offset)` - Resume download from offset
- `StoreAt(path, reader, offset)` - Resume upload from offset

### Directory Operations

- `List(path)` - List directory with details
- `NameList(path)` - Simple name list (NLST)
- `ChangeDir(path)` - Change working directory
- `CurrentDir()` - Get current directory
- `MakeDir(path)` - Create directory
- `RemoveDir(path)` - Remove directory

#### Supported LIST Formats

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

#### Custom Listing Parsers

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

### Feature Negotiation

- `Features()` - Query server capabilities (FEAT)
- `HasFeature(name)` - Check if feature is supported
- `SetOption(option, value)` - Set feature options (OPTS)

### Machine-Readable Listings

- `MLStat(path)` - Get structured file info (MLST)
- `MLList(path)` - Get structured directory listing (MLSD)

### Connection Management

- `Noop()` - Send keepalive to prevent timeout

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
