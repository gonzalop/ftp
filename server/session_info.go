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

	s.reply(150, "MLSD listing started.")

	for _, entry := range entries {
		s.writeMLEntry(conn, entry)
	}

	s.reply(226, "MLSD listing complete.")
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
	s.writeMLEntry(s.writer, info)
	_, _ = s.writer.WriteString("250 End\r\n")
	_ = s.writer.Flush()
}

func (s *session) writeMLEntry(w io.Writer, info os.FileInfo) {
	// Format: type=file;size=123;modify=20210101120000; name
	t := "file"
	if info.IsDir() {
		t = "dir"
	}

	// RFC 3659 Section 2.3: "Time values are always represented in UTC"
	sStr := fmt.Sprintf("type=%s;size=%d;modify=%s; %s\r\n",
		t, info.Size(), info.ModTime().UTC().Format("20060102150405"), info.Name())
	fmt.Fprint(w, sStr)
}
