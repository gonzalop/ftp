package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"

	"github.com/gonzalop/ftp/examples/quic/common"
	"github.com/quic-go/quic-go"
)

var (
	serverAddr = flag.String("server", "localhost:4242", "Server address to connect to")
	username   = flag.String("user", "anonymous", "Username for authentication")
	password   = flag.String("pass", "anonymous@", "Password for authentication")
)

type QuicFTPClient struct {
	quicConn    *quic.Conn
	controlConn net.Conn
	reader      *bufio.Reader
	writer      *bufio.Writer
}

func main() {
	flag.Parse()

	client, err := connectQUIC(*serverAddr, *username, *password)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	fmt.Println("Connected to QUIC-FTP server")
	fmt.Println("Type 'help' for available commands, 'quit' to exit")

	// Interactive command loop
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("ftp> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if err := client.handleCommand(line); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}
}

func connectQUIC(addr, user, pass string) (*QuicFTPClient, error) {
	log.Printf("Connecting to %s...", addr)

	// Setup TLS configuration
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"ftp-over-quic"},
	}

	// Setup QUIC configuration
	quicConfig := &quic.Config{
		MaxIncomingStreams: 100,
	}

	// Establish QUIC connection
	ctx := context.Background()
	quicConn, err := quic.DialAddr(ctx, addr, tlsConfig, quicConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial QUIC: %w", err)
	}

	// Open control stream
	log.Println("Opening control stream...")
	controlStream, err := quicConn.OpenStreamSync(ctx)
	if err != nil {
		quicConn.CloseWithError(0, "failed to open control stream")
		return nil, fmt.Errorf("failed to open control stream: %w", err)
	}

	log.Printf("Control stream opened (ID: %d)", controlStream.StreamID())

	// Wrap as net.Conn
	controlConn := common.NewQuicConn(controlStream, quicConn)

	client := &QuicFTPClient{
		quicConn:    quicConn,
		controlConn: controlConn,
		reader:      bufio.NewReader(controlConn),
		writer:      bufio.NewWriter(controlConn),
	}

	// IMPORTANT: In QUIC, the server's AcceptStream() won't return until
	// the client sends data on the stream. But FTP expects the server to
	// send the welcome message first. So we send a NOOP command to make the
	// stream visible, which the server will handle before sending the welcome.
	log.Println("Sending NOOP to initialize stream...")
	if _, err := controlStream.Write([]byte("NOOP\r\n")); err != nil {
		return nil, fmt.Errorf("failed to initialize stream: %w", err)
	}

	log.Println("Waiting for welcome message...")

	// Read NOOP response (200 or similar)
	if _, _, err := client.readResponse(); err != nil {
		return nil, fmt.Errorf("failed to read NOOP response: %w", err)
	}

	// Read welcome message
	if _, _, err := client.readResponse(); err != nil {
		return nil, fmt.Errorf("failed to read welcome: %w", err)
	}

	// Login
	if err := client.login(user, pass); err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}

	log.Println("Logged in successfully")
	return client, nil
}

func (c *QuicFTPClient) login(user, pass string) error {
	// Send USER
	if err := c.sendCommand("USER " + user); err != nil {
		return err
	}
	code, _, err := c.readResponse()
	if err != nil {
		return err
	}
	if code != 331 && code != 230 {
		return fmt.Errorf("USER failed with code %d", code)
	}

	// Send PASS if needed
	if code == 331 {
		if err := c.sendCommand("PASS " + pass); err != nil {
			return err
		}
		code, _, err = c.readResponse()
		if err != nil {
			return err
		}
		if code != 230 {
			return fmt.Errorf("PASS failed with code %d", code)
		}
	}

	return nil
}

func (c *QuicFTPClient) handleCommand(line string) error {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}

	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "help":
		c.printHelp()
	case "quit", "exit":
		fmt.Println("Goodbye!")
		os.Exit(0)
	case "pwd":
		return c.pwd()
	case "ls", "list", "dir":
		return c.list()
	case "cd":
		if len(parts) < 2 {
			return fmt.Errorf("usage: cd <directory>")
		}
		return c.cwd(parts[1])
	case "get":
		if len(parts) < 2 {
			return fmt.Errorf("usage: get <remote-file> [local-file]")
		}
		localFile := parts[1]
		if len(parts) > 2 {
			localFile = parts[2]
		}
		return c.get(parts[1], localFile)
	case "put":
		if len(parts) < 2 {
			return fmt.Errorf("usage: put <local-file> [remote-file]")
		}
		remoteFile := parts[1]
		if len(parts) > 2 {
			remoteFile = parts[2]
		}
		return c.put(parts[1], remoteFile)
	default:
		// Send raw command
		return c.sendRawCommand(line)
	}

	return nil
}

func (c *QuicFTPClient) pwd() error {
	if err := c.sendCommand("PWD"); err != nil {
		return err
	}
	_, msg, err := c.readResponse()
	if err != nil {
		return err
	}
	fmt.Println(msg)
	return nil
}

