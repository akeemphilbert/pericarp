package application

// AuthServiceOption configures the DefaultAuthenticationService.
type AuthServiceOption func(*DefaultAuthenticationService)

// WithTokenStore sets a custom token store for server-side OAuth token persistence.
func WithTokenStore(store TokenStore) AuthServiceOption {
	return func(s *DefaultAuthenticationService) {
		if store != nil {
			s.tokens = store
		}
	}
}

// WithAuthorizationChecker sets a custom authorization checker for permission resolution.
// A nil checker disables permission resolution; ValidateSession will return empty permissions.
func WithAuthorizationChecker(checker AuthorizationChecker) AuthServiceOption {
	return func(s *DefaultAuthenticationService) {
		if checker != nil {
			s.authorization = checker
		}
	}
}

// WithLogger sets a custom logger. The default is a no-op logger.
func WithLogger(logger Logger) AuthServiceOption {
	return func(s *DefaultAuthenticationService) {
		if logger != nil {
			s.logger = logger
		}
	}
}
