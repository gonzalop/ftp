package server

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func (s *session) handleSIZE(path string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	info, err := s.fs.GetFileInfo(path)
	if err != nil {
		s.reply(550, "Could not get file size.")
		return
	}

	s.reply(213, fmt.Sprintf("%d", info.Size()))
}

func (s *session) handleMDTM(path string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	info, err := s.fs.GetFileInfo(path)
	if err != nil {
		s.reply(550, "Could not get file modification time.")
		return
	}

	// YYYYMMDDHHMMSS format
	// RFC 3659 Section 2.3: "Time values are always represented in UTC"
	s.reply(213, info.ModTime().UTC().Format("20060102150405"))
}

func (s *session) handleFEAT(_ string) {
	if _, err := s.writer.WriteString("211-Features:\r\n"); err != nil {
		return
	}

	features := []string{
		"SIZE",
		"MDTM",
		"PASV",
		"EPSV",
		"EPRT",
		"UTF8",
		"TVFS",
		"MLST",
		"MLST type*;size*;modify*;",
		"REST STREAM",
		"HOST",
		"HASH SHA-1;SHA-256;SHA-512;MD5;CRC32",
		"MFMT",
	}

	if !s.server.disableMLSD {
		features = append(features, "MLSD")
	}

	if s.server.tlsConfig != nil {
		features = append(features, "AUTH TLS", "PBSZ", "PROT")
	}

	for _, f := range features {
		if _, err := s.writer.WriteString(" " + f + "\r\n"); err != nil {
			return
		}
	}

	if _, err := s.writer.WriteString("211 End\r\n"); err != nil {
		return
	}
	_ = s.writer.Flush()
}

func (s *session) handleOPTS(arg string) {
	if strings.HasPrefix(strings.ToUpper(arg), "UTF8 ON") {
		s.reply(200, "Always in UTF8 mode.")
		return
	}
	// OPTS HASH [ALGO]
	if strings.HasPrefix(strings.ToUpper(arg), "HASH") {
		parts := strings.Split(arg, " ")
		if len(parts) > 1 {
			algo := strings.ToUpper(parts[1])
			switch algo {
			case "SHA-1", "SHA-256", "SHA-512", "MD5", "CRC32":
				s.selectedHash = algo
				s.reply(200, algo+" selected.")
				return
			}
		}
	}
	s.reply(501, "Option not understood.")
}

func (s *session) handleMLSD(arg string) {
	if s.server.disableMLSD {
		s.reply(502, "Command not implemented.")
		return
	}

	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	// Parse flags and path
	// MLSD [-R] [path]
	var path string
	var recursive bool

	args := strings.FieldsSeq(arg)
	for a := range args {
		if strings.HasPrefix(a, "-") {
			if strings.Contains(a, "R") {
				recursive = true
			}
		} else {
			path = a
		}
	}

	conn, err := s.connData()
	if err != nil {
		s.reply(425, "Can't open data connection.")
		return
	}
	defer conn.Close()

	s.reply(150, "MLSD listing started.")

	if recursive {
		err = s.mlsdRecursive(conn, path, path)
	} else {
		var entries []os.FileInfo
		entries, err = s.fs.ListDir(path)
		if err == nil {
			for _, entry := range entries {
				s.writeMLEntry(conn, entry, "")
			}
		}
	}

	if err != nil {
		s.reply(550, "Error listing directory: "+err.Error())
		return
	}

	s.reply(226, "MLSD listing complete.")
}

func (s *session) mlsdRecursive(w io.Writer, root, currentPath string) error {
	entries, err := s.fs.ListDir(currentPath)
	if err != nil {
		return err
	}

	// Calculate display path for entries
	displayDir := currentPath
	if displayDir == "" || displayDir == "." {
		displayDir = ""
	}

	for _, entry := range entries {
		s.writeMLEntry(w, entry, displayDir)
	}

	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "." && entry.Name() != ".." {
			subPath := currentPath
			if subPath == "" || subPath == "." {
				subPath = entry.Name()
			} else {
				if strings.HasSuffix(subPath, "/") {
					subPath += entry.Name()
				} else {
					subPath += "/" + entry.Name()
				}
			}
			_ = s.mlsdRecursive(w, root, subPath)
		}
	}

	return nil
}

func (s *session) handleMLST(arg string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	info, err := s.fs.GetFileInfo(arg)
	if err != nil {
		s.reply(550, "Could not get file info.")
		return
	}

	_, _ = s.writer.WriteString("250- Listing follows\r\n")
	if err := s.writer.WriteByte(' '); err != nil {
		return
	}
	s.writeMLEntry(s.writer, info, "")
	_, _ = s.writer.WriteString("250 End\r\n")
	_ = s.writer.Flush()
}

func (s *session) writeMLEntry(w io.Writer, info os.FileInfo, dir string) {
	// Format: type=file;size=123;modify=20210101120000; name
	// If dir is provided, we prepend it to the name for recursive listings
	t := "file"
	if info.IsDir() {
		t = "dir"
	}

	name := info.Name()
	if dir != "" {
		if strings.HasSuffix(dir, "/") {
			name = dir + name
		} else {
			name = dir + "/" + name
		}
	}

	// RFC 3659 Section 2.3: "Time values are always represented in UTC"
	sStr := fmt.Sprintf("type=%s;size=%d;modify=%s; %s\r\n",
		t, info.Size(), info.ModTime().UTC().Format("20060102150405"), name)
	fmt.Fprint(w, sStr)
}
