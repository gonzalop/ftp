# Release v1.2.1

This release focuses on stability, thread-safety, and significantly improved test coverage. It addresses several data races and edge cases discovered through rigorous stress testing.

## 🛡️ Stability & Thread Safety

- **Control Connection Synchronization**: The client's `sendCommand` is now protected by a mutex, ensuring that background keep-alive `NOOP` commands do not interleave with user-initiated commands on the control socket.
- **Race-Free Testing**: Fixed several data races in the test suite by introducing a thread-safe `safeBuffer` for log capture and properly synchronizing access to shared test state.
- **Robust Active Data Connections**: The client's active mode (`PORT`) now correctly binds to the local interface used by the control connection, improving reliability in multi-homed environments and NAT scenarios.

## ✨ New Features

### Client
- **Virtual Hosting (HOST)**: Added support for the `HOST` command (RFC 7151), allowing the client to specify the target virtual host before authentication.

### Server
- **Dynamic Working Directory**: Fixed `PWD` command to correctly report the user's current working directory instead of always returning `/`.
- **RFC 3659 Compliance**: Fixed `MLST` response formatting to include the required leading space for entry lines.

## 🔧 Improvements & Bug Fixes

- **Server**: Fixed an off-by-one error in the `maxConnectionsPerIP` limit logic.
- **Makefile**: Added `test-race` for easy race detection and `coverage` for generating comprehensive HTML coverage reports.
- **Code Quality**: Removed redundant `TYPE` commands and cleaned up unused imports.

## 🧪 Enhanced Testing

- **Comprehensive Integration Suite**: Introduced `client_integration_test.go` with over 700 lines of end-to-end tests covering:
  - Implicit and Explicit TLS
  - Automatic Keep-Alive
  - Active and Passive data transfers
  - Directory operations and recursive transfers
  - Feature negotiation and HASH extensions
- **Limit Testing**: Added `server/limits_test.go` to verify global and per-IP connection limits.
- **Dynamic Ports**: Refactored all integration tests to use dynamic ports (port 0), eliminating "address already in use" conflicts in CI environments.

## 📦 Installation

```bash
go get github.com/gonzalop/ftp@v1.2.1
```

---

**Full Changelog**: https://github.com/gonzalop/ftp/compare/v1.2.0...v1.2.1
