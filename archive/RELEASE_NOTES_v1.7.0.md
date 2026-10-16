# Release v1.7.0

This release focuses on **correctness and reliability** across client and server: TLS data connections (`PROT P`) now interoperate with standard clients and servers, several deadlocks, races and resource leaks are fixed, and the transfer code paths on both sides have been consolidated so fixes land in one place.

## ⚠️ Behavior Changes

- **`Abort()` no longer sends `ABOR`**: The client now closes the data connection instead; the in-progress transfer call returns an error and reads the server's failure reply. For uploads, the server may keep the part of the file it already received. This removes a data race and a hang where `Abort` and the transfer goroutine read replies from the same connection.
- **Commands containing CR or LF are rejected**: Any client method whose arguments contain `\r` or `\n` now returns an error instead of sending them to the server.
- **`StoreAt` uses `REST` + `STOR`**: With an offset greater than zero, `StoreAt` previously sent `APPE`, which writes at the end of the remote file. It now sends `REST offset` followed by `STOR`, matching how `RetrieveFrom` resumes.

## 🔒 Security Hardening

### Client
- **Command Injection Prevention**: Arguments were written to the control connection unchecked, so `c.Delete("a.txt\r\nRMD important")` sent two commands. Programs passing user input as paths could let that user run arbitrary FTP commands.
- **Active Mode Data Connection Validation**: In active mode, the client now accepts data connections only from the server's address, preventing another host from injecting or receiving transfer data.

### Server
- **File Descriptor Leak on Re-Login**: A second `USER`/`PASS` on the same connection replaced the session's filesystem without closing it, leaking one `os.Root` handle per login and allowing a single connection to exhaust the process's file descriptors. A failed re-login also left the previous login active. `USER` and `PASS` now end the current login first.
- **Per-IP Connection Limit Drift**: The per-IP counter was decremented on every `Close` of a tracked connection, and data connections are closed several times. After a few transfers the count reached zero while sessions were still open, disabling the per-IP limit. Only control connections now count, and each untracks itself once.
- **Global Connection Limit Burst**: Connections arriving together could all pass the `MaxConnections` check before any of them incremented the counter. The counter is now incremented first.
- **Passive Port Range Overflow**: The round-robin passive port counter was an `int32`; after 2³¹ `PASV`/`EPSV` commands it went negative and the server listened below `PasvMinPort`, outside the configured firewall range.
- **Telnet Filtering After `AUTH TLS`**: After the TLS upgrade, encrypted sessions stopped stripping Telnet IAC sequences as RFC 959 requires, and bytes still buffered in the Telnet reader were lost. The pooled reader and writer are now reset onto the TLS connection with buffered bytes carried over.

## 🔐 TLS / FTPS Fixes

- **Client: `PROT P` transfers never completed**: In passive mode the client ran the data TLS handshake before sending `RETR`/`STOR`, while servers start TLS only after the transfer command. In active mode it used `tls.Server`, although RFC 4217 makes the FTP client the TLS client in both modes. The client now runs the handshake as TLS client after the 1xx reply.
- **Server: deadlock with common clients**: The server opened the data connection, including the TLS handshake, before sending `150`. Clients such as Python's `ftplib` wait for `150` before starting TLS, so both sides waited until timeout. The server now sends `150` first, as RFC 959 describes; failures still reply `425`.

## 🐛 Bug Fixes

### Client
- **Hang when the server skips the 1xx reply**: A 2xx or 3xx reply to a data command was treated as "transfer starting", and the client then waited for a completion reply that never came. Only 1xx is accepted now; anything else closes the data connection and returns a `ProtocolError`.
- **`Quit()` called twice panics**: With `WithIdleTimeout`, a second `Quit` (e.g. explicit plus deferred) panicked with "close of closed channel". `Quit` now runs once; later calls return `nil`.
- **`total N` line returned as a file**: Unix servers (vsftpd, proftpd) start `LIST` output with `total N`, which `List` returned as an entry named `total 12`, making `DownloadDir` fail trying to retrieve it. These lines are now skipped.

### Server
- **`ABOR` deadlocks the session**: `handleABOR` held `s.mu` while replying, which takes `s.mu` again. Every `ABOR`, even before login, hung the session forever and kept its connection slot.
- **A finished transfer could cancel the next one**: A late deferred cleanup from a completed transfer could cancel a `STOR`/`RETR` the client had already started after seeing `226`, which then failed with `426 Transfer aborted.`
- **Interleaved multi-line replies**: `STAT`, `FEAT`, `MLST`, `HELP` and the `CWD` `.message` banner wrote without holding `s.mu`, so a `STAT` during a transfer could interleave with the final `226`/`426` reply.
- **`LIST`/`MLSD` with spaces in paths**: `LIST My Documents` listed `Documents`, and words starting with `-` anywhere were treated as flags. Only leading `-` words are flags now; the rest of the line is the path.
- **`LIST` dates without a year**: Every entry used `Jan 02 15:04`, so clients showed old files as dated this year. Like `ls`, the year is now shown for files older than six months or dated in the future.
- **Stale `REST` offset after `APPE`/`STOU`**: `REST 2; APPE f; RETR f` sent the file from byte 2. The restart offset is now always cleared after use.
- **Accept loop spinning on errors**: `Serve` retried `Accept` immediately on any error, consuming a full CPU and flooding the log when out of file descriptors, and spinning forever on a listener closed outside `Shutdown`. It now backs off from 5 ms up to 1 s and returns when the listener is closed.

### Rate Limiting
- **Limits exceeded with large chunks and shared limiters**: The limiter capped its wait at one second and then let the whole chunk through, so a 16 KB/s limit moved 32 KB/s with 32 KB writes, and sessions sharing the global limiter could each send a chunk per second. Readers were also charged for their full buffer instead of the bytes actually read. Tokens can now go negative and callers sleep until the debt is paid.

## 🧹 Refactoring & Code Quality

- **Client transfer consolidation**: `Store`, `Retrieve`, `Append`, `StoreAt`, `RetrieveFrom` and `StoreUnique` now share one transfer helper for the TYPE/REST/data connection/rate limit/finish sequence. `Append`'s copy error now reads "upload failed" like the other uploads.
- **Server transfer consolidation**: `RETR`, `STOR`, `APPE` and `STOU` now share a single `runTransfer`. This also makes `APPE` and `STOU` emit the `transfer_complete` log like the others.
- **Small deduplications**: `PASV` and `EPSV` share `openPassive` (using `time.AfterFunc` instead of a sleeping goroutine per command), and a single `remoteHost` helper replaces seven copies of the same code.
- **Test stability**: Fixed a data race in the test helpers where a server's `Serve` goroutine could log after its test had finished.

## 📖 Documentation

- Updated `docs/client.md` and `docs/client-compliance.md` to describe the new `Abort()` behavior.

## 📦 Installation

**Client:**
```bash
go get github.com/gonzalop/ftp@v1.7.0
```

**Server:**
```bash
go get github.com/gonzalop/ftp/server@v1.7.0
```

---

**Full Changelog**: https://github.com/gonzalop/ftp/compare/v1.6.1...v1.7.0
