package server

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

func (s *session) handleRETR(path string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	file, err := s.fs.OpenFile(path, os.O_RDONLY)
	if err != nil {
		s.replyError(err)
		return
	}
	defer file.Close()

	if s.restartOffset > 0 {
		if seeker, ok := file.(io.Seeker); ok {
			_, err := seeker.Seek(s.restartOffset, io.SeekStart)
			if err != nil {
				s.replyError(err)
				return
			}
		} else {
			s.reply(550, "Resume not supported for this file.")
			s.restartOffset = 0
			return
		}
	}

	conn, err := s.connData()
	if err != nil {
		s.reply(425, "Can't open data connection.")
		return
	}
	defer conn.Close()

	if s.restartOffset > 0 {
		s.reply(150, fmt.Sprintf("Opening data connection for RETR (restarting at %d).", s.restartOffset))
	} else {
		s.reply(150, "Opening data connection for RETR.")
	}

	// Reset offset after use
	s.restartOffset = 0

	// Track transfer metrics
	startTime := time.Now()

	var src io.Reader = file
	if s.transferType == "A" {
		src = newASCIIReader(file)
	}

	bytesTransferred, err := io.Copy(conn, src)
	if err != nil {
		s.reply(426, "Connection closed; transfer aborted.")
		return
	}
	duration := time.Since(startTime)

	// Calculate throughput in MB/s
	throughputMBps := float64(0)
	if duration.Seconds() > 0 {
		throughputMBps = float64(bytesTransferred) / duration.Seconds() / 1024 / 1024
	}

	// Transfer logging
	s.server.logger.Info("transfer_complete",
		"session_id", s.sessionID,
		"remote_ip", s.redactIP(s.remoteIP),
		"user", s.user,
		"host", s.host,
		"operation", "RETR",
		"path", s.redactPath(path),
		"bytes", bytesTransferred,
		"duration_ms", duration.Milliseconds(),
		"throughput_mbps", fmt.Sprintf("%.2f", throughputMBps),
	)

	// Metrics collection
	if s.server.metricsCollector != nil {
		s.server.metricsCollector.RecordTransfer("RETR", bytesTransferred, duration)
	}

	s.reply(226, "Transfer complete.")
}

func (s *session) handleSTOR(path string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	// Determine flags based on restart
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if s.restartOffset > 0 {
		flags = os.O_WRONLY | os.O_CREATE
	}

	file, err := s.fs.OpenFile(path, flags)
	if err != nil {
		s.replyError(err)
		return
	}
	defer file.Close()

	if s.restartOffset > 0 {
		if seeker, ok := file.(io.Seeker); ok {
			_, err := seeker.Seek(s.restartOffset, io.SeekStart)
			if err != nil {
				s.replyError(err)
				return
			}
		} else {
			s.reply(550, "Resume not supported for this file.")
			s.restartOffset = 0
			return
		}
	}

	conn, err := s.connData()
	if err != nil {
		s.reply(425, "Can't open data connection.")
		return
	}
	defer conn.Close()

	s.reply(150, "Opening data connection for STOR.")

	// Track transfer metrics
	startTime := time.Now()

	var src io.Reader = conn
	if s.transferType == "A" {
		src = newASCIIWriter(conn)
	}

	bytesTransferred, err := io.Copy(file, src)
	if err != nil {
		s.reply(426, "Connection closed; transfer aborted.")
		return
	}
	duration := time.Since(startTime)

	// Calculate throughput in MB/s
	throughputMBps := float64(0)
	if duration.Seconds() > 0 {
		throughputMBps = float64(bytesTransferred) / duration.Seconds() / 1024 / 1024
	}

	// Transfer logging
	s.server.logger.Info("transfer_complete",
		"session_id", s.sessionID,
		"remote_ip", s.redactIP(s.remoteIP),
		"user", s.user,
		"host", s.host,
		"operation", "STOR",
		"path", s.redactPath(path),
		"bytes", bytesTransferred,
		"duration_ms", duration.Milliseconds(),
		"throughput_mbps", fmt.Sprintf("%.2f", throughputMBps),
	)

	// Metrics collection
	if s.server.metricsCollector != nil {
		s.server.metricsCollector.RecordTransfer("STOR", bytesTransferred, duration)
	}

	s.restartOffset = 0
	s.reply(226, "Transfer complete.")
}

