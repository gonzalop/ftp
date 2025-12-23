package server

import (
	"fmt"
	"strings"
	"time"
)

func (s *session) handleHOST(arg string) {
	if s.isLoggedIn {
		s.reply(503, "Cannot change host after login.")
		return
	}
	s.host = arg
	s.reply(220, "Host accepted.")
}

func (s *session) handleHASH(arg string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	path := arg
	// Use selected hash algorithm
	hash, err := s.fs.GetHash(path, s.selectedHash)
	if err != nil {
		s.replyError(err)
		return
	}

	s.reply(213, fmt.Sprintf("%s %s %s", s.selectedHash, hash, path))
}

func (s *session) handleMFMT(arg string) {
	if !s.isLoggedIn {
		s.reply(530, "Not logged in.")
		return
	}

	parts := strings.SplitN(arg, " ", 2)
	if len(parts) != 2 {
		s.reply(501, "Syntax error in parameters or arguments.")
		return
	}

	timeStr := parts[0]
	path := parts[1]

	// Format: YYYYMMDDHHMMSS
	// Go layout: 20060102150405
	t, err := time.Parse("20060102150405", timeStr)
	if err != nil {
		s.reply(501, "Invalid time format.")
		return
	}

	if err := s.fs.SetTime(path, t); err != nil {
		s.replyError(err)
		return
	}

	// Response format: "Modify=YYYYMMDDHHMMSS; /path"
	s.reply(213, fmt.Sprintf("Modify=%s; %s", timeStr, path))
}
