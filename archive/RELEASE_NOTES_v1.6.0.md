# Release v1.6.0

This release focuses on **security hardening** across both client and server, **zero-copy transfer optimizations**, and new features including recursive machine-readable listings (`MLSD -R`).

## 🔒 Security Hardening

### Client
- **FTP Bounce Prevention**: The client now enforces that the data connection host matches the control connection host, preventing FTP bounce attacks where a malicious server redirects data transfers to a third-party host via forged PASV responses.
- **AUTH TLS Buffered Data Preservation**: Fixed a potential data loss issue during the TLS upgrade (AUTH TLS) by preserving any data already buffered in the `bufio.Reader` before handing the connection to the TLS layer.

### Server
- **SITE Command Login Enforcement**: Fixed a server-side panic where the `SITE` command could be executed by unauthenticated users. The handler now correctly requires login before processing.
- **Authentication Rate Limiting**: Added random jitter delay (500–1500ms) on failed login attempts to mitigate brute-force password attacks.
- **AUTH TLS Buffered Data Preservation**: Mirrored the client-side fix for the TLS upgrade path, ensuring no buffered data is lost during the transition to TLS on the server side.

## 🚀 New Features

### Client
- **Recursive Machine Listings (`MLListRecursive`)**: Added `MLListRecursive(path)` method to retrieve machine-readable directory listings recursively using `MLSD -R`, providing a modern alternative to `LIST -R`.

### Server
- **Recursive MLSD Support**: The server now supports `MLSD -R` for recursive directory listings with full path names in the output, matching the new client method.

### Client Compatibility
- **Case-Insensitive STOU Response**: The `StoreUnique` command now parses the server's `FILE:` response prefix case-insensitively, improving compatibility with servers that use non-standard casing.

## ⚡ Performance

- **Zero-Copy Transfers**: Implemented `ReadFrom` and `WriteTo` on the server's `trackingConn`, enabling the kernel to use `sendfile(2)` for downloads and `splice(2)` for uploads when no rate limiting or TLS is active. This eliminates unnecessary user-space data copies for binary-mode transfers on Linux.

## 🧹 Code Quality & Tooling

- **Linter Migration**: Replaced `golangci-lint` + `go vet` with [`revive`](https://github.com/mgechev/revive) for faster, more focused static analysis. Fixed all issues reported by the new linter, including missing doc comments on exported methods.
- **Modernized Code**: Replaced `interface{}` with `any`, `go func() + WaitGroup.Add` with `WaitGroup.Go`, and `for i := 0; i < n; i++` with `for i := range n` (Go 1.22+ integer range).
- **Makefile Improvements**: Added `clean` target for removing build artifacts.

## 📖 Documentation

- **[Alternative Transports Guide](docs/alternative-transports.md)**: New comprehensive guide covering how to implement FTP over QUIC, Unix sockets, and other custom transports, with code examples and best practices.
- **Fixed Broken Links**: Corrected documentation links in the QUIC example README.

## 📦 Installation

**Client:**
```bash
go get github.com/gonzalop/ftp@v1.6.0
```

**Server:**
```bash
go get github.com/gonzalop/ftp/server@v1.6.0
```

---

**Full Changelog**: https://github.com/gonzalop/ftp/compare/v1.5.0...v1.6.0