func (c *QuicFTPClient) list() error {
	// Enter passive mode
	dataConn, err := c.pasv()
	if err != nil {
		return err
	}
	defer dataConn.Close()

	// Send LIST command
	if err := c.sendCommand("LIST"); err != nil {
		return err
	}
	if _, _, err := c.readResponse(); err != nil {
		return err
	}

	// Read listing
	data, err := io.ReadAll(dataConn)
	if err != nil {
		return err
	}

	fmt.Print(string(data))

	// Read completion response
	_, _, err = c.readResponse()
	return err
}

func (c *QuicFTPClient) cwd(dir string) error {
	if err := c.sendCommand("CWD " + dir); err != nil {
		return err
	}
	_, msg, err := c.readResponse()
	if err != nil {
		return err
	}
	fmt.Println(msg)
	return nil
}

func (c *QuicFTPClient) get(remote, local string) error {
	// Enter passive mode
	dataConn, err := c.pasv()
	if err != nil {
		return err
	}
	defer dataConn.Close()

	// Send RETR command
	if err := c.sendCommand("RETR " + remote); err != nil {
		return err
	}
	if _, _, err := c.readResponse(); err != nil {
		return err
	}

	// Create local file
	f, err := os.Create(local)
	if err != nil {
		return err
	}
	defer f.Close()

	// Download
	n, err := io.Copy(f, dataConn)
	if err != nil {
		return err
	}

	fmt.Printf("Downloaded %d bytes to %s\n", n, local)

	// Read completion response
	_, _, err = c.readResponse()
	return err
}

func (c *QuicFTPClient) put(local, remote string) error {
	// Open local file
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()

	// Enter passive mode
	dataConn, err := c.pasv()
	if err != nil {
		return err
	}
	defer dataConn.Close()

	// Send STOR command
	if err := c.sendCommand("STOR " + remote); err != nil {
		return err
	}
	if _, _, err := c.readResponse(); err != nil {
		return err
	}

	// Upload
	n, err := io.Copy(dataConn, f)
	if err != nil {
		return err
	}

	dataConn.Close() // Close to signal EOF

	fmt.Printf("Uploaded %d bytes to %s\n", n, remote)

	// Read completion response
	_, _, err = c.readResponse()
	return err
}

func (c *QuicFTPClient) pasv() (net.Conn, error) {
	// Send PASV command (server will open a QUIC stream)
	if err := c.sendCommand("PASV"); err != nil {
		return nil, err
	}

	code, msg, err := c.readResponse()
	if err != nil {
		return nil, err
	}

	if code != 227 {
		return nil, fmt.Errorf("PASV failed: %s", msg)
	}

	// For QUIC, the server will accept a new stream for data connection
	// Open a new QUIC stream
	log.Println("Opening data stream for PASV...")
	ctx := context.Background()
	stream, err := c.quicConn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open data stream: %w", err)
	}

	log.Printf("Data stream opened (ID: %d)", stream.StreamID())

	// IMPORTANT: Same as control stream - send a byte to make stream visible to server's AcceptStream()
	log.Println("Sending initialization byte on data stream...")
	if _, err := stream.Write([]byte{0}); err != nil {
		return nil, fmt.Errorf("failed to initialize data stream: %w", err)
	}

	return common.NewQuicConn(stream, c.quicConn), nil
}

func (c *QuicFTPClient) sendCommand(cmd string) error {
	_, err := fmt.Fprintf(c.writer, "%s\r\n", cmd)
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

func (c *QuicFTPClient) sendRawCommand(cmd string) error {
	if err := c.sendCommand(cmd); err != nil {
		return err
	}
	_, msg, err := c.readResponse()
	if err != nil {
		return err
	}
	fmt.Println(msg)
	return nil
}

func (c *QuicFTPClient) readResponse() (int, string, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return 0, "", err
	}

	line = strings.TrimSpace(line)
	fmt.Println("<-", line)

	var code int
	if _, err := fmt.Sscanf(line, "%d", &code); err != nil {
		return 0, line, nil
	}

	// Handle multi-line responses
	if len(line) > 3 && line[3] == '-' {
		for {
			nextLine, err := c.reader.ReadString('\n')
			if err != nil {
				return code, line, err
			}
			nextLine = strings.TrimSpace(nextLine)
			fmt.Println("<-", nextLine)
			line += "\n" + nextLine

			// Check if this is the last line
			if len(nextLine) > 3 && nextLine[3] == ' ' {
				var endCode int
				fmt.Sscanf(nextLine, "%d", &endCode)
				if endCode == code {
					break
				}
			}
		}
	}

	return code, line, nil
}

func (c *QuicFTPClient) Close() {
	c.sendCommand("QUIT")
	c.quicConn.CloseWithError(0, "client closing")
}

func (c *QuicFTPClient) printHelp() {
	fmt.Println("Available commands:")
	fmt.Println("  help           - Show this help message")
	fmt.Println("  pwd            - Print working directory")
	fmt.Println("  ls, list, dir  - List files in current directory")
	fmt.Println("  cd <dir>       - Change directory")
	fmt.Println("  get <file>     - Download file")
	fmt.Println("  put <file>     - Upload file")
	fmt.Println("  quit, exit     - Disconnect and exit")
	fmt.Println()
	fmt.Println("You can also send raw FTP commands directly.")
}
