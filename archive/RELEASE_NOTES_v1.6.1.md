# Release v1.6.1

This patch release focuses on **security hardening**, **resource protection**, and **session stability** across both client and server, addressing all findings from a comprehensive codebase security and quality audit.

## 🔒 Security Hardening

### Client
- **Directory Traversal Prevention**: Enforced strict path sanitization using `filepath.Rel` and `filepath.Clean` in `DownloadDir` to prevent malicious servers from writing or overwriting arbitrary files on the client's filesystem.
- **Infinite Loop Walk Protection**: Added cycle detection (via a visited path tracking map) and a maximum recursion depth limit of 100 to `Walk` to prevent stack overflows and client crashes against recursive directory loops.

### Server
- **Passive Mode Hijacking Prevention**: Enforced IP matching validation on accepted passive data connections. The server now checks that the incoming connection's remote IP matches the control connection's remote IP (with a loopback mismatch bypass for localhost testing).
- **Enforced Authorization Checks**: Added login checks to `REST`, `MODE`, `STRU`, and `SYST` commands to ensure unauthenticated clients cannot manipulate session state or retrieve server info before logging in.

## 🛡️ Mitigation & Reliability

### Server
- **Passive Listener Port Leak Prevention**: Implemented an idle timeout (defaulting to 15 seconds) for passive connections. A background timer automatically closes the passive port listener if no data connection is made by the client within the timeout period, mitigating file descriptor and port range exhaustion.
- **Session Reader Deadlock Resolution**: Fixed a connection leak and deadlock where the reader and main loop goroutines could block waiting for each other by turning the reader synchronization channel (`cmdReqChan`) into a buffered channel of capacity 1.

## 🧹 Refactoring & Code Quality

- **prefixedConn Deduplication**: Extracted the duplicate `prefixedConn` struct and helper methods into a new internal sub-package: `github.com/gonzalop/ftp/internal/sharedconn`. Both client and server now share `sharedconn.PrefixedConn`.
- **Linter & Style Fixes**: Cleaned up unused parameters in walk callbacks to satisfy `revive` rules and formatted the codebase using `gofmt`.

## 📦 Installation

**Client:**
```bash
go get github.com/gonzalop/ftp@v1.6.1
```

**Server:**
```bash
go get github.com/gonzalop/ftp/server@v1.6.1
```

---

**Full Changelog**: https://github.com/gonzalop/ftp/compare/v1.6.0...v1.6.1
