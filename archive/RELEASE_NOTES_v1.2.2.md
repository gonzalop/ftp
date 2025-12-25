# Release v1.2.2

This release introduces new features, improves RFC compliance for resumed transfers, and refactors core logic to significantly reduce cyclomatic complexity and improve maintainability.

## ✨ New Features

### Client
- **Store Unique (STOU)**: Added support for the `STOU` command, allowing clients to upload files with unique server-generated names.
- **Improved Response Handling**: Internal refactoring now allows the client to capture and inspect the server's response message during data transfers (essential for extracting generated filenames from `STOU`).

### Server
- **Passive Port Range**: Added support for configuring a range of ports for passive data connections (`PasvMinPort`, `PasvMaxPort`), simplifying firewall configuration.
- **MLSD Control**: Added the `WithDisableMLSD` option to allow disabling the modern `MLSD` command for compatibility testing with legacy clients.
- **PublicHost Flexibility**: Clarified and improved `PublicHost` handling to support both IP addresses and hostnames (with automatic IPv4 resolution).

## 🔧 Bug Fixes

- **Resumed Transfer Correctness**: Fixed a protocol desynchronization bug where the server sent a redundant `350` response during `REST + RETR` operations. This ensures clean state transitions for resumed downloads and uploads.
- **Documentation**: Fixed typos and improved GoDoc descriptions for `PublicHost` and `MLSD` options.

## 🧹 Refactoring & Code Quality

- **Complexity Reduction**: Significantly reduced cyclomatic complexity in high-impact functions:
  - `(*session).handleCommand`: Split the giant switch statement into categorized helper methods.
  - `parseUnixEntry`: Refactored Unix-style listing parsing into focused helper functions.
  - `readResponse`: Optimized single-line response handling and isolated multi-line logic.
  - `(*session).connData`: Split active and passive connection logic for better maintainability.
- **Clean Architecture**: Improved internal code structure and grouping of command handlers.

## 🧪 Enhanced Testing & Coverage

- **Coverage Boost**: Increased statement coverage from ~62% to over 71%.
- **New Integration Tests**: Added end-to-end tests for `StoreUnique`, `StoreFrom`, `RetrieveTo`, and the complete resume flow (`RestartAt`, `RetrieveFrom`, `StoreAt`).
- **Server Coverage Suite**: Added specific tests to exercise previously uncovered error branches and compliance commands (`CDUP`, `MODE`, `STRU`, unauthenticated access).
- **Active Mode Validation**: Added specialized tests for `activeDataConn` methods including write deadlines and address reporting.

## 📦 Installation

```bash
go get github.com/gonzalop/ftp@v1.2.2
```

---

**Full Changelog**: https://github.com/gonzalop/ftp/compare/v1.2.1...v1.2.2
