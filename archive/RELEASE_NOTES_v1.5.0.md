# Release v1.5.0

This release introduces major new features including support for alternative transports like **QUIC**, significant performance optimizations through buffer pooling and block processing, and enhanced security using Go 1.24's `os.Root` for filesystem isolation.

## 🚀 New Features

### Alternative Transport Support
Both the client and server are now transport-agnostic, allowing FTP operations over any reliable transport.
- **Client**: Added `WithCustomDialer(Dialer)` option to support non-TCP transports for data connections.
- **Server**: Added `WithListenerFactory(ListenerFactory)` option to support non-TCP listeners for data connections.
- **QUIC Implementation**: Comprehensive examples and helper code for running FTP over QUIC are provided in `examples/quic/`.

### Client Improvements
- **Custom Dialer**: Added `WithDialer(*net.Dialer)` to allow providing a custom dialer for the control connection, enabling fine-grained control over local addresses, timeouts, and network settings.

### Server Hardening & Customization
- **Command Control**: Added `WithDisableCommands(...string)` to selectively disable any FTP command.
- **Predefined Command Groups**: Introduced convenient groups like `LegacyCommands`, `ActiveModeCommands`, `WriteCommands`, and `SiteCommands` for quick server customization (e.g., creating read-only servers).
- **Enhanced Filesystem Security**: The `FSDriver` now utilizes Go 1.24's `os.Root` to safely jail all file operations within the root directory, providing kernel-level protection against directory traversal attacks.

## ⚡ Performance Optimizations

- **Data Transfer Pooling**: Implemented `sync.Pool` for data transfer buffers, significantly reducing GC pressure and memory allocations during high-throughput operations.
- **Optimized ASCII Mode**: Refactored ASCII transfer mode to use block processing instead of byte-by-byte conversion, resulting in a substantial speedup for text-mode transfers.
- **Session Buffer Pooling**: Added pooling for session-level command/response buffers (`bufio.Reader/Writer`) and Telnet readers.
- **Efficient Command Parsing**: Optimized the internal command reading loop and limit enforcement logic for better responsiveness.

## 🧪 Testing & Code Quality

- **Fuzz Testing**: Introduced continuous fuzz testing for the directory listing parser and the FTP client to ensure robustness against malformed or malicious server responses.
- **Improved Integration Tests**: Expanded the test suite to cover alternative transports and new configuration options.
- **CI/CD Updates**: Updated GitHub Actions workflows to support the latest Go versions and improved test reliability in CI environments.

## 📦 Installation

**Client:**
```bash
go get github.com/gonzalop/ftp@v1.5.0
```

**Server:**
```bash
go get github.com/gonzalop/ftp/server@v1.5.0
```

---

**Full Changelog**: https://github.com/gonzalop/ftp/compare/v1.4.0...v1.5.0
