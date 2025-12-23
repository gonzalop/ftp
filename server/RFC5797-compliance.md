# RFC 5797 FTP Server Command Compliance

This document provides a detailed compliance matrix for the **Server** implementation, referencing [RFC 5797 - FTP Command and Extension Registry](https://datatracker.ietf.org/doc/html/rfc5797).

## Compliance Summary

This server implements a **subset** of the standard FTP commands, focusing on modern, secure file transfer.

- ✅ **base** - FTP standard commands (RFC 959) - *Full Implementation*
- ✅ **RFC 1123** - Requirements for Internet Hosts - *Full Compliance*
- ✅ **secu** - FTP Security Extensions (RFC 2228)
- ✅ **AUTH** - Authentication/Security Mechanism
- ✅ **nat6** - FTP Extensions for NAT/IPv6 (RFC 2428)
- ✅ **feat** - FTP Feature Negotiation (RFC 2389)
- ✅ **MDTM** - File Modification Time (RFC 3659)
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

| Command | Description | Type | Conf | Implementation | Handler |
|---------|-------------|------|------|----------------|---------|
| **CWD** | Change Working Directory | a | m | ✅ Implemented | `handleCWD` |
| **CDUP** | Change to Parent Directory | a | o | ✅ Implemented | `handleCDUP` |
| **LIST** | List | s | m | ✅ Implemented | `handleLIST` |
| **NLST** | Name List | s | m | ✅ Implemented | `handleNLST` |
| **NOOP** | No-Op | s | m | ✅ Implemented | `handleNOOP` |
| **PASS** | Password | a | m | ✅ Implemented | `handlePASS` |
| **PASV** | Passive Mode | p | m | ✅ Implemented | `handlePASV` |
| **PORT** | Data Port | p | m | ✅ Implemented | `handlePORT` |
| **PWD** | Print Directory | s | o | ✅ Implemented | `handlePWD` |
| **QUIT** | Logout | a | m | ✅ Implemented | `handleQUIT` |
| **RETR** | Retrieve | s | m | ✅ Implemented | `handleRETR` |
| **RNFR** | Rename From | s/p | m | ✅ Implemented | `handleRNFR` |
| **RNTO** | Rename To | s | m | ✅ Implemented | `handleRNTO` |
| **STOR** | Store | s | m | ✅ Implemented | `handleSTOR` |
| **TYPE** | Representation Type | p | m | ✅ Implemented | `handleTYPE` |
| **USER** | User Name | a | m | ✅ Implemented | `handleUSER` |
| ABOR | Abort | s | m | ❌ Client closes connection | - |
| **ACCT** | Account | a | m | ✅ Implemented (RFC 1123) | `handleACCT` |
| ALLO | Allocate | s | m | ❌ Automatic on modern systems | - |
| APPE | Append | s | m | ✅ Implemented | `handleAPPE` |
| DELE | Delete File | s | m | ✅ Implemented | `handleDELE` |
| **HELP** | Help | s | m | ✅ Implemented (RFC 1123) | `handleHELP` |
| MKD | Make Directory | s | o | ✅ Implemented | `handleMKD` |
| **MODE** | Transfer Mode | p | m | ✅ Implemented (RFC 1123) | `handleMODE` |
| REIN | Reinitialize | a | m | ❌ Reconnect instead | - |
| RMD | Remove Directory | s | o | ✅ Implemented | `handleRMD` |
| **SITE** | Site Parameters | s | m | ✅ Implemented | `handleSITE` |
| SMNT | Structure Mount | a | o | ❌ Rarely used | - |
| **STAT** | Status | s | m | ✅ Implemented (RFC 1123) | `handleSTAT` |
| STOU | Store Unique | a | o | ✅ Implemented | `handleSTOU` |
| **STRU** | File Structure | p | m | ✅ Implemented (RFC 1123) | `handleSTRU` |
| **SYST** | System | s | o | ✅ Implemented (RFC 1123) | `handleSYST` |

**Legend:**
- **Type:** Command category from RFC 959 (Access control, Parameter, Service)
- **Conf:** Conformance requirement (m=Mandatory, o=Optional)
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
| **REST** | REST | Restart (for STREAM mode) | ✅ Implemented | |
| **SIZE** | SIZE | File Size | ✅ Implemented | |

---

### Other Extensions

| Command | RFC | Description | Implementation | Notes |
|---------|-----|-------------|----------------|-------|
| **HOST** | RFC 7151 | Virtual Hosting | ✅ Implemented | |
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
