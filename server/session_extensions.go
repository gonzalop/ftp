package server

import (
	"fmt"
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
