# Alternative Transports Guide

> 📖 **Navigation:** [← Main](../README.md) | [Client →](client.md) | [Server →](server.md)

This library is designed with a pluggable transport architecture, allowing you to run FTP over any reliable stream-based transport, not just TCP. Common use cases include:

- **FTP over QUIC**: Leverage QUIC's features like 0-RTT, connection migration, and head-of-line blocking avoidance.
- **FTP over Unix Sockets**: High-performance local transfers with file-system-based access control.
- **FTP over In-Memory Streams**: Useful for testing and highly specialized architectures.
- **FTP over Proxies**: Wrap your connections through specialized proxy tunnels.

---

## Table of Contents

- [Core Interfaces](#core-interfaces)
  - [Client: Dialer](#client-dialer)
  - [Server: ListenerFactory](#server-listenerfactory)
- [Implementing FTP over QUIC](#implementing-ftp-over-quic)
  - [The Challenge with QUIC](#the-challenge-with-quic)
  - [Client Implementation](#quic-client)
  - [Server Implementation](#quic-server)
- [Implementing FTP over Unix Sockets](#implementing-ftp-over-unix-sockets)
  - [Client Setup](#unix-client)
  - [Server Setup](#unix-server)
- [Best Practices](#best-practices)

---

## Core Interfaces

The library uses two primary interfaces to abstract the underlying transport for both the control connection and data connections.

### Client: Dialer

The client uses the `Dialer` interface to establish data connections (for commands like `LIST`, `RETR`, and `STOR`).

```go
type Dialer interface {
    DialContext(ctx context.Context, network, address string) (net.Conn, error)
}
```

By default, the client uses a standard `net.Dialer`. You can provide a custom implementation using the `WithCustomDialer` option.

### Server: ListenerFactory

The server uses the `ListenerFactory` interface to create listeners for passive mode (`PASV`/`EPSV`) data connections.

```go
type ListenerFactory interface {
    Listen(network, address string) (net.Listener, error)
}
```

By default, the server uses a standard `net.Listen`. You can provide a custom implementation using the `WithListenerFactory` option.

---

## Implementing FTP over QUIC

QUIC (via `quic-go`) is an excellent choice for modern FTP deployments. However, it requires some specialized handling.

### The Challenge with QUIC

In standard QUIC, `AcceptStream()` on the server side doesn't return until the client sends at least one byte of data on the stream. This creates a deadlock with FTP because:

1. The client opens a stream (e.g., for the control channel).
2. The server waits for data before `AcceptStream` returns.
3. The client waits for the server to send the "220 Service Ready" banner.

**Solution:** The client must send a "dummy" byte (e.g., `0x00`) or a protocol-level `NOOP` immediately after opening a new stream to "wake up" the server's listener.

### Client Implementation (QUIC)

Implement a `Dialer` that wraps QUIC streams:

```go
type QuicDialer struct {
    quicConn quic.Connection
}

func (d *QuicDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
    // Open a new bidirectional stream
    stream, err := d.quicConn.OpenStreamSync(ctx)
    if err != nil {
        return nil, err
    }

    // Wake up the server's AcceptStream
    _, _ = stream.Write([]byte{0x00})

    // Return a wrapper that implements net.Conn
    return NewQuicConn(stream, d.quicConn), nil
}

// Initialize client
client, _ := ftp.Dial("server:21", ftp.WithCustomDialer(&QuicDialer{quicConn: conn}))
```

### Server Implementation (QUIC)

Implement a `ListenerFactory` that yields streams from a single QUIC connection:

```go
type QuicListenerFactory struct {
    quicConn quic.Connection
}

func (f *QuicListenerFactory) Listen(network, address string) (net.Listener, error) {
    return &QuicStreamListener{quicConn: f.quicConn}, nil
}

// Use with FTP server
srv, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithListenerFactory(&QuicListenerFactory{quicConn: conn}),
    // Active mode (PORT) is typically disabled for non-TCP transports
    server.WithDisableCommands(server.ActiveModeCommands...),
)
```

**Working Example:** See the complete implementation in [examples/quic/](../examples/quic/).

---

## Implementing FTP over Unix Sockets

Unix domain sockets provide efficient local inter-process communication.

### Client Setup (Unix)

```go
client, err := ftp.Dial("/var/run/ftp.sock",
    ftp.WithDialer(&net.Dialer{
        // Force the dialer to use the "unix" network
        LocalAddr: &net.UnixAddr{Name: "/var/run/ftp.sock", Net: "unix"},
    }),
)
```

### Server Setup (Unix)

```go
ln, _ := net.Listen("unix", "/var/run/ftp.sock")
srv, _ := server.NewServer("/var/run/ftp.sock",
    server.WithDriver(driver),
)
srv.Serve(ln)
```

---

## Best Practices

1. **Disable Incompatible Commands**: Alternative transports like QUIC typically only support passive mode. Use `WithDisableCommands(server.ActiveModeCommands...)` on the server to prevent clients from trying to use `PORT` or `EPRT`.
2. **Handle Handshakes Early**: For encrypted transports, ensure TLS handshakes are completed before passing the connection to the FTP layer.
3. **Wrap Streams Carefully**: Ensure your `net.Conn` wrapper correctly implements `LocalAddr()`, `RemoteAddr()`, and deadlines, as the FTP library relies on these for timeouts and security checks.
4. **Clean Up Resources**: Custom listeners and dialers often involve long-lived connections (like a QUIC connection). Ensure these are closed gracefully when the FTP session ends.

---

## Related Documentation

- [Client Documentation](client.md)
- [Server Documentation](server.md)
- [Security Best Practices](security.md)
- [Performance Tuning Guide](performance.md)
