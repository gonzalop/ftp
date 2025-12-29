# Security Best Practices

> 📖 **Navigation:** [← Main](../README.md) | [Client →](client.md) | [Server →](server.md)

This guide covers security best practices for both the FTP client and server implementations.

---

## Table of Contents

- [Client Security](#client-security)
  - [TLS/FTPS Configuration](#tlsftps-configuration)
  - [Certificate Validation](#certificate-validation)
  - [Client Certificates (mTLS)](#client-certificates-mtls)
  - [Credential Management](#credential-management)
- [Server Security](#server-security)
  - [TLS/FTPS Setup](#tlsftps-setup)
  - [Authentication](#authentication)
  - [Access Control](#access-control)
  - [Brute Force Protection](#brute-force-protection)
  - [File System Security](#file-system-security)
  - [Privacy & Compliance](#privacy--compliance)
- [Network Security](#network-security)
- [Deployment Checklist](#deployment-checklist)

---

## Client Security

### TLS/FTPS Configuration

**Always use explicit or implicit FTPS in production.** Plain FTP transmits credentials and data in cleartext, making it vulnerable to interception.

#### Explicit TLS (Recommended)

Explicit TLS (AUTH TLS) is the recommended mode. The client connects on port 21 and upgrades to TLS:

```go
package main

import (
    "crypto/tls"
    "log"
    "github.com/gonzalop/ftp"
)

func main() {
    // Secure client with explicit TLS
    client, err := ftp.Dial("ftp.example.com:21",
        ftp.WithExplicitTLS(&tls.Config{
            ServerName: "ftp.example.com",
            MinVersion: tls.VersionTLS12, // Require TLS 1.2 or higher
        }),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Quit()

    // Login and use
    if err := client.Login("username", "password"); err != nil {
        log.Fatal(err)
    }
}
```

#### Implicit TLS (Legacy)

Implicit TLS wraps the entire connection in TLS from the start, typically on port 990:

```go
client, err := ftp.Dial("ftp.example.com:990",
    ftp.WithImplicitTLS(&tls.Config{
        ServerName: "ftp.example.com",
        MinVersion: tls.VersionTLS12,
    }),
)
```

#### Using the Connect Helper

The `Connect` helper automatically handles TLS based on the URL scheme:

```go
// Explicit TLS
client, err := ftp.Connect("ftp+explicit://user:pass@ftp.example.com")

// Implicit TLS
client, err := ftp.Connect("ftps://user:pass@ftp.example.com:990")
```

---

### Certificate Validation

> ⚠️ **WARNING:** Never use `InsecureSkipVerify: true` in production!

#### Production: Proper Certificate Validation

```go
client, err := ftp.Dial("ftp.example.com:21",
    ftp.WithExplicitTLS(&tls.Config{
        ServerName: "ftp.example.com",
        MinVersion: tls.VersionTLS12,
        // Let Go verify against system CA pool
    }),
)
```

#### Development: Self-Signed Certificates

For development/testing with self-signed certificates, add the CA to a custom cert pool:

```go
import (
    "crypto/x509"
    "os"
)

// Load custom CA certificate
caCert, err := os.ReadFile("ca.crt")
if err != nil {
    log.Fatal(err)
}

caCertPool := x509.NewCertPool()
caCertPool.AppendCertsFromPEM(caCert)

client, err := ftp.Dial("ftp.example.com:21",
    ftp.WithExplicitTLS(&tls.Config{
        ServerName: "ftp.example.com",
        RootCAs:    caCertPool,
        MinVersion: tls.VersionTLS12,
    }),
)
```

#### Last Resort: Skip Verification (Development Only)

```go
// ⚠️ ONLY for development/testing!
client, err := ftp.Dial("ftp.example.com:21",
    ftp.WithExplicitTLS(&tls.Config{
        InsecureSkipVerify: true, // DO NOT USE IN PRODUCTION
    }),
)
```

---

### Client Certificates (mTLS)

For mutual TLS authentication, provide a client certificate:

```go
// Load client certificate and key
cert, err := tls.LoadX509KeyPair("client.crt", "client.key")
if err != nil {
    log.Fatal(err)
}

client, err := ftp.Dial("ftp.example.com:21",
    ftp.WithExplicitTLS(&tls.Config{
        ServerName:   "ftp.example.com",
        Certificates: []tls.Certificate{cert},
        MinVersion:   tls.VersionTLS12,
    }),
)
```

---

### Credential Management

**Never hardcode credentials in source code.**

#### Use Environment Variables

```go
import "os"

username := os.Getenv("FTP_USERNAME")
password := os.Getenv("FTP_PASSWORD")

client, err := ftp.Connect(fmt.Sprintf("ftp://%s:%s@ftp.example.com",
    url.QueryEscape(username),
    url.QueryEscape(password)))
```

#### Use Configuration Files (with Restricted Permissions)

```go
import (
    "encoding/json"
    "os"
)

type Config struct {
    Host     string `json:"host"`
    Username string `json:"username"`
    Password string `json:"password"`
}

// Load from config file (ensure file has 0600 permissions)
data, err := os.ReadFile("ftp-config.json")
var config Config
json.Unmarshal(data, &config)

client, err := ftp.Dial(config.Host)
client.Login(config.Username, config.Password)
```

#### Use Secret Management Services

For production, use dedicated secret management:
- AWS Secrets Manager
- HashiCorp Vault
- Kubernetes Secrets
- Azure Key Vault

---

## Server Security

### TLS/FTPS Setup

#### Explicit TLS (Recommended)

```go
package main

import (
    "crypto/tls"
    "log"
    "github.com/gonzalop/ftp/server"
)

func main() {
    // Load server certificate
    cert, err := tls.LoadX509KeyPair("server.crt", "server.key")
    if err != nil {
        log.Fatal(err)
    }

    // Create driver
    driver, err := server.NewFSDriver("/var/ftp")
    if err != nil {
        log.Fatal(err)
    }

    // Create server with TLS
    srv, err := server.NewServer(":21",
        server.WithDriver(driver),
        server.WithTLS(&tls.Config{
            Certificates: []tls.Certificate{cert},
            MinVersion:   tls.VersionTLS12,
            CipherSuites: []uint16{
                tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
                tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
            },
        }),
    )
    if err != nil {
        log.Fatal(err)
    }

    log.Println("Starting secure FTP server on :21")
    log.Fatal(srv.ListenAndServe())
}
```

#### Implicit TLS (Port 990)

```go
import "net"

tlsConfig := &tls.Config{
    Certificates: []tls.Certificate{cert},
    MinVersion:   tls.VersionTLS12,
}

srv, _ := server.NewServer(":990",
    server.WithDriver(driver),
    server.WithTLS(tlsConfig),
)

// Listen with TLS wrapper
ln, _ := tls.Listen("tcp", ":990", tlsConfig)
srv.Serve(ln)
```

#### Certificate Management

**Best Practices:**
- Use certificates from trusted CAs (Let's Encrypt, DigiCert, etc.)
- Set up automatic certificate renewal
- Monitor certificate expiration
- Use strong key sizes (2048-bit RSA minimum, 4096-bit recommended)

**Let's Encrypt Example:**

```bash
# Use certbot to get certificates
certbot certonly --standalone -d ftp.example.com

# Certificates will be in:
# /etc/letsencrypt/live/ftp.example.com/fullchain.pem
# /etc/letsencrypt/live/ftp.example.com/privkey.pem
```

```go
cert, err := tls.LoadX509KeyPair(
    "/etc/letsencrypt/live/ftp.example.com/fullchain.pem",
    "/etc/letsencrypt/live/ftp.example.com/privkey.pem",
)
```

---

### Authentication

**Never use default anonymous access in production.**

#### Custom Authenticator

Implement a custom `Authenticator` for production deployments:

```go
import "net"

driver, err := server.NewFSDriver("/var/ftp/default",
    server.WithAuthenticator(func(user, pass, host string, remoteIP net.IP) (string, bool, error) {
        // Example: Database-backed authentication
        valid, rootPath, readOnly := validateUser(user, pass)
        if !valid {
            return "", false, fmt.Errorf("authentication failed")
        }

        return rootPath, readOnly, nil
    }),
)
```

#### IP-Based Access Control

Restrict access by IP address using the `remoteIP` parameter:

```go
import "net"

type IPFilter struct {
    allowedNetworks []*net.IPNet
}

func (f *IPFilter) IsAllowed(ip net.IP) bool {
    if ip == nil {
        return false
    }
    for _, network := range f.allowedNetworks {
        if network.Contains(ip) {
            return true
        }
    }
    return false
}

// Parse allowed networks
_, net1, _ := net.ParseCIDR("192.168.1.0/24")
_, net2, _ := net.ParseCIDR("10.0.0.0/8")
filter := &IPFilter{
    allowedNetworks: []*net.IPNet{net1, net2},
}

driver, _ := server.NewFSDriver("/var/ftp",
    server.WithAuthenticator(func(user, pass, host string, remoteIP net.IP) (string, bool, error) {
        // Check if IP is allowed
        if !filter.IsAllowed(remoteIP) {
            return "", false, fmt.Errorf("IP not allowed")
        }

        // Continue with credential validation
        return validateCredentials(user, pass)
    }),
)
```

**Additional IP-based examples:**

```go
// Block specific IPs
server.WithAuthenticator(func(user, pass, host string, remoteIP net.IP) (string, bool, error) {
    blocked := []string{"192.168.1.100", "10.0.0.50"}
    for _, ip := range blocked {
        if remoteIP.String() == ip {
            return "", false, fmt.Errorf("IP blocked")
        }
    }
    return validateCredentials(user, pass)
})

// Allow only localhost
server.WithAuthenticator(func(user, pass, host string, remoteIP net.IP) (string, bool, error) {
    if remoteIP != nil && !remoteIP.IsLoopback() {
        return "", false, fmt.Errorf("only localhost allowed")
    }
    return validateCredentials(user, pass)
})
```

#### Password Security

**Best Practices:**
- Hash passwords using bcrypt, scrypt, or Argon2
- Never store plaintext passwords
- Enforce strong password policies
- Consider multi-factor authentication (MFA)

```go
import "golang.org/x/crypto/bcrypt"

// Hash password when creating user
hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

// Verify during authentication
err := bcrypt.CompareHashAndPassword(hashedPassword, []byte(providedPassword))
if err != nil {
    return "", false, fmt.Errorf("invalid password")
}
```

---

### Brute Force Protection

Protect against password guessing attacks by limiting connection attempts.

#### Built-in Connection Limits

```go
srv, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithMaxConnections(100, 5), // Max 100 total, 5 per IP
)
```

#### Failed Login Tracking

Implement failed login tracking in your authenticator:

```go
import (
    "sync"
    "time"
)

type FailedLoginTracker struct {
    mu       sync.Mutex
    attempts map[string][]time.Time
}

func (t *FailedLoginTracker) RecordFailure(ip string) {
    t.mu.Lock()
    defer t.mu.Unlock()

    t.attempts[ip] = append(t.attempts[ip], time.Now())
}

func (t *FailedLoginTracker) IsBlocked(ip string) bool {
    t.mu.Lock()
    defer t.mu.Unlock()

    attempts := t.attempts[ip]

    // Remove attempts older than 15 minutes
    cutoff := time.Now().Add(-15 * time.Minute)
    valid := attempts[:0]
    for _, attempt := range attempts {
        if attempt.After(cutoff) {
            valid = append(valid, attempt)
        }
    }
    t.attempts[ip] = valid

    // Block if 3+ failed attempts in window
    return len(valid) >= 3
}

// Use in authenticator
tracker := &FailedLoginTracker{
    attempts: make(map[string][]time.Time),
}

driver, _ := server.NewFSDriver("/var/ftp",
    server.WithAuthenticator(func(user, pass, host string, remoteIP net.IP) (string, bool, error) {
        ip := remoteIP.String()

        if tracker.IsBlocked(ip) {
            return "", false, fmt.Errorf("too many failed attempts")
        }

        valid := validateCredentials(user, pass)
        if !valid {
            tracker.RecordFailure(ip)
            return "", false, fmt.Errorf("invalid credentials")
        }

        return "/var/ftp", false, nil
    }),
)
```

---

### File System Security

#### Chroot Jail with os.Root

The `FSDriver` uses Go's `os.Root` API to enforce a chroot jail, preventing directory traversal attacks:

```go
// FSDriver automatically uses os.Root for security
driver, err := server.NewFSDriver("/var/ftp")

// Users cannot access files outside /var/ftp
// Attempts to access /../etc/passwd will fail
```

#### File Permissions (Umask)

Control default permissions for uploaded files:

```go
driver, _ := server.NewFSDriver("/var/ftp",
    server.WithSettings(&server.Settings{
        Umask: 0022, // Files: 0644, Directories: 0755
    }),
)
```

#### Disable Anonymous Write Access

```go
driver, _ := server.NewFSDriver("/var/ftp",
    server.WithDisableAnonymous(true), // Disable anonymous entirely
    // OR
    // server.WithAnonWrite(false), // Allow anonymous read-only
)
```

---

### Privacy & Compliance

#### IP Address Redaction (GDPR)

Enable IP redaction in logs for privacy compliance:

```go
srv, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithRedactIPs(true), // "192.168.1.100" → "192.168.1.xxx"
)
```

#### Custom Path Redaction

Redact sensitive information from logged paths:

```go
import (
    "strings"
    "regexp"
)

srv, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithPathRedactor(func(path string) string {
        // Redact user IDs from paths like /users/12345/file.txt
        re := regexp.MustCompile(`/users/\d+/`)
        return re.ReplaceAllString(path, "/users/*/")
    }),
)
```

#### Transfer Logging (Audit Trail)

Enable standard xferlog format for compliance:

```go
import "os"

logFile, _ := os.OpenFile("/var/log/xferlog",
    os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

srv, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithTransferLog(logFile),
)
```

---

## Network Security

### Firewall Configuration

#### Client (Passive Mode)
- Outbound: Port 21 (control)
- Outbound: High ports for data (server-dependent)

#### Server (Passive Mode - Recommended)
- Inbound: Port 21 (control)
- Inbound: Passive port range (configure with `WithPasvMinPort`/`WithPasvMaxPort`)

```go
srv, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithPasvMinPort(50000),
    server.WithPasvMaxPort(51000),
)
```

**Firewall rules:**
```bash
# Allow FTP control
iptables -A INPUT -p tcp --dport 21 -j ACCEPT

# Allow passive data ports
iptables -A INPUT -p tcp --dport 50000:51000 -j ACCEPT
```

### DMZ Deployment

For internet-facing FTP servers:
1. Deploy in DMZ (demilitarized zone)
2. Use separate network segment
3. Restrict access to internal networks
4. Enable comprehensive logging
5. Monitor for suspicious activity

---

## Deployment Checklist

### Client Deployment

- [ ] Use FTPS (explicit or implicit TLS)
- [ ] Validate server certificates (no `InsecureSkipVerify` in production)
- [ ] Store credentials securely (environment variables, secret management)
- [ ] Use TLS 1.2 or higher
- [ ] Implement connection timeouts
- [ ] Enable keep-alive for long-running operations
- [ ] Log errors and connection issues
- [ ] Handle network failures gracefully

### Server Deployment

- [ ] Enable TLS/FTPS with valid certificates
- [ ] Disable anonymous access (or restrict to read-only)
- [ ] Implement custom authentication
- [ ] Set connection limits (`WithMaxConnections`)
- [ ] Configure passive port range for firewall
- [ ] Set appropriate file permissions (umask)
- [ ] Enable transfer logging for audit trail
- [ ] Configure IP redaction for privacy compliance
- [ ] Implement brute force protection
- [ ] Monitor failed login attempts
- [ ] Set up certificate renewal automation
- [ ] Enable structured logging
- [ ] Configure read/write timeouts
- [ ] Test graceful shutdown
- [ ] Document security policies

---

## Additional Resources

### RFCs
- [RFC 4217](https://datatracker.ietf.org/doc/html/rfc4217) - Securing FTP with TLS
- [RFC 2228](https://datatracker.ietf.org/doc/html/rfc2228) - FTP Security Extensions

### Related Documentation
- [Client Documentation](client.md)
- [Server Documentation](server.md)
- [Contributing Guidelines](../CONTRIBUTING.md)

### External Resources
- [OWASP FTP Security](https://owasp.org/www-community/vulnerabilities/FTP)
- [NIST TLS Guidelines](https://csrc.nist.gov/publications/detail/sp/800-52/rev-2/final)
- [Let's Encrypt](https://letsencrypt.org/) - Free TLS certificates
