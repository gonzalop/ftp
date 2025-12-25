# FTP Library for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/gonzalop/ftp.svg)](https://pkg.go.dev/github.com/gonzalop/ftp)
[![Tests](https://github.com/gonzalop/ftp/workflows/Tests/badge.svg)](https://github.com/gonzalop/ftp/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/gonzalop/ftp)](https://goreportcard.com/report/github.com/gonzalop/ftp)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A production-ready FTP library for Go providing both **client** and **server** implementations with extensive RFC compliance, TLS support, and modern extensions.

## 📦 Quick Links

- **[FTP Client →](docs/client.md)** - Full client documentation, API reference, examples
- **[FTP Server →](docs/server.md)** - Server documentation, custom drivers, deployment
- **[Examples →](examples/)** - Working code examples for common use cases
- **[Contributing →](CONTRIBUTING.md)** - How to contribute to this project

## Installation

```bash
# Client library
go get github.com/gonzalop/ftp

# Server library
go get github.com/gonzalop/ftp/server
```

## Quick Start

### Client Example

```go
package main

import (
    "log"
    "github.com/gonzalop/ftp"
)

func main() {
    // Connect to server (handles ftp, ftps, ftp+explicit)
    // and automatically logs in (default: anonymous)
    client, _ := ftp.Connect("ftp://ftp.example.com")
    defer client.Quit()
    
    // Upload file
    client.UploadFile("local.txt", "remote.txt")
    
    // Download file
    client.DownloadFile("remote.txt", "download.txt")
}
```

**[→ See full client documentation](docs/client.md)**

### Server Example

```go
package main

import (
    "log"
    "github.com/gonzalop/ftp/server"
)

func main() {
    // Start a standard server on port 21 serving files from /var/ftp
    log.Println("FTP server listening on :21")
    log.Fatal(server.ListenAndServe(":21", "/var/ftp"))
}
```

**[→ See full server documentation](docs/server.md)**

## Features

### Client Features

- **TLS/FTPS** - Both explicit (AUTH TLS) and implicit modes
- **Automatic Keep-Alive** - Prevents timeouts during idle periods or long transfers
- **Debug Logging** - Structured logging with `log/slog` for easier debugging
- **Progress Tracking** - Built-in callbacks for upload/download progress
- **File Operations** - Upload, download, append, store unique (STOU)
- **Resume Transfers** - Resume interrupted downloads/uploads (REST)
- **Modern Extensions** - MLST/MLSD, SIZE, MDTM, HASH support
- **IPv6 Support** - Full IPv6 via EPSV/EPRT (RFC 2428)
- **Rich Errors** - Detailed protocol error context

**[→ Full feature list and API reference](docs/client.md)**

### Server Features

- **Pluggable Backends** - Filesystem, S3, memory, or custom storage
- **Graceful Shutdown** - Stop accepting connections and wait for active transfers
- **TLS/FTPS** - Explicit and implicit FTPS support
- **Virtual Hosting** - HOST command for multi-tenant setups (RFC 7151)
- **File Hashing** - HASH command with SHA-256, SHA-512, MD5, CRC32

- **Secure by Default** - Built-in path validation and chroot support
- **Extensible** - Custom authentication and driver interfaces

**[→ Full server documentation](docs/server.md)**

## RFC Compliance

Both client and server implement modern FTP standards:

- **RFC 959** - File Transfer Protocol (base)
- **RFC 2389** - Feature Negotiation (FEAT)
- **RFC 2428** - IPv6 and NAT Extensions (EPSV/EPRT)
- **RFC 3659** - Extensions (MLST/MLSD, SIZE, MDTM, REST)
- **RFC 4217** - Securing FTP with TLS
- **RFC 5797** - FTP Command Registry

**Detailed compliance matrices:**
- [Client Compliance →](docs/client-compliance.md)
- [Server Compliance →](docs/server-compliance.md)

## Documentation

- **[Client Documentation](docs/client.md)** - Complete API reference, examples, TLS setup
- **[Server Documentation](docs/server.md)** - Server setup, custom drivers, authentication
- **[Examples Directory](examples/)** - Working code examples
- **[Client API (GoDoc)](https://pkg.go.dev/github.com/gonzalop/ftp)** - Client API documentation
- **[Server API (GoDoc)](https://pkg.go.dev/github.com/gonzalop/ftp/server)** - Server API documentation

## License

MIT License - see [LICENSE](LICENSE) for details

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.
