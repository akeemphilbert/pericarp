package mocks

import (
	"context"
	"fmt"
	"sync"
	"time"

	internalapp "github.com/akeemphilbert/pericarp/internal/application"
	internaldomain "github.com/akeemphilbert/pericarp/internal/domain"
	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	"github.com/google/uuid"
)

// InMemoryEventStore provides an in-memory implementation of EventStore for testing
type InMemoryEventStore struct {
	events map[string][]pkgdomain.Envelope // aggregateID -> events
	mu     sync.RWMutex
}

// NewInMemoryEventStore creates a new in-memory event store
func NewInMemoryEventStore() *InMemoryEventStore {
	return &InMemoryEventStore{
		events: make(map[string][]pkgdomain.Envelope),
	}
}

// Save persists events and returns envelopes with metadata
func (s *InMemoryEventStore) Save(ctx context.Context, events []pkgdomain.Event) ([]pkgdomain.Envelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var envelopes []pkgdomain.Envelope

	for _, event := range events {
		envelope := &TestEnvelope{
			event:     event,
			eventID:   uuid.New().String(),
			timestamp: time.Now(),
			metadata: map[string]interface{}{
				"aggregate_id": event.AggregateID(),
				"event_type":   event.EventType(),
				"version":      event.Version(),
			},
		}

		s.events[event.AggregateID()] = append(s.events[event.AggregateID()], envelope)
		envelopes = append(envelopes, envelope)
	}

	return envelopes, nil
}

// Load retrieves all events for an aggregate
func (s *InMemoryEventStore) Load(ctx context.Context, aggregateID string) ([]pkgdomain.Envelope, error) {
	return s.LoadFromVersion(ctx, aggregateID, 0)
}

// LoadFromVersion retrieves events for an aggregate starting from a specific version
func (s *InMemoryEventStore) LoadFromVersion(ctx context.Context, aggregateID string, version int) ([]pkgdomain.Envelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allEvents := s.events[aggregateID]
	var filteredEvents []pkgdomain.Envelope

	for _, envelope := range allEvents {
		if envelope.Event().Version() > version {
			filteredEvents = append(filteredEvents, envelope)
		}
	}

	return filteredEvents, nil
}

// Clear removes all events (useful for test cleanup)
func (s *InMemoryEventStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = make(map[string][]pkgdomain.Envelope)
}

// TestEnvelope implements pkgdomain.Envelope for testing
type TestEnvelope struct {
	event     pkgdomain.Event
	eventID   string
	timestamp time.Time
	metadata  map[string]interface{}
}

func (e *TestEnvelope) Event() pkgdomain.Event {
	return e.event
}

func (e *TestEnvelope) EventID() string {
	return e.eventID
}

func (e *TestEnvelope) Timestamp() time.Time {
	return e.timestamp
}

func (e *TestEnvelope) Metadata() map[string]interface{} {
	return e.metadata
}

// InMemoryEventDispatcher provides an in-memory implementation of EventDispatcher for testing
type InMemoryEventDispatcher struct {
	handlers         map[string][]pkgdomain.EventHandler
	dispatchedEvents []pkgdomain.Envelope
	mu               sync.RWMutex
}

// NewInMemoryEventDispatcher creates a new in-memory event dispatcher
func NewInMemoryEventDispatcher() *InMemoryEventDispatcher {
	return &InMemoryEventDispatcher{
		handlers: make(map[string][]pkgdomain.EventHandler),
	}
}

// Dispatch sends envelopes to registered event handlers
func (d *InMemoryEventDispatcher) Dispatch(ctx context.Context, envelopes []pkgdomain.Envelope) error {
	d.mu.Lock()
	d.dispatchedEvents = append(d.dispatchedEvents, envelopes...)
	d.mu.Unlock()

	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, envelope := range envelopes {
		eventType := envelope.Event().EventType()
		handlers := d.handlers[eventType]

		for _, handler := range handlers {
			if err := handler.Handle(ctx, envelope); err != nil {
				return fmt.Errorf("handler failed for event %s: %w", eventType, err)
			}
		}
	}

	return nil
}

// Subscribe registers an event handler for specific event types
func (d *InMemoryEventDispatcher) Subscribe(eventType string, handler pkgdomain.EventHandler) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.handlers[eventType] = append(d.handlers[eventType], handler)
	return nil
}

// GetDispatchedEvents returns all dispatched events (useful for testing)
func (d *InMemoryEventDispatcher) GetDispatchedEvents() []pkgdomain.Envelope {
	d.mu.RLock()
	defer d.mu.RUnlock()

	events := make([]pkgdomain.Envelope, len(d.dispatchedEvents))
	copy(events, d.dispatchedEvents)
	return events
}

// Clear removes all dispatched events and handlers (useful for test cleanup)
func (d *InMemoryEventDispatcher) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers = make(map[string][]pkgdomain.EventHandler)
	d.dispatchedEvents = nil
}

// InMemoryUnitOfWork provides an in-memory implementation of UnitOfWork for testing
type InMemoryUnitOfWork struct {
	eventStore       pkgdomain.EventStore
	eventDispatcher  pkgdomain.EventDispatcher
	registeredEvents []pkgdomain.Event
	mu               sync.Mutex
}

// NewInMemoryUnitOfWork creates a new in-memory unit of work
func NewInMemoryUnitOfWork(eventStore pkgdomain.EventStore, eventDispatcher pkgdomain.EventDispatcher) *InMemoryUnitOfWork {
	return &InMemoryUnitOfWork{
		eventStore:      eventStore,
		eventDispatcher: eventDispatcher,
	}
}

