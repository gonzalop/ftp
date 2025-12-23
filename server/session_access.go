package server

func (s *session) handleUSER(user string) error {
	s.user = user
	s.reply(331, "User name okay, need password.")
	return nil
}

func (s *session) handlePASS(pass string) error {
	ctx, err := s.server.driver.Authenticate(s.user, pass, s.host)
	if err != nil {
		// Security audit: failed authentication
		s.server.logger.Warn("authentication_failed",
			"session_id", s.sessionID,
			"remote_ip", s.remoteIP,
			"user", s.user,
			"reason", err.Error(),
		)
		// Metrics collection
		if s.server.metricsCollector != nil {
			s.server.metricsCollector.RecordAuthentication(false, s.user)
		}
		s.reply(530, "Login incorrect.")
		return nil
	}
	s.fs = ctx
	s.isLoggedIn = true
	// Security audit: successful authentication
	s.server.logger.Info("authentication_success",
		"session_id", s.sessionID,
		"remote_ip", s.remoteIP,
		"user", s.user,
	)
	// Metrics collection
	if s.server.metricsCollector != nil {
		s.server.metricsCollector.RecordAuthentication(true, s.user)
	}
	s.reply(230, "User logged in, proceed.")
	return nil
}
