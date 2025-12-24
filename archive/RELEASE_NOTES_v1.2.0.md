# Release v1.2.0

This release introduces significant improvements in client robustness, observability, and server feature completeness.

## 🚀 New Features

### Client
- **Automatic Keep-Alive**: The new `WithIdleTimeout(duration)` option enables an automatic background keep-alive mechanism. The client will send `NOOP` commands if the connection is idle on the control channel, preventing server timeouts. It intelligently detects active data transfers to avoid corrupting the command stream.
- **Debug Logging**: Added `WithLogger(*slog.Logger)` option to enable detailed structured logging of all FTP commands, responses, and connection state changes.
- **Robust Timeout Handling**: The `WithTimeout` option now applies strict deadlines to **all** network operations, including:
  - Initial connection and greeting
  - Implicit and Explicit TLS handshakes
  - Control commands (read/write)
  - Active mode (`PORT`) listener acceptance
  - Data transfer completion responses
- **Rolling Idle Timeouts**: Data connections now enforce a "rolling" deadline, ensuring that long file transfers don't timeout as long as data is flowing, while still catching stalled connections.

### Server
- **Graceful Shutdown**: The new `Shutdown(context.Context)` method allows the server to stop accepting new connections and strictly wait for active transfers to complete (up to the context deadline) before closing resources.
- **MFMT Support**: Added support for the `MFMT` command (RFC 3659), allowing clients to set the modification time of files.
- **SITE CHMOD Support**: Added support for `SITE CHMOD` to allow clients to change file permissions.

## ⚡ Improvements

- **Protocol Optimization**: The client now tracks the current transfer mode (`TYPE I`/`TYPE A`) and avoids sending redundant `TYPE` commands before every file transfer, significantly reducing round-trips for batch operations.
- **Thread Safety**: The client's control connection is now protected by a mutex, ensuring safe concurrent access between user commands and the background keep-alive routine.

## 📦 API Changes

- **Server**: `Server.Shutdown` now accepts a `context.Context`.
- **Client**: `WithDebug` (deprecated/internal) replaced/standardized to `WithLogger`.
