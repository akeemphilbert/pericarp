package application

import (
	esDomain "github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// InviteServiceOption configures the InviteService.
type InviteServiceOption func(*InviteService)

// WithInviteEventStore sets the event store for atomic event persistence via UnitOfWork.
func WithInviteEventStore(store esDomain.EventStore) InviteServiceOption {
	return func(s *InviteService) {
		if store != nil {
			s.eventStore = store
		}
	}
}

// WithInviteLogger sets a custom logger for the InviteService.
func WithInviteLogger(logger Logger) InviteServiceOption {
	return func(s *InviteService) {
		if logger != nil {
			s.logger = logger
		}
	}
}
