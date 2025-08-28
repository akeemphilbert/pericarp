package infrastructure

import (
	"context"
	"fmt"

	"github.com/akeemphilbert/pericarp/internal/domain"
	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	"github.com/segmentio/ksuid"
)

// UserEventSourcingRepository implements UserRepository using event sourcing
type UserEventSourcingRepository struct {
	eventStore pkgdomain.EventStore
	logger     pkgdomain.Logger
}

// NewUserEventSourcingRepository creates a new event sourcing repository for users
func NewUserEventSourcingRepository(eventStore pkgdomain.EventStore, logger pkgdomain.Logger) *UserEventSourcingRepository {
	return &UserEventSourcingRepository{
		eventStore: eventStore,
		logger:     logger,
	}
}

// Save persists the user aggregate by saving its uncommitted events
func (r *UserEventSourcingRepository) Save(user *domain.User) error {
	ctx := context.Background()

	uncommittedEvents := user.UncommittedEvents()
	if len(uncommittedEvents) == 0 {
		r.logger.Debug("No uncommitted events to save", "user_id", user.ID())
		return nil
	}

	r.logger.Debug("Saving user events", "user_id", user.ID(), "event_count", len(uncommittedEvents))

	// Save events to event store
	envelopes, err := r.eventStore.Save(ctx, uncommittedEvents)
	if err != nil {
		r.logger.Error("Failed to save user events", "user_id", user.ID(), "error", err)
		return fmt.Errorf("failed to save user events: %w", err)
	}

	// Mark events as committed
	user.MarkEventsAsCommitted()

	r.logger.Info("User events saved successfully", "user_id", user.ID(), "envelopes_created", len(envelopes))
	return nil
}

// FindByID loads a user aggregate by reconstructing it from events
func (r *UserEventSourcingRepository) FindByID(id ksuid.KSUID) (*domain.User, error) {
	ctx := context.Background()
	aggregateID := id.String() // ✅ Correct conversion

	r.logger.Debug("Loading user from events", "user_id", aggregateID)

	// Load events from event store
	envelopes, err := r.eventStore.Load(ctx, aggregateID)
	if err != nil {
		r.logger.Error("Failed to load user events", "user_id", aggregateID, "error", err)
		return nil, fmt.Errorf("failed to load user events: %w", err)
	}

	if len(envelopes) == 0 {
		r.logger.Debug("No events found for user", "user_id", aggregateID)
		return nil, fmt.Errorf("user not found: %s", aggregateID)
	}

	// Extract events from envelopes
	events := make([]pkgdomain.Event, len(envelopes))
	for i, envelope := range envelopes {
		events[i] = envelope.Event()
	}

	// Create empty user aggregate
	user := &domain.User{}

	// Reconstruct user from events
	user.LoadFromHistory(events)

	r.logger.Debug("User loaded from events", "user_id", aggregateID, "version", user.Version(), "event_count", len(events))
	return user, nil
}

// FindByEmail finds a user by email address
// Note: This is not efficient with pure event sourcing and would typically use a read model
// For demo purposes, we'll load all user events and check emails
func (r *UserEventSourcingRepository) FindByEmail(email string) (*domain.User, error) {
	r.logger.Warn("FindByEmail is not efficient with pure event sourcing - consider using read models", "email", email)

	// In a real implementation, you would:
	// 1. Use a read model/projection to maintain email -> user ID mapping
	// 2. Or use event store queries if supported
	// 3. Or maintain an in-memory cache

	// For now, return an error indicating this should use read models
	return nil, fmt.Errorf("FindByEmail not implemented for event sourcing repository - use read model instead")
}

// Delete removes a user (in event sourcing, this would typically be a "soft delete" event)
func (r *UserEventSourcingRepository) Delete(id ksuid.KSUID) error {
	// In event sourcing, deletion is typically handled by emitting a "deleted" event
	// rather than actually removing data. For this demo, we'll return an error
	// indicating this should be handled through domain methods

	aggregateID := id.String() // ✅ Correct conversion
	r.logger.Warn("Delete operation should be handled through domain methods (e.g., Deactivate)", "user_id", aggregateID)
	return fmt.Errorf("delete operation should be handled through domain methods - use Deactivate instead")
}

// LoadFromVersion loads a user aggregate from a specific version (implements domain.UserRepository)
func (r *UserEventSourcingRepository) LoadFromVersion(id ksuid.KSUID, version int) (*domain.User, error) {
	ctx := context.Background()
	aggregateID := id.String() // ✅ Correct conversion

	r.logger.Debug("Loading user from specific version", "user_id", aggregateID, "version", version)

	// Load events from specific version
	envelopes, err := r.eventStore.LoadFromVersion(ctx, aggregateID, version)
	if err != nil {
		r.logger.Error("Failed to load user events from version", "user_id", aggregateID, "version", version, "error", err)
		return nil, fmt.Errorf("failed to load user events from version %d: %w", version, err)
	}

	if len(envelopes) == 0 {
		r.logger.Debug("No events found for user from version", "user_id", aggregateID, "version", version)
		return nil, fmt.Errorf("user not found from version %d: %s", version, aggregateID)
	}

	// Extract events from envelopes
	events := make([]pkgdomain.Event, len(envelopes))
	for i, envelope := range envelopes {
		events[i] = envelope.Event()
	}

	// Create empty user aggregate
	user := &domain.User{}

	// Reconstruct user from events
	user.LoadFromHistory(events)

	r.logger.Debug("User loaded from version", "user_id", aggregateID, "final_version", user.Version(), "event_count", len(events))
	return user, nil
}
