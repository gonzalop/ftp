package server

func (s *session) handleUSER(user string) error {
	s.user = user
	s.reply(331, "User name okay, need password.")
	return nil
}

func (s *session) handlePASS(pass string) error {
	ctx, err := s.server.driver.Authenticate(s.user, pass, s.host)
	if err != nil {
		s.reply(530, "Login incorrect.")
		return nil
	}
	s.fs = ctx
	s.isLoggedIn = true
	s.reply(230, "User logged in, proceed.")
	return nil
}
