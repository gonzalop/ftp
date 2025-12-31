# FTP over QUIC Examples

This directory contains a complete, working implementation of FTP over QUIC transport, demonstrating the transport-agnostic capabilities of the `gonzalop/ftp` library.

## Overview

These examples show how to use the FTP library's pluggable transport interfaces to run FTP over QUIC instead of TCP:

- **Client**: Uses `ftp.Dialer` interface with QUIC streams for data connections
- **Server**: Uses `server.ListenerFactory` interface to accept QUIC streams for passive mode data connections
- **Common**: Provides `QuicConn` wrapper that implements `net.Conn` for QUIC streams

## Features

- ✅ Full FTP protocol over QUIC
- ✅ Interactive client with commands: `ls`, `cd`, `pwd`, `get`, `put`
- ✅ Passive mode data connections via QUIC streams
- ✅ Active mode disabled (PORT/EPRT) using `WithDisableCommands()`
- ✅ TLS encryption (QUIC includes TLS 1.3 by default)
- ✅ Anonymous and authenticated access

## Prerequisites

```bash
go get github.com/quic-go/quic-go
```

## Quick Start

### 1. Start the Server

```bash
cd server
go run main.go
```

The server will:
- Generate a self-signed TLS certificate for testing
- Listen on `:4242` (QUIC)
- Serve files from `./quic-ftp-files` directory
- Accept anonymous users (read-only) or `user`/`pass` (read-write)

### 2. Run the Client

```bash
cd client
go run main.go
```

The client will:
- Connect to the server using QUIC
- Accept the self-signed certificate (InsecureSkipVerify for testing)
- Provide an interactive FTP prompt

```
ftp> help
Available commands:
  help           - Show this help message
  pwd            - Print working directory
  ls, list, dir  - List files in current directory
  cd <dir>       - Change directory
  get <file>     - Download file
  put <file>     - Upload file
  quit, exit     - Disconnect and exit

ftp> ls
ftp> get example.txt
ftp> put myfile.txt
```

## How It Works

### Client Implementation

The client implements the `ftp.Dialer` interface to provide QUIC streams for data connections:

```go
type QuicDialer struct {
    quicConn *quic.Conn
}

func (d *QuicDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
    stream, _ := d.quicConn.OpenStreamSync(ctx)
    return common.NewQuicConn(stream, d.quicConn), nil
}
```

**Note**: The client currently handles the FTP protocol manually. A future enhancement would be to fully integrate with `ftp.Dial()` using `WithCustomDialer()`.

### Server Implementation

The server uses `server.WithListenerFactory()` to provide QUIC stream listeners for passive mode:

```go
srv, _ := server.NewServer(":0",
    server.WithDriver(driver),
    server.WithListenerFactory(&QuicDataListenerFactory{quicConn: conn}),
    server.WithDisableCommands(server.ActiveModeCommands...),
)
```

Each QUIC connection spawns a separate FTP server instance that accepts QUIC streams as data connections.

### QUIC Stream Initialization

QUIC has a quirk: `AcceptStream()` on the server doesn't return until the client sends data on the stream. Since FTP expects the server to send first (welcome banner, directory listings), we work around this by:

1. **Control stream**: Client sends `NOOP\r\n` after opening to make stream visible
2. **Data streams**: Client sends a single initialization byte (0x00) after opening each data stream
3. **Server**: Reads and discards the initialization data before FTP protocol handling

This is a QUIC-specific workaround and wouldn't be needed for other transports.

## Architecture

```
examples/quic/
├── client/          # Interactive FTP client over QUIC
│   └── main.go
├── server/          # FTP server accepting QUIC connections
│   └── main.go
├── common/          # Shared QUIC stream wrapper
│   └── quicconn.go  # Implements net.Conn for quic.Stream
├── go.mod           # Separate module with QUIC dependencies
└── README.md        # This file
```

## Command-Line Options

### Server

```bash
go run main.go [options]
  -addr string
        Server address to listen on (default ":4242")
  -root string
        Root directory for file storage (default "./quic-ftp-files")
  -cert string
        TLS certificate file (auto-generated if not provided)
  -key string
        TLS key file (auto-generated if not provided)
```

### Client

```bash
go run main.go [options]
  -server string
        Server address to connect to (default "localhost:4242")
  -user string
        Username for authentication (default "anonymous")
  -pass string
        Password for authentication (default "anonymous@")
```

## Authentication

The server accepts:
- **Anonymous**: `anonymous` or `ftp` (read-only access)
- **User**: `user` / `pass` (read-write access)

## Limitations

- Active mode (PORT/EPRT) is disabled - QUIC only supports passive mode
- The client doesn't fully integrate with `ftp.Dial()` yet (manual protocol handling)
- Both client and server use self-signed certificates for testing (not production-ready)

## Production Considerations

For production use:
1. Use proper TLS certificates (not self-signed)
2. Implement proper authentication (not hardcoded credentials)
3. Remove debug logging
4. Add error handling and retry logic
5. Consider connection pooling for multiple concurrent transfers

## Related Documentation

- [Main FTP Library Documentation](../../README.md)
- [Alternative Transports Guide](../../ALTERNATIVE_TRANSPORTS.md)
- [Server Documentation](../../docs/server.md)
- [Client Documentation](../../docs/client.md)

## License

Same as the main FTP library (MIT / Unlicense).
