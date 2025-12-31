package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/gonzalop/ftp/examples/quic/common"
	"github.com/gonzalop/ftp/server"
	"github.com/quic-go/quic-go"
)

var (
	addr     = flag.String("addr", ":4242", "Server address to listen on")
	rootDir  = flag.String("root", "./quic-ftp-files", "Root directory for file storage")
	certFile = flag.String("cert", "", "TLS certificate file (auto-generated if not provided)")
	keyFile  = flag.String("key", "", "TLS key file (auto-generated if not provided)")
)

func main() {
	flag.Parse()

	// Create root directory if it doesn't exist
	if err := os.MkdirAll(*rootDir, 0755); err != nil {
		log.Fatalf("Failed to create root directory: %v", err)
	}

	// Setup TLS configuration
	tlsConfig, err := setupTLS()
	if err != nil {
		log.Fatalf("Failed to setup TLS: %v", err)
	}

	// Configure QUIC
	quicConfig := &quic.Config{
		MaxIncomingStreams: 100,
		KeepAlivePeriod:    30 * time.Second,
	}

	// Start QUIC listener
	listener, err := quic.ListenAddr(*addr, tlsConfig, quicConfig)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	log.Printf("QUIC-FTP server listening on %s", *addr)
	log.Printf("Root directory: %s", *rootDir)

	log.Println("Accepting QUIC connections...")

	// Accept QUIC connections and handle them
	for {
		conn, err := listener.Accept(context.Background())
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		log.Printf("New QUIC connection from %s", conn.RemoteAddr())
		go handleQUICConnection(conn)
	}
}

// QuicListenerFactory implements server.ListenerFactory for QUIC streams
type QuicListenerFactory struct {
	listener *quic.Listener
}

func (f *QuicListenerFactory) Listen(network, address string) (net.Listener, error) {
	// Return a stream listener that accepts QUIC streams as connections
	return &QuicStreamListener{}, nil
}

// QuicDataListenerFactory creates listeners for PASV data connections using QUIC streams
type QuicDataListenerFactory struct {
	quicConn *quic.Conn
}

func (f *QuicDataListenerFactory) Listen(network, address string) (net.Listener, error) {
	// For data connections, we return a listener that will accept the next QUIC stream
	return &QuicDataListener{quicConn: f.quicConn}, nil
}

// QuicDataListener accepts a single QUIC stream as a data connection
type QuicDataListener struct {
	quicConn *quic.Conn
	accepted bool
}

func (l *QuicDataListener) Accept() (net.Conn, error) {
	log.Printf("QuicDataListener.Accept() called, accepted=%v", l.accepted)
	if l.accepted {
		// Only accept one connection per listener
		return nil, fmt.Errorf("listener already accepted connection")
	}
	l.accepted = true

	// Accept the next QUIC stream from the client
	log.Println("Waiting to accept data stream from client...")
	ctx := context.Background()
	stream, err := l.quicConn.AcceptStream(ctx)
	if err != nil {
		log.Printf("Failed to accept data stream: %v", err)
		return nil, err
	}

	log.Printf("Accepted data stream %d", stream.StreamID())

	// IMPORTANT: Client sends an initialization byte to make the stream visible.
	// Read and discard it before returning the connection.
	log.Println("Reading initialization byte from data stream...")
	initByte := make([]byte, 1)
	if _, err := stream.Read(initByte); err != nil {
		log.Printf("Failed to read initialization byte: %v", err)
		return nil, err
	}
	log.Println("Data stream initialized")

	return common.NewQuicConn(stream, l.quicConn), nil
}

func (l *QuicDataListener) Close() error {
	return nil
}

func (l *QuicDataListener) Addr() net.Addr {
	return l.quicConn.LocalAddr()
}

// QuicStreamListener implements net.Listener for QUIC streams
type QuicStreamListener struct {
	streamChan chan net.Conn
	closed     bool
}

