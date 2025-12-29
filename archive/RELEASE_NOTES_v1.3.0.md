# Release v1.3.0

This release introduces major new features for security and performance, including bandwidth limiting, comprehensive audit logging, IP-based access control, and enhanced client reliability. It also includes extensive new documentation covering security and performance best practices.

## ✨ New Features

### Client
- **Improved EPSV Handling**: Client now automatically disables `EPSV` for the remainder of the session if the server responds with a 502 (Not Implemented) error, falling back to `PASV` for subsequent data connections.
- **Bandwidth Limiting**: Added `WithBandwidthLimit` option to control upload/download speeds using a token bucket algorithm.
- **Graceful Transfer Abort**: Modified `Quit` to actively abort in-progress transfers by closing the data connection, preventing hangs during shutdown.
- **Enhanced Protocol Support**: Implemented `ChangeDirToParent` (CDUP) command and documented RFC 7151 (HOST) support.
- **Security Enhancement**: Password masking in debug logs to prevent credential leakage.

### Server
- **Bandwidth Limiting**: Added `WithBandwidthLimit` option supporting both global and per-user rate limits for all transfer operations (RETR, STOR, APPE, STOU).
- **Comprehensive Audit Logging**: Added detailed logging for security-relevant operations:
  - Session lifecycle events (session start with IP address)
  - File rename operations (RNFR/RNTO) with from/to paths
  - Permission changes (SITE CHMOD) with path and mode
  - Modification time changes (MFMT) with path and timestamp
  - Transfer completion with bandwidth metrics
- **IP-Based Access Control**: Updated `Driver` interface and `WithAuthenticator` to include client IP address (`net.IP`), enabling:
  - IP whitelisting/blacklisting
  - Subnet-based restrictions
  - Geo-based access control
  - Brute force protection by IP
- **Enhanced RFC Compliance**: Added support for historic RFC 1123 aliases (XCWD, XCUP, XPWD) and fixed FEAT response to advertise MLST and EPRT.
- **Privacy Compliance**: IP redaction now consistently applied across all server logging when `WithRedactIPs` is enabled.

## 📚 Documentation

- **New Security Guide** (`docs/security.md`): Comprehensive coverage of:
  - Client and server TLS configuration
  - Certificate validation and custom CA handling
  - Authentication and access control patterns
  - IP-based access control with practical examples
  - Brute force protection strategies
  - File system security and privacy compliance
  - Network security and deployment checklist

- **New Performance Guide** (`docs/performance.md`): Comprehensive coverage of:
  - Client optimization (connection pooling, timeouts, bandwidth)
  - Server optimization (connection limits, bandwidth, caching)
  - Transfer optimization (buffer sizes, compression, resume)
  - Monitoring and profiling techniques
  - Benchmarking examples and best practices

- **Improved Navigation**: Added cross-links between README, client.md, server.md, and new guides for easier navigation.
- **Updated Examples**: Authenticator examples now demonstrate IP-based access control using `net.IP` parameter.

## 🧪 Enhanced Testing & Code Quality

- **Test Consolidation**: Merged multiple small test files into unified integration test suites for both client and server packages, improving maintainability.
- **Expanded Client Tests**: Added comprehensive tests for `Connect` helper and EPSV fallback behavior.

## 📦 Installation

```bash
go get github.com/gonzalop/ftp@v1.3.0
```

---

**Full Changelog**: https://github.com/gonzalop/ftp/compare/v1.2.3...v1.3.0
