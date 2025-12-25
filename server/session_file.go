package server

import (
	"fmt"
	"io"
	"os"
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

	// Parse flags and path
	// Common flags: -l, -a, -R
	// Format: LIST [-flags] [path]
	var path string
	var recursive bool

	args := strings.Fields(arg)
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			if strings.Contains(a, "R") {
				recursive = true
			}
		} else {
			path = a
		}
	}

	// If no path provided, list current
	// if path == "" {
	// 	// internal logic handles empty path as current dir
	// }

	conn, err := s.connData()
	if err != nil {
		s.reply(425, "Can't open data connection.")
		return
	}
	defer conn.Close()

	s.reply(150, "Here comes the directory listing.")

	if recursive {
		err = s.listRecursive(conn, path)
	} else {
		entries, listErr := s.fs.ListDir(path)
		if listErr != nil {
			// If not recursive, we might error out.
			// But for LIST, often empty list is better than error if dir empty,
			// but ListDir usually returns error if not found.
			// However, standard says we should probably send error before opening data conn if path invalid?
			// But we already opened data conn (standard behavior varies).
			// Let's reply error on control channel if data conn empty?
			// Actually RFC says if file not found, 550.
			// But since we already sent 150, we should close data conn and maybe 226 or just empty.
			// But simplest is to try-catch before 150?
			// Let's stick to previous pattern: check error first.
			// Wait, I already opened data conn. If ListDir fails, I should probably close and send 450/550.
			// But `s.fs.ListDir` was called BEFORE `s.connData` in original code.
			// I moved it after to handle recursion streaming.
			// Let's revert to checking first for non-recursive case, or just handle error gracefully.
			err = listErr
		} else {
			for _, entry := range entries {
				s.printListEntry(conn, entry)
			}
		}
	}

	if err != nil {
		// If we haven't written anything, we could send 550?
		// But we sent 150. So we must close data conn (done by defer) and send 450 or 550.
		// Or just 226 Transfer complete (but empty).
		// If path invalid, better 550.
		s.reply(550, "Error listing directory: "+err.Error())
		return
	}

	s.reply(226, "Directory send OK.")
}

func (s *session) listRecursive(w io.Writer, path string) error {
	// 1. List current dir
	entries, err := s.fs.ListDir(path)
	if err != nil {
		return err
	}

	// Print current dir header if we are deep? Standard ls -R style:
	// .:
	// ...
	//
	// ./subdir:
	// ...

	// Helper to print entries
	for _, entry := range entries {
		s.printListEntry(w, entry)
	}

	// 2. Recurse into directories
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "." && entry.Name() != ".." {
			subPath := path
			if subPath == "" || subPath == "." {
				subPath = entry.Name()
			} else {
				if strings.HasSuffix(subPath, "/") {
					subPath += entry.Name()
				} else {
					subPath += "/" + entry.Name()
				}
			}

			// Add a blank line and header
			fmt.Fprintf(w, "\r\n%s:\r\n", subPath)

			// Recurse (ignoring errors for subdirs to keep going)
			_ = s.listRecursive(w, subPath)
		}
	}

	return nil
}

func (s *session) printListEntry(w io.Writer, entry os.FileInfo) {
	// Constructing a Unix-style listing string.
	sStr := fmt.Sprintf("%s 1 owner group %d %s %s\r\n",
		entry.Mode().String(), entry.Size(), entry.ModTime().Format("Jan 02 15:04"), entry.Name())
	fmt.Fprint(w, sStr)
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
