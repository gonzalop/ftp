# RFC 5797 FTP Client Command Compliance

This document provides a detailed compliance matrix for the **Client** implementation, referencing [RFC 5797 - FTP Command and Extension Registry](https://datatracker.ietf.org/doc/html/rfc5797).

## Compliance Summary

This library implements **comprehensive support** for the following RFC 5797 FEAT codes:

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
- ✅ **HOST** - Virtual Hosting (RFC 7151)

## Detailed Command Matrix

### Base FTP Commands (RFC 959)

| Command | Description | Implementation |
|---------|-------------|----------------|
| **APPE** | Append (with create) | ✅ `Append()` |
| **CWD** | Change Working Directory | ✅ `ChangeDir()` |
| **DELE** | Delete File | ✅ `Delete()` |
| **LIST** | List | ✅ `List()` |
| **MKD** | Make Directory | ✅ `MakeDir()` |
| **NLST** | Name List | ✅ `NameList()` |
| **PASS** | Password | ✅ `Login()` |
| **PASV** | Passive Mode | ✅ Internal |
| **PWD** | Print Directory | ✅ `CurrentDir()` |
| **QUIT** | Logout | ✅ `Quit()` |
| **RETR** | Retrieve | ✅ `Retrieve()` |
| **RMD** | Remove Directory | ✅ `RemoveDir()` |
| **RNFR** | Rename From | ✅ `Rename()` |
| **RNTO** | Rename To | ✅ `Rename()` |
| **STOR** | Store | ✅ `Store()` |
| **TYPE** | Representation Type | ✅ `Type()` |
| **USER** | User Name | ✅ `Login()` |
| **NOOP** | No-Op | ✅ `Noop()` |
| **PORT** | Data Port | ✅ `WithActiveMode()` |
| ABOR | Abort | ⚠️ `Abort()` closes the data connection instead of sending ABOR |
| ACCT | Account | ❌ Obsolete auth method |
| ALLO | Allocate | ❌ Automatic on modern systems |
| CDUP | Change to Parent Directory | ✅ `ChangeDirToParent()` |
| HELP | Help | ❌ Client knows capabilities |
| MODE | Transfer Mode | ❌ Stream mode (default) only |
| REIN | Reinitialize | ❌ Reconnect instead |
| SITE | Site Parameters | ✅ `Chmod()` (for SITE CHMOD) |
| SMNT | Structure Mount | ❌ Rarely used |
| STAT | Status | ❌ Not needed by client |
| STOU | Store Unique | ✅ `StoreUnique()` |
| STRU | File Structure | ❌ File structure (default) only |
| **SYST** | System | ✅ `Syst()` |
| XCUP | {obsolete: use CDUP} | ✅🏛️ Historic - deprecated by RFC 1123 |
| XCWD | {obsolete: use CWD} | ✅🏛️ Historic - deprecated by RFC 1123 |
| XMKD | {obsolete: use MKD} | ✅🏛️ Historic - deprecated by RFC 1123 |
| XPWD | {obsolete: use PWD} | ✅🏛️ Historic - deprecated by RFC 1123 |
| XRMD | {obsolete: use RMD} | ✅🏛️ Historic - deprecated by RFC 1123 |

**Legend:**

- **Implementation:** ✅ = Implemented, ❌ = Not implemented, 🏛️ = Historic/deprecated

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
| **EPRT** | nat6 | Extended Port | ✅ Internal | [data.go](data.go) |

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
| **REST** | REST STREAM | Restart (for STREAM mode) | ✅ `RestartAt()`, `RetrieveFrom()` | [transfer.go](transfer.go) |
| **SIZE** | SIZE | File Size | ✅ `Size()` | [directory.go](directory.go) |

---

### Virtual Hosting (RFC 7151)

| Command | FEAT Code | Description | Implementation | File |
|---------|-----------|-------------|----------------|------|
| **HOST** | HOST | Virtual Hosting | ✅ `Host()` | [client.go](client.go) |

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
- [RFC 7151](https://datatracker.ietf.org/doc/html/rfc7151) - FTP HOST Command for Virtual Hosts