func (s *session) handleAPPE(path string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	file, err := s.fs.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE)
	if err != nil {
		s.replyError(err)
		return
	}
	defer file.Close()

	conn, err := s.connData()
	if err != nil {
		s.reply(425, "Can't open data connection.")
		return
	}
	defer conn.Close()

	s.reply(150, "Opening data connection for APPE.")

	var src io.Reader = conn
	if s.transferType == "A" {
		src = newASCIIWriter(conn)
	}

	if _, err := io.Copy(file, src); err != nil {
		s.reply(426, "Connection closed; transfer aborted.")
		return
	}

	s.reply(226, "Transfer complete.")
}

func (s *session) handleSTOU() {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	uuid := fmt.Sprintf("ftp-%d", time.Now().UnixNano())
	path := uuid

	file, err := s.fs.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		s.replyError(err)
		return
	}
	defer file.Close()

	conn, err := s.connData()
	if err != nil {
		s.reply(425, "Can't open data connection.")
		return
	}
	defer conn.Close()

	s.reply(150, fmt.Sprintf("FILE: %s", path))

	var src io.Reader = conn
	if s.transferType == "A" {
		src = newASCIIWriter(conn)
	}

	if _, err := io.Copy(file, src); err != nil {
		s.reply(426, "Connection closed; transfer aborted.")
		return
	}

	s.reply(226, "Transfer complete.")
}

func (s *session) handleTYPE(arg string) {
	if !s.isLoggedIn {
		s.reply(530, "Please login with USER and PASS.")
		return
	}
	// Only support ASCII (A) and Binary (I). Fail if EBCDIC (E).
	switch strings.ToUpper(arg) {
	case "A", "A N":
		s.transferType = "A"
		s.reply(200, "Type set to A.")
	case "I", "L 8":
		s.transferType = "I"
		s.reply(200, "Type set to I.")
	default:
		s.reply(504, "Type not supported.")
	}
}

func (s *session) handlePORT(arg string) {
	if !s.isLoggedIn {
		s.reply(530, "Please login with USER and PASS.")
		return
	}

	// Format: h1,h2,h3,h4,p1,p2
	parts := strings.Split(arg, ",")
	if len(parts) != 6 {
		s.reply(501, "Syntax error in parameters or arguments.")
		return
	}

	p1, err1 := strconv.Atoi(parts[4])
	p2, err2 := strconv.Atoi(parts[5])
	if err1 != nil || err2 != nil || p1 < 0 || p1 > 255 || p2 < 0 || p2 > 255 {
		s.reply(501, "Invalid port number.")
		return
	}

	ipStr := strings.Join(parts[0:4], ".")
	ip := net.ParseIP(ipStr)
	if ip == nil {
		s.reply(501, "Invalid IP address.")
		return
	}

	if !s.validateActiveIP(ip) {
		s.reply(500, "Illegal PORT command.")
		return
	}

	s.activeIP = ip.String()
	s.activePort = p1*256 + p2

	s.reply(200, "PORT command successful.")
}

func (s *session) listenPassive() (net.Listener, error) {
	settings := s.fs.GetSettings()
	if settings != nil && settings.PasvMinPort > 0 && settings.PasvMaxPort >= settings.PasvMinPort {
		minPort := settings.PasvMinPort
		maxPort := settings.PasvMaxPort
		rangeLen := int32(maxPort - minPort + 1)

		// Get a starting offset using round-robin
		startOffset := atomic.AddInt32(&s.server.nextPassivePort, 1)

		for i := int32(0); i < rangeLen; i++ {
			offset := (startOffset + i) % rangeLen
			port := int(int32(minPort) + offset)

			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err == nil {
				return ln, nil
			}
		}
		return nil, fmt.Errorf("no available ports in range [%d, %d]", minPort, maxPort)
	}
	return net.Listen("tcp", ":0")
}

