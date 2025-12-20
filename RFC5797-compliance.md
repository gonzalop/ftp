# RFC 5797 FTP Command Compliance

This document provides a detailed compliance matrix for [RFC 5797 - FTP Command and Extension Registry](https://datatracker.ietf.org/doc/html/rfc5797).

## Compliance Summary

This library implements **full support** for the following RFC 5797 FEAT codes:

- ✅ **base** - FTP standard commands (RFC 959)
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

## Detailed Command Matrix

### Base FTP Commands (RFC 959)

| Command | Description | Type | Conf | Implementation | File |
|---------|-------------|------|------|----------------|------|
| **APPE** | Append (with create) | s | m | ✅ `Append()` | [transfer.go](transfer.go) |
| **CWD** | Change Working Directory | a | m | ✅ `ChangeDir()` | [directory.go](directory.go) |
| **DELE** | Delete File | s | m | ✅ `Delete()` | [directory.go](directory.go) |
| **LIST** | List | s | m | ✅ `List()` | [directory.go](directory.go) |
| **MKD** | Make Directory | s | o | ✅ `MakeDir()` | [directory.go](directory.go) |
| **NLST** | Name List | s | m | ✅ `NameList()` | [directory.go](directory.go) |
| **PASS** | Password | a | m | ✅ `Login()` | [client.go](client.go) |
| **PASV** | Passive Mode | p | m | ✅ Internal | [data.go](data.go) |
| **PWD** | Print Directory | s | o | ✅ `CurrentDir()` | [directory.go](directory.go) |
| **QUIT** | Logout | a | m | ✅ `Quit()` | [client.go](client.go) |
| **RETR** | Retrieve | s | m | ✅ `Retrieve()` | [transfer.go](transfer.go) |
| **RMD** | Remove Directory | s | o | ✅ `RemoveDir()` | [directory.go](directory.go) |
| **RNFR** | Rename From | s/p | m | ✅ `Rename()` | [directory.go](directory.go) |
| **RNTO** | Rename To | s | m | ✅ `Rename()` | [directory.go](directory.go) |
| **STOR** | Store | s | m | ✅ `Store()` | [transfer.go](transfer.go) |
| **TYPE** | Representation Type | p | m | ✅ `Type()` | [client.go](client.go) |
| **USER** | User Name | a | m | ✅ `Login()` | [client.go](client.go) |
| **NOOP** | No-Op | s | m | ✅ `Noop()` | [client.go](client.go) |
| **PORT** | Data Port | p | m | ✅ `WithActiveMode()` | [data.go](data.go) |
| ABOR | Abort | s | m | ❌ Client closes connection | - |
| ACCT | Account | a | m | ❌ Obsolete auth method | - |
| ALLO | Allocate | s | m | ❌ Automatic on modern systems | - |
| CDUP | Change to Parent Directory | a | o | ❌ Use CWD(..) instead | - |
| HELP | Help | s | m | ❌ Client knows capabilities | - |
| MODE | Transfer Mode | p | m | ❌ Stream mode (default) only | - |
| REIN | Reinitialize | a | m | ❌ Reconnect instead | - |
| SITE | Site Parameters | s | m | ❌ Server-specific | - |
| SMNT | Structure Mount | a | o | ❌ Rarely used | - |
| STAT | Status | s | m | ❌ Not needed by client | - |
| STOU | Store Unique | a | o | ❌ Not implemented | - |
| STRU | File Structure | p | m | ❌ File structure (default) only | - |
| SYST | System | s | o | ❌ Not needed | - |
| XCUP | {obsolete: use CDUP} | s | h | 🏛️ Historic - deprecated by RFC 1123 | - |
| XCWD | {obsolete: use CWD} | s | h | 🏛️ Historic - deprecated by RFC 1123 | - |
| XMKD | {obsolete: use MKD} | s | h | 🏛️ Historic - deprecated by RFC 1123 | - |
| XPWD | {obsolete: use PWD} | s | h | 🏛️ Historic - deprecated by RFC 1123 | - |
| XRMD | {obsolete: use RMD} | s | h | 🏛️ Historic - deprecated by RFC 1123 | - |

**Legend:**
- **Type:** Command category from RFC 959 Section 4.1
  - `a` = **Access control** - Authentication and session management (USER, PASS, QUIT, etc.)
  - `p` = **Parameter** - Set transfer parameters (TYPE, MODE, PASV, etc.)
  - `s` = **Service** - Execute file operations (STOR, RETR, LIST, DELE, etc.)
  - `p/s` or `s/p` = Combination of parameter and service
- **Conf:** Conformance requirement from RFC 959
  - `m` = **Mandatory** - Must be implemented by compliant servers
  - `o` = **Optional** - May be implemented by servers
  - `h` = **Historic** - Obsolete/deprecated commands (not recommended for new implementations)
- **Implementation:** ✅ = Implemented, ❌ = Not implemented (see notes below)

---

### Security Extensions (RFC 2228)

| Command | FEAT Code | Description | Implementation | File |
|---------|-----------|-------------|----------------|------|
| **AUTH** | secu/AUTH | Authentication/Security Mechanism | ✅ `WithExplicitTLS()` | [client.go](client.go) |
| **PBSZ** | secu/PBSZ | Protection Buffer Size | ✅ Automatic | [client.go](client.go) |
| **PROT** | secu/PROT | Data Channel Protection Level | ✅ Automatic | [client.go](client.go) |
| ADAT | secu | Authentication/Security Data | ❌ Not needed | - |
| CCC | secu | Clear Command Channel | ❌ Not needed | - |
| CONF | secu | Confidentiality Protected Command | ❌ Not needed | - |
| ENC | secu | Privacy Protected Command | ❌ Not needed | - |
| MIC | secu | Integrity Protected Command | ❌ Not needed | - |

---

### NAT/IPv6 Extensions (RFC 2428)

| Command | FEAT Code | Description | Implementation | File |
|---------|-----------|-------------|----------------|------|
| **EPSV** | nat6 | Extended Passive Mode | ✅ Internal | [data.go](data.go) |
| EPRT | nat6 | Extended Port | ❌ Not needed (client) | - |

---

### Feature Negotiation (RFC 2389)

| Command | FEAT Code | Description | Implementation | File |
|---------|-----------|-------------|----------------|------|
| **FEAT** | feat | Feature Negotiation | ✅ `Features()` | [client.go](client.go) |
| **OPTS** | feat | Options | ✅ `SetOption()` | [client.go](client.go) |

---

### Extensions to FTP (RFC 3659)

| Command | FEAT Code | Description | Implementation | File |
|---------|-----------|-------------|----------------|------|
| **MDTM** | MDTM | File Modification Time | ✅ `ModTime()` | [directory.go](directory.go) |
| **MLSD** | MLST | List Directory (for machine) | ✅ `MLList()` | [mlst.go](mlst.go) |
| **MLST** | MLST | List Single Object | ✅ `MLStat()` | [mlst.go](mlst.go) |
| **REST** | REST | Restart (for STREAM mode) | ✅ `RestartAt()`, `RetrieveFrom()` | [transfer.go](transfer.go) |
| **SIZE** | SIZE | File Size | ✅ `Size()` | [directory.go](directory.go) |

---

## Implementation Notes

### Automatic Features

Some commands are used automatically by the library:
- **PBSZ/PROT** - Sent automatically after AUTH TLS
- **TYPE I** - Binary mode set automatically for transfers
- **EPSV fallback to PASV** - Automatic IPv6/IPv4 handling

### TLS Support

The library implements **RFC 4217** (Securing FTP with TLS):
- Implicit TLS (port 990)
- Explicit TLS (AUTH TLS on port 21)
- Automatic TLS session reuse for data connections
- PBSZ 0 and PROT P sent automatically

---

## Usage Examples

### Feature Detection

```go
client, _ := ftp.Dial("ftp.example.com:21")
client.Login("user", "pass")

// Query all features
features, _ := client.Features()
for feat, params := range features {
    fmt.Printf("%s: %s\n", feat, params)
}

// Check specific feature
if client.HasFeature("MLST") {
    entries, _ := client.MLList("/")
}
```

### Modern Extensions

```go
// Get file metadata
modTime, _ := client.ModTime("file.txt")
size, _ := client.Size("file.txt")

// Machine-readable listing
entries, _ := client.MLList("/pub")
for _, entry := range entries {
    fmt.Printf("%s: %d bytes, %s\n", 
        entry.Name, entry.Size, entry.ModTime)
}

// Resume interrupted download
file, _ := os.OpenFile("large.bin", os.O_WRONLY|os.O_APPEND, 0644)
info, _ := file.Stat()
client.RetrieveFrom("large.bin", file, info.Size())
```

---

## References

- [RFC 959](https://datatracker.ietf.org/doc/html/rfc959) - File Transfer Protocol (FTP)
- [RFC 2228](https://datatracker.ietf.org/doc/html/rfc2228) - FTP Security Extensions
- [RFC 2389](https://datatracker.ietf.org/doc/html/rfc2389) - Feature negotiation mechanism for FTP
- [RFC 2428](https://datatracker.ietf.org/doc/html/rfc2428) - FTP Extensions for IPv6 and NATs
- [RFC 3659](https://datatracker.ietf.org/doc/html/rfc3659) - Extensions to FTP
- [RFC 4217](https://datatracker.ietf.org/doc/html/rfc4217) - Securing FTP with TLS
- [RFC 5797](https://datatracker.ietf.org/doc/html/rfc5797) - FTP Command and Extension Registry
