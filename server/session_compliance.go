package server

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// handleACCT handles the ACCT command.
// RFC 1123 requires this command, but most modern servers don't need it.
func (s *session) handleACCT(arg string) {
	s.reply(202, "Command not implemented, superfluous at this site.")
}

// handleMODE handles the MODE command.
// RFC 1123 requires Stream mode support.
func (s *session) handleMODE(arg string) {
	mode := strings.ToUpper(strings.TrimSpace(arg))
	switch mode {
	case "S":
		// Stream mode (default and only supported mode)
		s.reply(200, "Mode set to Stream.")
	case "B":
		s.reply(504, "Block mode not implemented.")
	case "C":
		s.reply(504, "Compressed mode not implemented.")
	default:
		s.reply(504, "Command not implemented for that parameter.")
	}
}

// handleSTRU handles the STRU command.
// RFC 1123 requires File structure support.
func (s *session) handleSTRU(arg string) {
	stru := strings.ToUpper(strings.TrimSpace(arg))
	switch stru {
	case "F":
		// File structure (default and only supported structure)
		s.reply(200, "Structure set to File.")
	case "R":
		s.reply(504, "Record structure not implemented.")
	case "P":
		s.reply(504, "Page structure not implemented.")
	default:
		s.reply(504, "Command not implemented for that parameter.")
	}
}

// handleSYST handles the SYST command.
// Returns the system type, dynamically detected based on runtime.GOOS.
func (s *session) handleSYST() {
	var systType string
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "openbsd", "netbsd", "dragonfly", "solaris", "illumos", "aix":
		systType = "UNIX Type: L8"
	case "windows":
		systType = "Windows_NT"
	case "plan9":
		systType = "Plan9"
	default:
		systType = "UNKNOWN Type: L8"
	}
	s.reply(215, systType)
}

// handleSTAT handles the STAT command.
// Returns connection status information.
func (s *session) handleSTAT(arg string) {
	if arg != "" {
		// STAT with path argument - list directory (like LIST but over control connection)
		// This is optional and complex, so we'll just reject it for now
		s.reply(502, "STAT with path not implemented. Use LIST instead.")
		return
	}

	// Return connection status using multi-line response
	fmt.Fprintf(s.writer, "211-Status:\r\n")

	if s.isLoggedIn {
		fmt.Fprintf(s.writer, " Logged in as: %s\r\n", s.user)
	} else {
		fmt.Fprintf(s.writer, " Not logged in\r\n")
	}

	fmt.Fprintf(s.writer, " TYPE: ASCII, FORM: Nonprint; STRUcture: File; transfer MODE: Stream\r\n")

	if s.pasvList != nil {
		fmt.Fprintf(s.writer, " Passive mode enabled\r\n")
	} else if s.activeIP != "" {
		fmt.Fprintf(s.writer, " Active mode: %s:%d\r\n", s.activeIP, s.activePort)
	}

	fmt.Fprintf(s.writer, "211 End of status\r\n")
	s.writer.Flush()
}

// handleHELP handles the HELP command.
// Returns a list of supported commands.
func (s *session) handleHELP(arg string) {
	if arg != "" {
		// Help for specific command - we'll keep it simple
		s.reply(214, fmt.Sprintf("No help available for %s.", arg))
		return
	}

	// List all supported commands using multi-line response
	fmt.Fprintf(s.writer, "214-The following commands are supported:\r\n")
	fmt.Fprintf(s.writer, " USER PASS QUIT ACCT\r\n")
	fmt.Fprintf(s.writer, " CWD CDUP PWD MKD XMKD RMD XRMD\r\n")
	fmt.Fprintf(s.writer, " LIST NLST MLSD MLST\r\n")
	fmt.Fprintf(s.writer, " RETR STOR APPE STOU DELE\r\n")
	fmt.Fprintf(s.writer, " RNFR RNTO REST\r\n")
	fmt.Fprintf(s.writer, " TYPE MODE STRU PORT PASV EPSV EPRT\r\n")
	fmt.Fprintf(s.writer, " SIZE MDTM FEAT OPTS\r\n")
	fmt.Fprintf(s.writer, " AUTH PROT PBSZ\r\n")
	fmt.Fprintf(s.writer, " SYST STAT HELP NOOP SITE\r\n")
	fmt.Fprintf(s.writer, " HOST HASH\r\n")
	fmt.Fprintf(s.writer, "214 End of help\r\n")
	s.writer.Flush()
}

// handleSITE handles the SITE command.
// Provides server-specific commands (RFC 959).
func (s *session) handleSITE(arg string) {
	if arg == "" {
		s.reply(501, "SITE command requires parameters.")
		return
	}

	parts := strings.Fields(arg)
	cmd := strings.ToUpper(parts[0])

	switch cmd {
	case "HELP":
		s.reply(214, "Available SITE commands: HELP, CHMOD")
	case "CHMOD":
		// Syntax: SITE CHMOD <mode> <file>
		if len(parts) < 3 {
			s.reply(501, "Syntax error in parameters or arguments.")
			return
		}
		modeStr := parts[1]
		path := strings.Join(parts[2:], " ") // path might contain spaces

		// Parse octal mode
		mode, err := strconv.ParseUint(modeStr, 8, 32)
		if err != nil {
			s.reply(501, "Invalid mode.")
			return
		}

		// Validate mode: only allow standard permission bits (0-777)
		if mode > 0777 {
			s.reply(501, "Invalid mode: special bits not allowed.")
			return
		}

		if err := s.fs.Chmod(path, os.FileMode(mode)); err != nil {
			s.replyError(err)
			return
		}
		s.reply(200, "SITE CHMOD command successful.")

	default:
		s.reply(502, "SITE command not implemented.")
	}
}
