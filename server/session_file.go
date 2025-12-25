package server

import (
	"fmt"
	"io"
	"strings"
)

func (s *session) handlePWD() {
	if !s.isLoggedIn {
		s.reply(530, "Please login with USER and PASS.")
		return
	}
	cwd, err := s.fs.GetWd()
	if err != nil {
		s.replyError(err)
		return
	}
	s.reply(257, fmt.Sprintf("%q is the current directory.", cwd))
}

func (s *session) handleCWD(path string) {
	if !s.isLoggedIn {
		s.reply(530, "Please login with USER and PASS.")
		return
	}
	if err := s.fs.ChangeDir(path); err != nil {
		s.replyError(err)
		return
	}

	// Check for .message file if enabled
	if s.server.enableDirMessage {
		f, err := s.fs.OpenFile(".message", 0)
		if err == nil {
			// Read up to 2KB to avoid excessive memory usage
			lr := io.LimitReader(f, 2048)
			b, _ := io.ReadAll(lr)
			f.Close()
			if len(b) > 0 {
				fmt.Fprintf(s.writer, "250-Message:\r\n")
				// Trim trailing newlines to avoid an extra empty line at the end
				msg := strings.TrimRight(string(b), "\r\n")
				lines := strings.Split(msg, "\n")
				for _, line := range lines {
					line = strings.TrimRight(line, "\r")
					fmt.Fprintf(s.writer, "250-%s\r\n", line)
				}
			}
		}
	}
	s.reply(250, "Directory successfully changed.")
}

func (s *session) handleCDUP() {
	s.handleCWD("..")
}

func (s *session) handleLIST(arg string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	// In standard FTP, LIST without arguments lists the current directory.
	// Some clients might send a path.
	path := arg

	entries, err := s.fs.ListDir(path)
	if err != nil {
		s.replyError(err)
		return
	}

	conn, err := s.connData()
	if err != nil {
		s.reply(425, "Can't open data connection.")
		return
	}
	defer conn.Close()

	s.reply(150, "Here comes the directory listing.")

	for _, entry := range entries {
		// Constructing a Unix-style listing string.
		// Note: This is a simplified format compatible with most clients.
		sStr := fmt.Sprintf("%s 1 owner group %d %s %s\r\n",
			entry.Mode().String(), entry.Size(), entry.ModTime().Format("Jan 02 15:04"), entry.Name())
		fmt.Fprint(conn, sStr)
	}

	s.reply(226, "Directory send OK.")
}

func (s *session) handleNLST(arg string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	path := arg
	entries, err := s.fs.ListDir(path)
	if err != nil {
		s.replyError(err)
		return
	}

	conn, err := s.connData()
	if err != nil {
		s.reply(425, "Can't open data connection.")
		return
	}
	defer conn.Close()

	s.reply(150, "Here comes the file list.")

	for _, entry := range entries {
		fmt.Fprintf(conn, "%s\r\n", entry.Name())
	}

	s.reply(226, "Transfer complete.")
}

func (s *session) handleMKD(path string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}
	if err := s.fs.MakeDir(path); err != nil {
		s.replyError(err)
		return
	}
	// Security audit: directory created
	s.server.logger.Info("directory_created",
		"session_id", s.sessionID,
		"remote_ip", s.redactIP(s.remoteIP),
		"user", s.user,
		"host", s.host,
		"path", s.redactPath(path),
	)
	// RFC 959: 257 "PATHNAME" created.
	// Quote the path.
	s.reply(257, fmt.Sprintf("%q created.", path))
}

func (s *session) handleRMD(path string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}
	if err := s.fs.RemoveDir(path); err != nil {
		s.replyError(err)
		return
	}
	// Security audit: directory removed
	s.server.logger.Info("directory_removed",
		"session_id", s.sessionID,
		"remote_ip", s.redactIP(s.remoteIP),
		"user", s.user,
		"host", s.host,
		"path", s.redactPath(path),
	)
	s.reply(250, "Directory removed.")
}

func (s *session) handleDELE(path string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}
	if err := s.fs.DeleteFile(path); err != nil {
		s.replyError(err)
		return
	}
	// Security audit: file deleted
	s.server.logger.Info("file_deleted",
		"session_id", s.sessionID,
		"remote_ip", s.redactIP(s.remoteIP),
		"user", s.user,
		"host", s.host,
		"path", s.redactPath(path),
	)
	s.reply(250, "File deleted.")
}

func (s *session) handleRNFR(path string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	// Verify file exists
	_, err := s.fs.GetFileInfo(path)
	if err != nil {
		s.reply(550, "File not found.")
		return
	}

	s.renameFrom = path
	s.reply(350, "Requested file action pending further information.")
}

func (s *session) handleRNTO(path string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	if s.renameFrom == "" {
		s.reply(503, "Bad sequence of commands. Send RNFR first.")
		return
	}

	err := s.fs.Rename(s.renameFrom, path)
	if err != nil {
		s.replyError(err)
		s.renameFrom = ""
		return
	}

	s.renameFrom = ""
	s.reply(250, "Requested file action successful, file renamed.")
}
