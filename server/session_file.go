package server

import (
	"fmt"
)

func (s *session) handlePWD() {
	if !s.isLoggedIn {
		s.reply(530, "Please login with USER and PASS.")
		return
	}
	s.reply(257, "\"/\" is the current directory.")
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
