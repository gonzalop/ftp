I am excited to announce the first public release of `github.com/gonzalop/ftp`, a modern, production-ready FTP client library for Go.

## 🚀 Key Features

- **Standards Compliant**: Detailed adherence to FTP specifications, including a full [RFC 5797 Compliance Matrix](https://github.com/gonzalop/ftp/blob/main/RFC5797-compliance.md).
- **Secure by Default**: First-class support for Explicit (FTPS) and Implicit TLS with automatic session reuse.
- **Robust Compatibility**: Works with virtually any FTP server thanks to our sophisticated directory listing parser.
- **Developer-friendly**: Clean API, built-in progress tracking (io.Reader/Writer wrappers), and rich error context.
- **Full Feature Set**: Support for all standard operations plus resume support (REST), feature negotiation (FEAT), and file modification times (MDTM).
- **Robust Directory Parsing**: Unix-style, DOS-style, and EPLF support.

## 📦 Installation

```bash
go get github.com/gonzalop/ftp
