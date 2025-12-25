# Release v1.2.3

This release introduces significant ease-of-use improvements for both client and server, including high-level helpers, support for `ABOR` (abort) and `SYST` commands, recursive directory listings, and enhanced server logging.

## ✨ New Features

### Client
- **High-Level Helpers**: Added `Connect`, `UploadFile`, and `DownloadFile` functions for common operations, significantly reducing boilerplate for simple tasks.
- **Protocol Commands**: Implemented support for `ABOR` (Abort) and `SYST` (System type) commands.
- **Improved Documentation**: Updated documentation with examples for new helper functions.

### Server
- **Asynchronous Transfers & ABOR**: Implemented support for the `ABOR` command through a new asynchronous transfer mechanism, allowing clients to cancel long-running transfers gracefully.
- **Enhanced Directory Support**:
    - Added support for recursive directory listings (`LIST -R`).
    - Added directory messages (banners) to provide context when users change directories.
- **Logging & Visibility**: Implemented `xferlog` support for standard FTP transfer logging.
- **Configuration & Ease of Use**:
    - Added `ListenAndServe` helper for quick server startup.
    - Added `Settings.Umask` support for better control over uploaded file permissions (similar to `local_umask` in vsftpd).
    - Added support for anonymous writes.
- **Transfer Modes**: Added support for ASCII transfer mode.

## 🔧 Bug Fixes

- **Stability**: Added timeouts to `Shutdown` and other network operations to prevent potential hangs during connection closure or network issues.

## 🧪 Enhanced Testing & Code Quality

- **Test Performance**: Parallelized integration tests to significantly reduce the time required for the test suite to run.
- **Robustness**: Improved test coverage for shutdown scenarios and network timeouts.

## 📦 Installation

```bash
go get github.com/gonzalop/ftp@v1.2.3
```

---

**Full Changelog**: https://github.com/gonzalop/ftp/compare/v1.2.2...v1.2.3