func (l *QuicStreamListener) Accept() (net.Conn, error) {
	if l.streamChan == nil {
		// Block forever - streams are provided externally
		select {}
	}
	conn, ok := <-l.streamChan
	if !ok {
		return nil, fmt.Errorf("listener closed")
	}
	return conn, nil
}

func (l *QuicStreamListener) Close() error {
	l.closed = true
	if l.streamChan != nil {
		close(l.streamChan)
	}
	return nil
}

func (l *QuicStreamListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

func handleQUICConnection(conn *quic.Conn) {
	defer conn.CloseWithError(0, "connection closed")

	log.Printf("handleQUICConnection started for %s", conn.RemoteAddr())

	// Accept the control stream (stream 0)
	ctx := context.Background()
	log.Printf("Waiting to accept control stream...")
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		log.Printf("Failed to accept control stream: %v", err)
		return
	}

	log.Printf("Accepted stream %d", stream.StreamID())

	if stream.StreamID() != 0 {
		log.Printf("First stream is not stream 0 (got %d), closing connection", stream.StreamID())
		stream.Close()
		return
	}

	log.Printf("Control stream established (stream %d)", stream.StreamID())

	// Wrap the stream as net.Conn
	controlConn := common.NewQuicConn(stream, conn)

	// IMPORTANT: The client sends a NOOP command to make the stream visible
	// to AcceptStream(). The FTP server will handle it normally.
	log.Println("Stream initialized, starting FTP session...")

	log.Printf("Handling FTP session for %s", conn.RemoteAddr())

	// We need to handle the FTP session ourselves since the server's internal
	// session handler isn't exported. For now, create a new server instance
	// for each connection with a custom listener factory that provides data streams.

	// Create a driver for this session
	sessionDriver, err := server.NewFSDriver(*rootDir,
		server.WithAuthenticator(func(user, pass, host string, remoteIP net.IP) (string, bool, error) {
			if user == "user" && pass == "pass" {
				return *rootDir, false, nil
			}
			if user == "anonymous" || user == "ftp" {
				return *rootDir, true, nil
			}
			return "", false, os.ErrPermission
		}),
	)
	if err != nil {
		log.Printf("Failed to create session driver: %v", err)
		return
	}

	// Create a listener factory that provides QUIC streams for data connections
	dataListenerFactory := &QuicDataListenerFactory{quicConn: conn}

	// Create a server for this session
	sessionServer, err := server.NewServer(":0",
		server.WithDriver(sessionDriver),
		server.WithListenerFactory(dataListenerFactory),
		server.WithDisableCommands(server.ActiveModeCommands...),
	)
	if err != nil {
		log.Printf("Failed to create session server: %v", err)
		return
	}

	// Create a single-connection listener
	singleListener := &singleConnListener{
		conn: controlConn,
		addr: controlConn.LocalAddr(),
	}

	// Serve this single connection
	log.Printf("Starting FTP session...")
	if err := sessionServer.Serve(singleListener); err != nil {
		log.Printf("FTP session ended: %v", err)
	}
}

// singleConnListener returns a single pre-established connection
type singleConnListener struct {
	conn net.Conn
	addr net.Addr
	once bool
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if l.once {
		// Block forever after returning the connection once
		select {}
	}
	l.once = true
	return l.conn, nil
}

func (l *singleConnListener) Close() error {
	return nil
}

func (l *singleConnListener) Addr() net.Addr {
	return l.addr
}

func setupTLS() (*tls.Config, error) {
	// If cert and key files are provided, use them
	if *certFile != "" && *keyFile != "" {
		cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate: %w", err)
		}
		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"ftp-over-quic"},
		}, nil
	}

	// Generate self-signed certificate
	log.Println("Generating self-signed certificate for testing...")
	cert, err := generateSelfSignedCert()
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"ftp-over-quic"},
	}, nil
}

func generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"QUIC-FTP Server"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return tls.X509KeyPair(certPEM, keyPEM)
}
