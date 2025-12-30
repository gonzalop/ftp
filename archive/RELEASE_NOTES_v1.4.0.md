# Release v1.4.0

This release focuses on performance improvements and code quality enhancements, featuring a significantly improved bandwidth limiting implementation and new client convenience methods.

## ✨ New Features

### Client
- **Manual Keep-Alive**: Added `NoOp()` method for manual connection keep-alive when automatic keep-alive is disabled
- **Recursive Directory Removal**: Added `RemoveDirRecursive()` method for convenient recursive directory deletion
- **Enhanced Documentation**: Improved client API documentation with additional examples and clarifications

### Performance
- **Improved Bandwidth Limiting**: Replaced tick-based rate limiter with true token bucket algorithm
  - Achieves **95%+ accuracy** of configured bandwidth limits (vs ~30% with old implementation)
  - Supports burst capacity (1 second worth of data) for responsive transfers
  - Minimal overhead with adaptive sleep times
  - Fixed client downloads to properly rate-limit network reads

## 🐛 Bug Fixes

- **Client**: Fixed bandwidth limiting for downloads by wrapping network reader instead of destination writer
- **Tests**: Fixed flaky test that only failed in CI environment

## 🧪 Testing & Code Quality

- **Enhanced Test Coverage**: Added tests for REST+STOR and authentication failure scenarios
- **Updated Test Expectations**: Adjusted bandwidth test timing to account for token bucket burst behavior
- **Makefile Improvements**: Updated build and test automation

## 📜 Licensing

- **Dual License**: Added Unlicense as an alternative to MIT license for maximum flexibility
  - Users can choose between MIT (permissive) or Unlicense (public domain)

## 📦 Installation

```bash
go get github.com/gonzalop/ftp@v1.4.0
```
---

**Full Changelog**: https://github.com/gonzalop/ftp/compare/v1.3.0...v1.4.0
