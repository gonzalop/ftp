# FTP Server Library for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/gonzalop/ftp/server.svg)](https://pkg.go.dev/github.com/gonzalop/ftp/server)
[![Tests](https://github.com/gonzalop/ftp/workflows/Tests/badge.svg)](https://github.com/gonzalop/ftp/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/gonzalop/ftp)](https://goreportcard.com/report/github.com/gonzalop/ftp)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> 📖 **Navigation:** [← Main](../README.md) | [Client →](client.md) | [Security →](security.md) | [Performance →](performance.md) | [Examples →](../examples/) | [Compliance →](server-compliance.md)

> 💡 **Looking for the FTP client?** See [FTP Client documentation](client.md)

A flexible and modular FTP server implementation in Go. Embed an FTP server into your application with support for custom storage backends (filesystem, S3, database, etc.).

## Features

- **Pluggable Drivers**: Abstract `Driver` interface allows backends for local filesystems, S3, memory, etc.
    - Built-in `FSDriver` uses [`os.Root`](https://pkg.go.dev/os#Root) for secure filesystem access
- **RFC Compliance**: Implements key FTP RFCs for broad client compatibility
- **Bandwidth Limiting** - Global and per-user rate limits for transfer control
- **Audit Logging** - Comprehensive logging for security-relevant operations (session lifecycle, file operations, transfers)
- **IP-Based Access Control** - Authenticator receives client IP for security policies
- **Explicit TLS (FTPS)** - Secure connections using AUTH TLS (recommended)
- **Implicit TLS** - Legacy FTPS on port 990
- **Asynchronous Transfers & ABOR** - Support for aborting transfers (RFC 959)
- **Transfer Logging** - Support for standard `xferlog` format
- **Directory Messages** - Custom banner messages for directory changes
- **IPv6 Support** - Full support for IPv6 via RFC 2428 (EPRT/EPSV)
- **Modern Extensions** - Supports `SIZE`, `MDTM`, `MFMT`, `MLST/MLSD`, `STOU`, `SITE CHMOD`, `HASH`, `LIST -R` (Recursive) and more

## RFC Compliance

This server implements the following RFCs:

- **RFC 959** (File Transfer Protocol): Core commands (`USER`, `PASS`, `CWD`, `PWD`, `CDUP`, `PORT`, `PASV`, `TYPE`, `MODE`, `STRU`, `RETR`, `STOR`, `NOOP`, `QUIT`).
- **RFC 1123** (Requirements for Internet Hosts): Full compliance with minimum implementation requirements (§4.1.2.13) including `ACCT`, `MODE`, `STRU`, `SYST`, `STAT`, `HELP` commands.
- **RFC 1635** (How to Use): Informational compliance (anonymous login support depends on Driver).
- **RFC 2389** (Feature negotiation): `FEAT`, `OPTS`.
- **RFC 2428** (FTP Extensions for IPv6 and NATs): `EPRT`, `EPSV`.
- **RFC 3659** (Extensions to FTP): `SIZE`, `MDTM`, `MLSD`, `MLST`, `REST`.
- **RFC 4217** (Securing FTP with TLS): `AUTH`, `PROT`, `PBSZ`.
- **RFC 7151** (HOST Command): `HOST` (Virtual Hosting).
- **draft-somers-ftp-mfxx** (MFMT Command): `MFMT` (Modify Fact: Modification Time).
- **draft-bryan-ftp-hash** (HASH Command): `HASH` (Integrity Check -  SHA-1, SHA-256, SHA-512, MD5, CRC32).


📋 **[Detailed Compliance Matrix](server-compliance.md)** - Detailed tables of all FTP commands and their implementation status

## Installation

```bash
go get github.com/gonzalop/ftp/server
```

## Usage

### Quick Start

Start a standard server on port 21 serving files from `/var/ftp`:

```go
package main

import (
    "log"
    "github.com/gonzalop/ftp/server"
)

func main() {
    log.Fatal(server.ListenAndServe(":21", "/var/ftp"))
}
```

### Manual Setup

The following example shows how to manually configure the driver and server, which allows for more customization (e.g. disabling anonymous access).

```go
package main

import (
    "log"
    "github.com/gonzalop/ftp/server"
)

func main() {
    // Create a driver to serve a local directory
    driver, err := server.NewFSDriver("/var/ftp",
        // Optional: Disable the default anonymous login
        server.WithDisableAnonymous(true),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Create the server
    srv, err := server.NewServer(":21", server.WithDriver(driver))
    if err != nil {
        log.Fatal(err)
    }

    log.Println("Starting FTP server on :21")
    if err := srv.ListenAndServe(); err != nil {
        log.Fatal(err)
    }
}
```

### Anonymous Access & Security

By default, if no `Authenticator` is provided, `NewFSDriver` allows read-only anonymous access (using usernames `anonymous` or `ftp`).

- Use `WithDisableAnonymous(true)` to prevent these default logins.
- Use `WithAnonWrite(true)` to allow anonymous users to upload and modify files.
- If you define a custom `Authenticator` via `WithAuthenticator`, the `DisableAnonymous` flag is ignored, as your custom function takes full responsibility for deciding which users (including anonymous ones) are permitted.

### FTPS Support

The server supports both Explicit (AUTH TLS) and Implicit (legacy) FTPS modes.

#### Explicit FTPS (RFC 4217) - Recommended
Clients connect to the standard port (21) and upgrade using `AUTH TLS`.

```go
cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")
driver, _ := server.NewFSDriver("/var/ftp")
server, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithTLS(&tls.Config{Certificates: []tls.Certificate{cert}}),
)
server.ListenAndServe()
```

#### Implicit FTPS (Port 990)
The entire connection is encrypted from the start.

```go
cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")
tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}

driver, _ := server.NewFSDriver("/var/ftp")
server, _ := server.NewServer(":990",
    server.WithDriver(driver),
    server.WithTLS(tlsConfig),
)

// Listen on port 990 and wrap with TLS
l, _ := net.Listen("tcp", ":990")
tlsListener := tls.NewListener(l, tlsConfig)

server.Serve(tlsListener)
```

### Transfer Logging (xferlog)

The server can generate logs in the standard `xferlog` format, compatible with most FTP log analyzers.

```go
logFile, _ := os.OpenFile("/var/log/xferlog", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
srv, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithTransferLog(logFile),
)
```

### File Creation Mask (Umask)

You can control the default permissions for uploaded files using a `Umask`. This is configured via the `Settings` in the `FSDriver`.

```go
driver, _ := server.NewFSDriver("/var/ftp",
    server.WithSettings(&server.Settings{
        Umask: 0022, // Resulting files: 0644, Directories: 0755
    }),
)
```

### Bandwidth Limiting

Control transfer speeds with global and per-user bandwidth limits. This is useful for preventing bandwidth abuse and ensuring fair resource allocation.

```go
driver, _ := server.NewFSDriver("/var/ftp")
srv, _ := server.NewServer(":21",
    server.WithDriver(driver),
    // 10 MB/s global limit, 1 MB/s per user
    server.WithBandwidthLimit(10*1024*1024, 1024*1024),
)
```

When both limits are set, the most restrictive limit applies. Set either value to 0 for unlimited bandwidth.

#### Client Authentication (mTLS)

To require or verify client certificates, configure `ClientCAs` and `ClientAuth` in the `tls.Config` passed to `WithTLS`:

```go
// Load CA cert to trust
caCert, _ := os.ReadFile("ca.crt")
caPool := x509.NewCertPool()
caPool.AppendCertsFromPEM(caCert)

// Load server cert
cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")

config := &tls.Config{
    Certificates: []tls.Certificate{cert},
    ClientCAs:    caPool,
    ClientAuth:   tls.RequireAndVerifyClientCert, // or VerifyClientCertIfGiven
}

server, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithTLS(config),
)
```



### Authentication & Virtual Hosting

You can customize authentication and support virtual hosts using the `Authenticator` hook in `FSDriver`. This allows you to serve different directories based on the username or the `HOST` provided by the client (RFC 7151).

```go
driver, _ := server.NewFSDriver("/var/ftp/default",
    server.WithAuthenticator(func(user, pass, host string, remoteIP net.IP) (string, bool, error) {
        // Implement your logic here
        if host == "example.com" {
             if user == "admin" && pass == "secret" {
                 return "/var/ftp/example_com", false, nil // Full access
             }
        }

        // Default strict anonymous behavior
        if user == "anonymous" {
            return "/var/ftp/public", true, nil // Read-only
        }

        return "", false, fmt.Errorf("auth failed")
    }),
)
```

### Alternative Transports

The server supports custom transports (QUIC, Unix sockets, etc.) through the `WithListenerFactory` option:

```go
// Implement the ListenerFactory interface
type QuicListenerFactory struct {
    quicConn quic.Connection
}

func (f *QuicListenerFactory) Listen(network, address string) (net.Listener, error) {
    return &QuicStreamListener{quicConn: f.quicConn}, nil
}

// Use with FTP server
srv, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithListenerFactory(&QuicListenerFactory{...}),
    server.WithDisableCommands(server.ActiveModeCommands...),
)
```

See [ALTERNATIVE_TRANSPORTS.md](../ALTERNATIVE_TRANSPORTS.md) for details.

### Command Control

Disable specific FTP commands for security or transport compatibility:

```go
// Disable active mode for QUIC
srv, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithDisableCommands(server.ActiveModeCommands...),
)

// Create read-only server
srv, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithDisableCommands(server.WriteCommands...),
)

// Disable legacy commands
srv, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithDisableCommands(server.LegacyCommands...),
)
```

Predefined command groups: `ActiveModeCommands`, `WriteCommands`, `LegacyCommands`, `SiteCommands`.

## Architecture

### Server
The `Server` struct is the main entry point. Create it using `NewServer(addr, ...options)`:

```go
driver, _ := server.NewFSDriver("/var/ftp")
srv, err := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithMaxIdleTime(10*time.Minute),
    server.WithMaxConnections(100),
    server.WithDisableMLSD(true), // Optional: for compatibility testing
)
```

### Driver Interface
To support a custom backend (e.g., S3, database), implement the `Driver` interface:

```go
type Driver interface {
    Authenticate(user, pass, host string) (ClientContext, error)
}
```

And the `ClientContext` interface for session operations:
```go
type ClientContext interface {
    ChangeDir(path string) error
    ListDir(path string) ([]os.FileInfo, error)
    OpenFile(path string, flag int) (io.ReadWriteCloser, error)
    GetFileInfo(path string) (os.FileInfo, error)
    Close() error
    // ... (see driver.go for full list)
}
```

### Provided Drivers
- **FSDriver**: A production-ready driver for serving local filesystem directories. It uses Go's secure [`os.Root`](https://pkg.go.dev/os#Root) API to enforce a root jail, preventing directory traversal attacks.
