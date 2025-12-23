# v1.1.0 - FTP Server Implementation

## 🎉 Major New Feature: FTP Server

This release introduces a complete, production-ready **FTP server implementation** alongside the existing client library, making this a comprehensive FTP solution for Go.

### Server Features

- ✅ **Pluggable Storage Backends** - Abstract `Driver` interface supports filesystem, S3, databases, or custom backends
- ✅ **Secure Filesystem Driver** - Built-in `FSDriver` uses `os.Root` for kernel-level path traversal protection
- ✅ **TLS Support** - Both Explicit (AUTH TLS) and Implicit FTPS modes
- ✅ **Flexible Authentication** - Support for anonymous access, custom authentication, and per-user directories
- ✅ **IPv6 Support** - Full IPv6 support via EPRT/EPSV commands
- ✅ **Connection Management** - Configurable connection limits and idle timeouts
- ✅ **Virtual Hosting** - HOST command support (RFC 7151)

### RFC Compliance

The server implements extensive RFC compliance:

- **RFC 959** - Base FTP Protocol (USER, PASS, CWD, LIST, RETR, STOR, etc.)
- **RFC 1123** - Requirements for Internet Hosts (ACCT, MODE, STRU, SYST, STAT, HELP)
- **RFC 2389** - Feature Negotiation (FEAT, OPTS)
- **RFC 2428** - IPv6 and NATs (EPRT, EPSV)
- **RFC 3659** - Extensions (SIZE, MDTM, MLST/MLSD, REST)
- **RFC 4217** - Securing FTP with TLS (AUTH, PROT, PBSZ)
- **RFC 7151** - HOST Command for Virtual Hosting
- **draft-bryan-ftp-hash** - HASH Command (SHA-256, SHA-512, SHA-1, MD5, CRC32)

See [RFC 5797 Compliance Matrix](https://github.com/gonzalop/ftp/blob/main/server/RFC5797-compliance.md) for detailed command implementation status.

### Quick Start

```go
package main

import (
    "log"
    "github.com/gonzalop/ftp/server"
)

func main() {
    // Create filesystem driver
    driver, err := server.NewFSDriver("/tmp/ftp")
    if err != nil {
        log.Fatal(err)
    }

    // Create and start server
    s, err := server.NewServer(":21", server.WithDriver(driver))
    if err != nil {
        log.Fatal(err)
    }

    log.Fatal(s.ListenAndServe())
}
```

### Documentation

- 📖 **[Server Documentation](https://github.com/gonzalop/ftp/blob/main/server/README.md)** - Complete guide with examples
- 📖 **[API Reference](https://pkg.go.dev/github.com/gonzalop/ftp/server)** - Full godoc documentation
- 📖 **[RFC Compliance](https://github.com/gonzalop/ftp/blob/main/server/RFC5797-compliance.md)** - Detailed command implementation status

### Examples Included

- Basic server setup with filesystem driver
- Custom authentication with per-user directories
- TLS configuration (Explicit and Implicit FTPS)
- Self-signed certificates for development
- Passive mode configuration for NAT/Docker environments
- Virtual hosting with HOST command

### Testing

- Comprehensive test suite with 44.2% coverage
- All tests passing with race detector
- Integration tests for end-to-end functionality
- RFC compliance tests

## 📚 Documentation Improvements

- Updated root README with package overview for both client and server
- Added RFC compliance summaries for both packages
- Added navigation links between client and server documentation
- Enhanced troubleshooting guides

## 📦 Installation

**Client:**
```bash
go get github.com/gonzalop/ftp@v1.1.0
```

**Server:**
```bash
go get github.com/gonzalop/ftp/server@v1.1.0
```

## 🔗 Links

- [Client Documentation](https://github.com/gonzalop/ftp/blob/main/README.md)
- [Server Documentation](https://github.com/gonzalop/ftp/blob/main/server/README.md)
- [API Reference](https://pkg.go.dev/github.com/gonzalop/ftp)
- [Examples](https://github.com/gonzalop/ftp/tree/main/examples)

---

**Full Changelog**: https://github.com/gonzalop/ftp/compare/v1.0.0...v1.1.0
