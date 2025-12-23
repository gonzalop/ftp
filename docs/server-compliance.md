# RFC 5797 FTP Server Command Compliance

This document provides a detailed compliance matrix for the **Server** implementation, referencing [RFC 5797 - FTP Command and Extension Registry](https://datatracker.ietf.org/doc/html/rfc5797).

## Compliance Summary

This server implements **comprehensive** FTP protocol support, focusing on modern, secure file transfer with extensive RFC compliance.

- ✅ **base** - FTP standard commands (RFC 959) - *Full Implementation*
- ✅ **RFC 1123** - Requirements for Internet Hosts - *Full Compliance*
- ✅ **secu** - FTP Security Extensions (RFC 2228)
- ✅ **AUTH** - Authentication/Security Mechanism
- ✅ **nat6** - FTP Extensions for NAT/IPv6 (RFC 2428)
- ✅ **feat** - FTP Feature Negotiation (RFC 2389)
- ✅ **MDTM** - File Modification Time (RFC 3659)
- ✅ **MFMT** - Modify Fact: Modification Time (draft-somers-ftp-mfxx)
- ✅ **MLST** - Machine-Readable Listings (RFC 3659)
- ✅ **PBSZ** - Protection Buffer Size
- ✅ **PROT** - Data Channel Protection Level
- ✅ **REST** - Restart Transfer (RFC 3659)
- ✅ **SIZE** - File Size (RFC 3659)
- ✅ **UTF8** - UTF-8 Support (RFC 2640)
- ✅ **HOST** - Virtual Hosting (RFC 7151)
- ✅ **HASH** - File Hashes

## Detailed Command Matrix

### Base FTP Commands (RFC 959)

| Command | Description | Implementation |
|---------|-------------|----------------|
| **CWD** | Change Working Directory | ✅ Implemented |
| **CDUP** | Change to Parent Directory | ✅ Implemented |
| **LIST** | List | ✅ Implemented |
| **NLST** | Name List | ✅ Implemented |
| **NOOP** | No-Op | ✅ Implemented |
| **PASS** | Password | ✅ Implemented |
| **PASV** | Passive Mode | ✅ Implemented |
| **PORT** | Data Port | ✅ Implemented |
| **PWD** | Print Directory | ✅ Implemented |
| **QUIT** | Logout | ✅ Implemented |
| **RETR** | Retrieve | ✅ Implemented |
| **RNFR** | Rename From | ✅ Implemented |
| **RNTO** | Rename To | ✅ Implemented |
| **STOR** | Store | ✅ Implemented |
| **TYPE** | Representation Type | ✅ Implemented |
| **USER** | User Name | ✅ Implemented |
| ABOR | Abort | ❌ Client closes connection |
| **ACCT** | Account | ✅ Implemented (RFC 1123) |
| ALLO | Allocate | ❌ Automatic on modern systems |
| APPE | Append | ✅ Implemented |
| DELE | Delete File | ✅ Implemented |
| **HELP** | Help | ✅ Implemented (RFC 1123) |
| MKD | Make Directory | ✅ Implemented |
| **MODE** | Transfer Mode | ✅ Implemented (RFC 1123) |
| REIN | Reinitialize | ❌ Reconnect instead |
| RMD | Remove Directory | ✅ Implemented |
| **SITE** | Site Parameters | ✅ Implemented | HELP, CHMOD |
| SMNT | Structure Mount | ❌ Rarely used |
| **STAT** | Status | ✅ Implemented (RFC 1123) |
| STOU | Store Unique | ✅ Implemented |
| **STRU** | File Structure | ✅ Implemented (RFC 1123) |
| **SYST** | System | ✅ Implemented (RFC 1123) |

**Legend:**

- **Implementation:** ✅ = Implemented, ❌ = Not implemented

---

### Security Extensions (RFC 2228)

| Command | FEAT Code | Description | Implementation | Notes |
|---------|-----------|-------------|----------------|-------|
| **AUTH** | secu/AUTH | Authentication/Security Mechanism | ✅ Implemented | TLS only |
| **PBSZ** | secu/PBSZ | Protection Buffer Size | ✅ Implemented | Supports 0 |
| **PROT** | secu/PROT | Data Channel Protection Level | ✅ Implemented | C and P |

---

### NAT/IPv6 Extensions (RFC 2428)

| Command | FEAT Code | Description | Implementation | Notes |
|---------|-----------|-------------|----------------|-------|
| **EPSV** | nat6 | Extended Passive Mode | ✅ Implemented | |
| **EPRT** | nat6 | Extended Port | ✅ Implemented | IPv4 and IPv6 |

---

### Feature Negotiation (RFC 2389)

| Command | FEAT Code | Description | Implementation | Notes |
|---------|-----------|-------------|----------------|-------|
| **FEAT** | feat | Feature Negotiation | ✅ Implemented | explicit list |
| **OPTS** | feat | Options | ✅ Implemented | UTF8, HASH |

---

### Extensions to FTP (RFC 3659)

| Command | FEAT Code | Description | Implementation | Notes |
|---------|-----------|-------------|----------------|-------|
| **MDTM** | MDTM | File Modification Time | ✅ Implemented | |
| **MLSD** | MLST | List Directory (for machine) | ✅ Implemented | |
| **MLST** | MLST | List Single Object | ✅ Implemented | |
| **REST** | REST STREAM | Restart (for STREAM mode) | ✅ Implemented | |
| **SIZE** | SIZE | File Size | ✅ Implemented | |

---

### Other Extensions

| Command | RFC | Description | Implementation | Notes |
|---------|-----|-------------|----------------|-------|
| **HOST** | RFC 7151 | Virtual Hosting | ✅ Implemented | |
| **MFMT** | Draft | Modify Time | ✅ Implemented | |
| **HASH** | Draft | File Hash | ✅ Implemented | SHA-1, SHA-256, SHA-512, MD5, CRC32 |


---

## Implementation Notes

### Security

- The server strictly implements **RFC 4217** (Securing FTP with TLS).
- Implicit TLS is detected automatically on connection.
- Explicit TLS is supported via `AUTH TLS`.
- Data connection protection is configurable via `PROT`.

### Passive Mode

- The server implements smart passive mode handling.
- It attempts to resolve the public IP for PASV responses.
- EPSV is supported for NAT-friendly and IPv6 operation.

### Data Format

- Only `TYPE I` (Binary) and `TYPE A` (ASCII) are supported.
- `MODE` is always Stream.
- `STRU` is always File.

---

## References

- [RFC 959](https://datatracker.ietf.org/doc/html/rfc959) - File Transfer Protocol (FTP)
- [RFC 1123](https://datatracker.ietf.org/doc/html/rfc1123) - Requirements for Internet Hosts
- [RFC 2228](https://datatracker.ietf.org/doc/html/rfc2228) - FTP Security Extensions
- [RFC 2389](https://datatracker.ietf.org/doc/html/rfc2389) - Feature negotiation mechanism for FTP
- [RFC 2428](https://datatracker.ietf.org/doc/html/rfc2428) - FTP Extensions for IPv6 and NATs
- [RFC 3659](https://datatracker.ietf.org/doc/html/rfc3659) - Extensions to FTP
- [RFC 4217](https://datatracker.ietf.org/doc/html/rfc4217) - Securing FTP with TLS
- [RFC 5797](https://datatracker.ietf.org/doc/html/rfc5797) - FTP Command and Extension Registry
- [RFC 7151](https://datatracker.ietf.org/doc/html/rfc7151) - FTP HOST Command for Virtual Hosts
- [draft-somers-ftp-mfxx-04](https://datatracker.ietf.org/doc/html/draft-somers-ftp-mfxx-04) - FTP MFMT Command
- [draft-bryan-ftp-hash](https://datatracker.ietf.org/doc/html/draft-bryan-ftpext-hash-02) - FTP HASH Command


