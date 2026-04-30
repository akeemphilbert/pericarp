package application

import (
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
	esDomain "github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

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

// WithEventStore sets the event store for atomic event persistence via UnitOfWork.
// When configured, FindOrCreateAgent will commit events atomically before saving projections.
func WithEventStore(store esDomain.EventStore) AuthServiceOption {
	return func(s *DefaultAuthenticationService) {
		if store != nil {
			s.eventStore = store
		}
	}
}

// WithEventDispatcher sets an in-process EventDispatcher that receives every
// committed domain event after the UnitOfWork persists it. Consumers can
// Subscribe[T] to react to events such as agent.created (e.g., to auto-assign
// a default role). Dispatch is best-effort: handler errors are non-fatal and
// do not roll back the auth operation, since the event is already durable.
// Dispatch only fires when an EventStore is also configured via WithEventStore.
func WithEventDispatcher(dispatcher *esDomain.EventDispatcher) AuthServiceOption {
	return func(s *DefaultAuthenticationService) {
		if dispatcher != nil {
			s.dispatcher = dispatcher
		}
	}
}

// WithJWTService sets a JWTService for issuing identity tokens.
// When configured, IssueIdentityToken will produce a signed JWT;
// otherwise it returns an empty string (opaque-session-only mode).
func WithJWTService(js JWTService) AuthServiceOption {
	return func(s *DefaultAuthenticationService) {
		if js != nil {
			s.jwtService = js
		}
	}
}

// WithPasswordCredentialRepository wires a PasswordCredentialRepository for
// password authentication support. The password methods on
// AuthenticationService return ErrPasswordSupportNotConfigured until this
// is set.
func WithPasswordCredentialRepository(repo repositories.PasswordCredentialRepository) AuthServiceOption {
	return func(s *DefaultAuthenticationService) {
		if repo != nil {
			s.passwordCredentials = repo
		}
	}
}

// WithSubscriptionService wires a SubscriptionService for snapshotting
// subscription state into issued JWTs. When unset, IssueIdentityToken
// issues tokens with no subscription claim.
func WithSubscriptionService(svc SubscriptionService) AuthServiceOption {
	return func(s *DefaultAuthenticationService) {
		if svc != nil {
			s.subscriptionService = svc
		}
	}
}

// WithBcryptCost overrides the bcrypt cost used when hashing newly
// registered or updated passwords. A non-positive value falls back to
// bcrypt.DefaultCost.
func WithBcryptCost(cost int) AuthServiceOption {
	return func(s *DefaultAuthenticationService) {
		s.bcryptCost = cost
	}
}
