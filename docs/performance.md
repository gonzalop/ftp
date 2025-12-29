# Performance Tuning Guide

> 📖 **Navigation:** [← Main](../README.md) | [Client →](client.md) | [Server →](server.md)

This guide covers performance optimization techniques for both the FTP client and server implementations.

---

## Table of Contents

- [Client Performance](#client-performance)
  - [Connection Pooling](#connection-pooling)
  - [Parallel Transfers](#parallel-transfers)
  - [Keep-Alive Configuration](#keep-alive-configuration)
  - [Transfer Optimization](#transfer-optimization)
- [Server Performance](#server-performance)
  - [Connection Limits](#connection-limits)
  - [Passive Port Range](#passive-port-range)
  - [Timeout Configuration](#timeout-configuration)
  - [Resource Management](#resource-management)
- [Network Optimization](#network-optimization)
- [Benchmarking](#benchmarking)

---

## Client Performance

### Connection Pooling

While this library doesn't include built-in connection pooling, you can easily implement it yourself. Connection pooling significantly improves performance for applications making frequent FTP operations.

#### Simple Connection Pool

```go
package main

import (
    "sync"
    "github.com/gonzalop/ftp"
)

type ClientPool struct {
    clients chan *ftp.Client
    factory func() (*ftp.Client, error)
    mu      sync.Mutex
    closed  bool
}

func NewClientPool(size int, factory func() (*ftp.Client, error)) (*ClientPool, error) {
    pool := &ClientPool{
        clients: make(chan *ftp.Client, size),
        factory: factory,
    }

    // Pre-populate the pool
    for i := 0; i < size; i++ {
        client, err := factory()
        if err != nil {
            pool.Close()
            return nil, err
        }
        pool.clients <- client
    }

    return pool, nil
}

func (p *ClientPool) Get() (*ftp.Client, error) {
    p.mu.Lock()
    if p.closed {
        p.mu.Unlock()
        return nil, fmt.Errorf("pool is closed")
    }
    p.mu.Unlock()

    select {
    case client := <-p.clients:
        return client, nil
    default:
        // Pool exhausted, create new client
        return p.factory()
    }
}

func (p *ClientPool) Put(client *ftp.Client) {
    p.mu.Lock()
    defer p.mu.Unlock()

    if p.closed {
        client.Quit()
        return
    }

    select {
    case p.clients <- client:
        // Successfully returned to pool
    default:
        // Pool is full, close the client
        client.Quit()
    }
}

func (p *ClientPool) Close() {
    p.mu.Lock()
    defer p.mu.Unlock()

    if p.closed {
        return
    }
    p.closed = true

    close(p.clients)
    for client := range p.clients {
        client.Quit()
    }
}

// Usage example
func main() {
    pool, err := NewClientPool(10, func() (*ftp.Client, error) {
        return ftp.Connect("ftp://user:pass@ftp.example.com")
    })
    if err != nil {
        log.Fatal(err)
    }
    defer pool.Close()

    // Get a client from the pool
    client, err := pool.Get()
    if err != nil {
        log.Fatal(err)
    }
    defer pool.Put(client)

    // Use the client
    entries, err := client.List("/")
    if err != nil {
        log.Fatal(err)
    }

    for _, entry := range entries {
        fmt.Println(entry.Name)
    }
}
```

#### Advanced Pool with Health Checks

```go
type HealthCheckPool struct {
    *ClientPool
    healthCheckInterval time.Duration
    stopHealthCheck     chan struct{}
}

func NewHealthCheckPool(size int, factory func() (*ftp.Client, error),
    healthCheckInterval time.Duration) (*HealthCheckPool, error) {

    basePool, err := NewClientPool(size, factory)
    if err != nil {
        return nil, err
    }

    pool := &HealthCheckPool{
        ClientPool:          basePool,
        healthCheckInterval: healthCheckInterval,
        stopHealthCheck:     make(chan struct{}),
    }

    // Start health check routine
    go pool.healthCheckLoop()

    return pool, nil
}

func (p *HealthCheckPool) healthCheckLoop() {
    ticker := time.NewTicker(p.healthCheckInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            p.performHealthCheck()
        case <-p.stopHealthCheck:
            return
        }
    }
}

func (p *HealthCheckPool) performHealthCheck() {
    // Try to get a client
    client, err := p.Get()
    if err != nil {
        return
    }

    // Test with NOOP command
    if err := client.Noop(); err != nil {
        // Connection is dead, don't return to pool
        client.Quit()

        // Create a replacement
        if newClient, err := p.factory(); err == nil {
            p.Put(newClient)
        }
        return
    }

    // Connection is healthy, return to pool
    p.Put(client)
}

func (p *HealthCheckPool) Close() {
    close(p.stopHealthCheck)
    p.ClientPool.Close()
}
```

**Performance Impact:**
- ✅ Eliminates connection overhead (handshake, TLS negotiation, authentication)
- ✅ Reduces latency by 50-90% for repeated operations
- ✅ Better resource utilization

**When to Use:**
- Web applications serving FTP content
- Batch processing systems
- Microservices with FTP backends
- Any application making frequent FTP operations

---

### Parallel Transfers

For transferring multiple files, use parallel workers with multiple clients.

#### Worker Pool Pattern

```go
package main

import (
    "fmt"
    "log"
    "path/filepath"
    "sync"
    "github.com/gonzalop/ftp"
)

type TransferJob struct {
    LocalPath  string
    RemotePath string
}

type TransferResult struct {
    Job   TransferJob
    Error error
}

func ParallelUpload(jobs []TransferJob, workers int,
    clientFactory func() (*ftp.Client, error)) []TransferResult {

    jobChan := make(chan TransferJob, len(jobs))
    resultChan := make(chan TransferResult, len(jobs))

    var wg sync.WaitGroup

    // Start workers
    for i := 0; i < workers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()

            // Each worker gets its own client
            client, err := clientFactory()
            if err != nil {
                log.Printf("Failed to create client: %v", err)
                return
            }
            defer client.Quit()

            for job := range jobChan {
                err := client.UploadFile(job.LocalPath, job.RemotePath)
                resultChan <- TransferResult{
                    Job:   job,
                    Error: err,
                }
            }
        }()
    }

    // Send jobs
    for _, job := range jobs {
        jobChan <- job
    }
    close(jobChan)

    // Wait for completion
    go func() {
        wg.Wait()
        close(resultChan)
    }()

    // Collect results
    var results []TransferResult
    for result := range resultChan {
        results = append(results, result)
    }

    return results
}

// Usage example
func main() {
    jobs := []TransferJob{
        {LocalPath: "file1.txt", RemotePath: "/upload/file1.txt"},
        {LocalPath: "file2.txt", RemotePath: "/upload/file2.txt"},
        {LocalPath: "file3.txt", RemotePath: "/upload/file3.txt"},
        {LocalPath: "file4.txt", RemotePath: "/upload/file4.txt"},
        {LocalPath: "file5.txt", RemotePath: "/upload/file5.txt"},
    }

    results := ParallelUpload(jobs, 3, func() (*ftp.Client, error) {
        return ftp.Connect("ftp://user:pass@ftp.example.com")
    })

    // Check results
    for _, result := range results {
        if result.Error != nil {
            log.Printf("Failed to upload %s: %v",
                result.Job.LocalPath, result.Error)
        } else {
            log.Printf("Successfully uploaded %s", result.Job.LocalPath)
        }
    }
}
```

#### Parallel Download with Progress

```go
type DownloadProgress struct {
    TotalFiles     int
    CompletedFiles int
    FailedFiles    int
    mu             sync.Mutex
}

func (p *DownloadProgress) Update(success bool) {
    p.mu.Lock()
    defer p.mu.Unlock()

    p.CompletedFiles++
    if !success {
        p.FailedFiles++
    }
}

func (p *DownloadProgress) String() string {
    p.mu.Lock()
    defer p.mu.Unlock()

    return fmt.Sprintf("Progress: %d/%d completed, %d failed",
        p.CompletedFiles, p.TotalFiles, p.FailedFiles)
}

func ParallelDownloadWithProgress(jobs []TransferJob, workers int,
    clientFactory func() (*ftp.Client, error)) {

    progress := &DownloadProgress{TotalFiles: len(jobs)}

    jobChan := make(chan TransferJob, len(jobs))
    var wg sync.WaitGroup

    // Progress reporter
    stopProgress := make(chan struct{})
    go func() {
        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-ticker.C:
                fmt.Println(progress.String())
            case <-stopProgress:
                return
            }
        }
    }()

    // Workers
    for i := 0; i < workers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()

            client, _ := clientFactory()
            defer client.Quit()

            for job := range jobChan {
                err := client.DownloadFile(job.RemotePath, job.LocalPath)
                progress.Update(err == nil)
            }
        }()
    }

    // Send jobs
    for _, job := range jobs {
        jobChan <- job
    }
    close(jobChan)

    wg.Wait()
    close(stopProgress)

    fmt.Println("Final:", progress.String())
}
```

**Performance Impact:**
- ✅ 3-5x faster for many small files
- ✅ Better network utilization
- ✅ Scales with number of workers

**Optimal Worker Count:**
- CPU-bound: Number of CPU cores
- Network-bound: 2-4x number of CPU cores
- Start with 5 workers and tune based on testing

---

### Keep-Alive Configuration

Use `WithIdleTimeout` to prevent connection timeouts during long operations or idle periods.

```go
// For long-running operations
client, err := ftp.Dial("ftp.example.com:21",
    ftp.WithIdleTimeout(5*time.Minute), // Send NOOP every 5 minutes
)

// For applications with sporadic usage
client, err := ftp.Dial("ftp.example.com:21",
    ftp.WithIdleTimeout(2*time.Minute), // More aggressive keep-alive
)
```

**When to Use:**
- Long file transfers (>10 minutes)
- Applications with idle periods between operations
- Servers with aggressive timeout settings

**Trade-offs:**
- ✅ Prevents unexpected disconnections
- ❌ Slight overhead from NOOP commands
- ❌ Keeps connections alive longer (uses server resources)

---

### Transfer Optimization

#### Progress Tracking

Use progress callbacks to monitor transfers without impacting performance:

```go
import "os"

file, _ := os.Open("large-file.bin")
defer file.Close()

// Get file size for progress calculation
info, _ := file.Stat()
totalSize := info.Size()

var transferred int64
pr := &ftp.ProgressReader{
    Reader: file,
    Callback: func(bytesTransferred int64) {
        transferred = bytesTransferred
        percentage := float64(transferred) / float64(totalSize) * 100
        fmt.Printf("\rProgress: %.2f%%", percentage)
    },
}

client.Store("remote-file.bin", pr)
fmt.Println() // New line after progress
```

#### Optimizing Buffer Sizes

The library uses sensible defaults, but you can optimize at the OS level:

```go
import (
    "net"
    "time"
)

// Custom dialer with optimized settings
dialer := &net.Dialer{
    Timeout:   30 * time.Second,
    KeepAlive: 30 * time.Second,
}

client, err := ftp.Dial("ftp.example.com:21",
    ftp.WithDialer(dialer),
)
```

**OS-Level Tuning (Linux):**

```bash
# Increase TCP buffer sizes for large transfers
sysctl -w net.core.rmem_max=16777216
sysctl -w net.core.wmem_max=16777216
sysctl -w net.ipv4.tcp_rmem="4096 87380 16777216"
sysctl -w net.ipv4.tcp_wmem="4096 65536 16777216"
```

---

## Server Performance

### Connection Limits

Configure connection limits to prevent resource exhaustion:

```go
srv, err := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithMaxConnections(100, 10), // Max 100 total, 10 per IP
)
```

**Tuning Guidelines:**

| Server Type | Total Connections | Per-IP Limit |
|-------------|-------------------|--------------|
| Small (1-2 cores) | 50-100 | 5-10 |
| Medium (4-8 cores) | 200-500 | 10-20 |
| Large (16+ cores) | 1000+ | 20-50 |

**Monitoring:**

```go
import "github.com/gonzalop/ftp/server"

type MetricsCollector struct{}

func (m *MetricsCollector) RecordCommand(cmd string, duration time.Duration) {
    // Track command performance
}

func (m *MetricsCollector) RecordTransfer(bytes int64, duration time.Duration, upload bool) {
    // Track transfer metrics
}

func (m *MetricsCollector) RecordConnection(connected bool) {
    // Track connection count
}

func (m *MetricsCollector) RecordAuth(success bool, user string) {
    // Track authentication attempts
}

srv, _ := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithMetricsCollector(&MetricsCollector{}),
)
```

---

### Passive Port Range

Configure a specific port range for passive mode to simplify firewall rules:

```go
srv, err := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithPasvMinPort(50000),
    server.WithPasvMaxPort(51000),
)
```

**Port Range Sizing:**

```
Required Ports = Max Concurrent Transfers × 2
```

Example:
- 100 concurrent transfers → 200 ports
- Range: 50000-50200

**Firewall Configuration:**

```bash
# iptables
iptables -A INPUT -p tcp --dport 21 -j ACCEPT
iptables -A INPUT -p tcp --dport 50000:51000 -j ACCEPT

# firewalld
firewall-cmd --permanent --add-port=21/tcp
firewall-cmd --permanent --add-port=50000-51000/tcp
firewall-cmd --reload
```

---

### Timeout Configuration

Configure timeouts to prevent slow clients from consuming resources:

```go
srv, err := server.NewServer(":21",
    server.WithDriver(driver),
    server.WithReadTimeout(30*time.Second),   // Prevent slow-read attacks
    server.WithWriteTimeout(30*time.Second),  // Prevent slow-write attacks
    server.WithMaxIdleTime(10*time.Minute),   // Disconnect idle clients
)
```

**Timeout Guidelines:**

| Operation | Recommended Timeout |
|-----------|---------------------|
| Read | 30-60 seconds |
| Write | 30-60 seconds |
| Idle | 5-15 minutes |

---

### Resource Management

#### Graceful Shutdown

Implement graceful shutdown to avoid interrupting active transfers:

```go
import (
    "context"
    "os"
    "os/signal"
    "syscall"
)

func main() {
    driver, _ := server.NewFSDriver("/var/ftp")
    srv, _ := server.NewServer(":21", server.WithDriver(driver))

    // Handle shutdown signals
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    go func() {
        <-sigChan
        log.Println("Shutting down gracefully...")

        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        if err := srv.Shutdown(ctx); err != nil {
            log.Printf("Shutdown error: %v", err)
        }
    }()

    log.Println("Starting FTP server on :21")
    if err := srv.ListenAndServe(); err != nil && err != server.ErrServerClosed {
        log.Fatal(err)
    }
}
```

#### Memory Management

For high-throughput servers, monitor and optimize memory usage:

```go
import "runtime"

// Periodically trigger GC for long-running servers
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        runtime.GC()
    }
}()
```

---

## Network Optimization

### TCP Tuning

#### Client-Side

```go
dialer := &net.Dialer{
    Timeout:   30 * time.Second,
    KeepAlive: 30 * time.Second,
}

client, err := ftp.Dial("ftp.example.com:21",
    ftp.WithDialer(dialer),
)
```

#### Server-Side (OS Level)

**Linux:**

```bash
# Enable TCP window scaling
sysctl -w net.ipv4.tcp_window_scaling=1

# Increase max TCP buffer sizes
sysctl -w net.core.rmem_max=16777216
sysctl -w net.core.wmem_max=16777216

# Tune TCP buffer sizes
sysctl -w net.ipv4.tcp_rmem="4096 87380 16777216"
sysctl -w net.ipv4.tcp_wmem="4096 65536 16777216"

# Enable TCP fast open
sysctl -w net.ipv4.tcp_fastopen=3

# Increase connection backlog
sysctl -w net.core.somaxconn=4096
```

### Latency vs Throughput

**Low Latency (Many Small Files):**
- Use parallel transfers (5-10 workers)
- Connection pooling
- Smaller timeout values

**High Throughput (Large Files):**
- Single connection per file
- Larger TCP buffers
- Longer timeout values
- Progress tracking

---

## Benchmarking

### Client Benchmarks

```go
package main

import (
    "testing"
    "github.com/gonzalop/ftp"
)

func BenchmarkList(b *testing.B) {
    client, _ := ftp.Connect("ftp://user:pass@localhost")
    defer client.Quit()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = client.List("/")
    }
}

func BenchmarkListWithPool(b *testing.B) {
    pool, _ := NewClientPool(10, func() (*ftp.Client, error) {
        return ftp.Connect("ftp://user:pass@localhost")
    })
    defer pool.Close()

    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            client, _ := pool.Get()
            _, _ = client.List("/")
            pool.Put(client)
        }
    })
}

func BenchmarkUpload(b *testing.B) {
    client, _ := ftp.Connect("ftp://user:pass@localhost")
    defer client.Quit()

    data := make([]byte, 1024*1024) // 1 MB

    b.ResetTimer()
    b.SetBytes(int64(len(data)))

    for i := 0; i < b.N; i++ {
        reader := bytes.NewReader(data)
        _ = client.Store(fmt.Sprintf("test_%d.bin", i), reader)
    }
}
```

**Run benchmarks:**

```bash
go test -bench=. -benchmem -benchtime=10s
```

### Performance Metrics

Track these metrics for optimization:

**Client:**
- Connection establishment time
- Authentication time
- Command latency
- Transfer throughput (MB/s)
- Operations per second

**Server:**
- Active connections
- Connections per second
- Transfer throughput
- Command processing time
- Memory usage
- CPU usage

---

## Best Practices Summary

### Client

✅ **DO:**
- Use connection pooling for frequent operations
- Implement parallel transfers for multiple files
- Configure keep-alive for long operations
- Monitor transfer progress
- Handle errors and retry transient failures

❌ **DON'T:**
- Create new connections for every operation
- Transfer files sequentially when parallel is possible
- Ignore timeout configuration
- Skip error handling

### Server

✅ **DO:**
- Set appropriate connection limits
- Configure passive port range for firewalls
- Implement graceful shutdown
- Monitor metrics and performance
- Tune timeouts based on use case

❌ **DON'T:**
- Allow unlimited connections
- Use random ports in production
- Skip timeout configuration
- Ignore resource monitoring

---

## Additional Resources

- [Client Documentation](client.md)
- [Server Documentation](server.md)
- [Security Best Practices](security.md)
- [Contributing Guidelines](../CONTRIBUTING.md)