// RegisterEvents adds events to be persisted in the current transaction
func (u *InMemoryUnitOfWork) RegisterEvents(events []pkgdomain.Event) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.registeredEvents = append(u.registeredEvents, events...)
}

// Commit persists all registered events and returns envelopes
func (u *InMemoryUnitOfWork) Commit(ctx context.Context) ([]pkgdomain.Envelope, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if len(u.registeredEvents) == 0 {
		return []pkgdomain.Envelope{}, nil
	}

	// Save events
	envelopes, err := u.eventStore.Save(ctx, u.registeredEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to save events: %w", err)
	}

	// Dispatch events
	if err := u.eventDispatcher.Dispatch(ctx, envelopes); err != nil {
		return nil, fmt.Errorf("failed to dispatch events: %w", err)
	}

	// Clear registered events
	u.registeredEvents = nil

	return envelopes, nil
}

// Rollback discards all registered events
func (u *InMemoryUnitOfWork) Rollback() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.registeredEvents = nil
	return nil
}

// InMemoryUserRepository provides an in-memory implementation of UserRepository for testing
type InMemoryUserRepository struct {
	users map[uuid.UUID]*internaldomain.User
	mu    sync.RWMutex
}

// NewInMemoryUserRepository creates a new in-memory user repository
func NewInMemoryUserRepository() *InMemoryUserRepository {
	return &InMemoryUserRepository{
		users: make(map[uuid.UUID]*internaldomain.User),
	}
}

// Save persists a user
func (r *InMemoryUserRepository) Save(user *internaldomain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create a copy to avoid shared state issues
	userCopy := *user
	r.users[user.UserID()] = &userCopy
	return nil
}

// FindByID retrieves a user by ID
func (r *InMemoryUserRepository) FindByID(id uuid.UUID) (*internaldomain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, exists := r.users[id]
	if !exists {
		return nil, fmt.Errorf("user not found: %s", id)
	}

	// Return a copy to avoid shared state issues
	userCopy := *user
	return &userCopy, nil
}

// FindByEmail retrieves a user by email
func (r *InMemoryUserRepository) FindByEmail(email string) (*internaldomain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, user := range r.users {
		if user.Email() == email {
			// Return a copy to avoid shared state issues
			userCopy := *user
			return &userCopy, nil
		}
	}

	return nil, fmt.Errorf("user not found with email: %s", email)
}

// Delete removes a user
func (r *InMemoryUserRepository) Delete(id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.users, id)
	return nil
}

// LoadFromVersion loads a user from a specific version (not implemented for in-memory)
func (r *InMemoryUserRepository) LoadFromVersion(id uuid.UUID, version int) (*internaldomain.User, error) {
	return r.FindByID(id)
}

// Clear removes all users (useful for test cleanup)
func (r *InMemoryUserRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users = make(map[uuid.UUID]*internaldomain.User)
}

// InMemoryUserReadModelRepository provides an in-memory implementation of UserReadModelRepository for testing
type InMemoryUserReadModelRepository struct {
	users map[uuid.UUID]*internalapp.UserReadModel
	mu    sync.RWMutex
}

// NewInMemoryUserReadModelRepository creates a new in-memory user read model repository
func NewInMemoryUserReadModelRepository() *InMemoryUserReadModelRepository {
	return &InMemoryUserReadModelRepository{
		users: make(map[uuid.UUID]*internalapp.UserReadModel),
	}
}

// GetByID retrieves a user read model by ID
func (r *InMemoryUserReadModelRepository) GetByID(ctx context.Context, id uuid.UUID) (*internalapp.UserReadModel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, exists := r.users[id]
	if !exists {
		return nil, fmt.Errorf("user not found: %s", id)
	}

	// Return a copy to avoid shared state issues
	userCopy := *user
	return &userCopy, nil
}

// GetByEmail retrieves a user read model by email
func (r *InMemoryUserReadModelRepository) GetByEmail(ctx context.Context, email string) (*internalapp.UserReadModel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, user := range r.users {
		if user.Email == email {
			// Return a copy to avoid shared state issues
			userCopy := *user
			return &userCopy, nil
		}
	}

	return nil, fmt.Errorf("user not found with email: %s", email)
}

// List retrieves a paginated list of user read models with optional active filter
func (r *InMemoryUserReadModelRepository) List(ctx context.Context, page, pageSize int, active *bool) ([]internalapp.UserReadModel, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filteredUsers []internalapp.UserReadModel
	for _, user := range r.users {
		if active == nil || user.IsActive == *active {
			filteredUsers = append(filteredUsers, *user)
		}
	}

	totalCount := len(filteredUsers)

	// Apply pagination
	start := (page - 1) * pageSize
	end := start + pageSize

	if start >= totalCount {
		return []internalapp.UserReadModel{}, totalCount, nil
	}

	if end > totalCount {
		end = totalCount
	}

	return filteredUsers[start:end], totalCount, nil
}

// Save saves or updates a user read model
func (r *InMemoryUserReadModelRepository) Save(ctx context.Context, user *internalapp.UserReadModel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create a copy to avoid shared state issues
	userCopy := *user
	r.users[user.ID] = &userCopy
	return nil
}

// Delete removes a user read model
func (r *InMemoryUserReadModelRepository) Delete(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.users, id)
	return nil
}

// Count returns the total number of users with optional active filter
func (r *InMemoryUserReadModelRepository) Count(ctx context.Context, active *bool) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, user := range r.users {
		if active == nil || user.IsActive == *active {
			count++
		}
	}

	return count, nil
}

// Clear removes all users (useful for test cleanup)
func (r *InMemoryUserReadModelRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users = make(map[uuid.UUID]*internalapp.UserReadModel)
}