func (s *session) handlePASV() {
	if !s.isLoggedIn {
		s.reply(530, "Please login with USER and PASS.")
		return
	}

	if s.pasvList != nil {
		s.pasvList.Close()
	}

	ln, err := s.listenPassive()
	if err != nil {
		s.reply(425, "Can't open passive connection.")
		return
	}
	s.pasvList = ln

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	// Determine IP to send
	// 1. Get local connection IP
	host, _, _ := net.SplitHostPort(s.conn.LocalAddr().String())

	// 2. Override with PublicHost if set
	settings := s.fs.GetSettings()
	if settings != nil && settings.PublicHost != "" {
		host = settings.PublicHost
	}

	// 3. Resolve to IPv4
	ip := net.ParseIP(host)
	if ip == nil {
		// Use cached resolution if available
		if host == s.lastPublicHost && s.resolvedIP != nil {
			ip = s.resolvedIP
		} else {
			// Try resolving hostname
			fileArgs, err := net.LookupIP(host)
			if err == nil {
				for _, resolvedIP := range fileArgs {
					if ipv4 := resolvedIP.To4(); ipv4 != nil {
						ip = ipv4
						s.lastPublicHost = host
						s.resolvedIP = ip
						break
					}
				}
			}
		}
	}

	// 4. Format for PASV response (h1,h2,h3,h4)
	var ipParts []string
	if ip != nil && ip.To4() != nil {
		ip = ip.To4()
		ipParts = strings.Split(ip.String(), ".")
	}

	if len(ipParts) != 4 {
		// Fallback for non-IPv4 or failed resolution
		ipParts = []string{"0", "0", "0", "0"}
	}

	p1 := port / 256
	p2 := port % 256
	arg := fmt.Sprintf("%s,%s,%s,%s,%d,%d", ipParts[0], ipParts[1], ipParts[2], ipParts[3], p1, p2)
	s.reply(227, "Entering Passive Mode ("+arg+").")
}

func (s *session) handleEPSV() {
	if !s.isLoggedIn {
		s.reply(530, "Please login with USER and PASS.")
		return
	}

	if s.pasvList != nil {
		s.pasvList.Close()
	}

	ln, err := s.listenPassive()
	if err != nil {
		s.reply(425, "Can't open passive connection.")
		return
	}
	s.pasvList = ln

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	s.reply(229, fmt.Sprintf("Entering Extended Passive Mode (|||%s|)", portStr))
}

func (s *session) handleEPRT(arg string) {
	if !s.isLoggedIn {
		s.reply(530, "Please login with USER and PASS.")
		return
	}

	if len(arg) < 4 {
		s.reply(501, "Syntax error in parameters or arguments.")
		return
	}

	delim := string(arg[0])
	parts := strings.Split(arg, delim)

	// Expected format: <delim><proto><delim><ip><delim><port><delim>
	// Split results in: ["", "proto", "ip", "port", ""]
	if len(parts) != 5 {
		s.reply(501, "Syntax error in parameters or arguments.")
		return
	}

	// Protocol: 1 = IPv4, 2 = IPv6
	proto := parts[1]
	ipStr := parts[2]
	portStr := parts[3]

	// Validate IP
	ip := net.ParseIP(ipStr)
	if ip == nil {
		s.reply(501, "Invalid network address.")
		return
	}

	// Validate Protocol vs IP type
	if proto == "1" && ip.To4() == nil {
		s.reply(522, "Network protocol not supported, use (2).")
		return
	}
	// if proto == "2" && ip.To4() != nil {
	// 	// Strictly speaking, IPv4-mapped IPv6 is valid in Go, but RFC implies 2 is for IPv6.
	// 	// We'll accept it but verify parsing.
	// }
	if proto != "1" && proto != "2" {
		s.reply(522, "Network protocol not supported, use (1,2).")
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		s.reply(501, "Invalid port number.")
		return
	}

	if !s.validateActiveIP(ip) {
		s.reply(500, "Illegal EPRT command.")
		return
	}

	s.activeIP = ip.String()
	s.activePort = port

	s.reply(200, "EPRT command successful.")
}

func (s *session) handleREST(arg string) {
	offset, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		s.reply(501, "Invalid offset.")
		return
	}
	s.restartOffset = offset
	s.reply(350, fmt.Sprintf("Restarting at %d. Send STOR or RETR to initiate transfer.", offset))
}
