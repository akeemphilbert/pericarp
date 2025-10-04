package mocks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/akeemphilbert/pericarp/examples"
	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	"github.com/google/uuid"
	"github.com/segmentio/ksuid"
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
	if len(events) == 0 {
		return nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	envelopes := make([]pkgdomain.Envelope, len(events))
	for i, event := range events {
		envelope := &TestEnvelope{
			event:     event,
			eventID:   ksuid.New().String(),
			timestamp: time.Now(),
			metadata:  make(map[string]interface{}),
		}
		envelopes[i] = envelope
	}

	// Group events by aggregate GetID
	aggregateID := events[0].AggregateID()
	s.events[aggregateID] = append(s.events[aggregateID], envelopes...)

	return envelopes, nil
}

// Save persists a single event
func (s *InMemoryEventStore) Save(ctx context.Context, event pkgdomain.Event) error {
	envelopes, err := s.Save(ctx, []pkgdomain.Event{event})
	if err != nil {
		return err
	}
	_ = envelopes // Ignore return value for single event
	return nil
}

// GetEvents retrieves events for an aggregate
func (s *InMemoryEventStore) GetEvents(ctx context.Context, aggregateID string, fromVersion int64) ([]pkgdomain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	envelopes, exists := s.events[aggregateID]
	if !exists {
		return []pkgdomain.Event{}, nil
	}

	events := make([]pkgdomain.Event, 0, len(envelopes))
	for _, envelope := range envelopes {
		events = append(events, envelope.Event())
	}

	return events, nil
}

// GetEventsByType retrieves events by type
func (s *InMemoryEventStore) GetEventsByType(ctx context.Context, eventType string) ([]pkgdomain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var events []pkgdomain.Event
	for _, envelopes := range s.events {
		for _, envelope := range envelopes {
			if envelope.Event().Type() == eventType {
				events = append(events, envelope.Event())
			}
		}
	}

	return events, nil
}

// Clear removes all events (useful for test cleanup)
func (s *InMemoryEventStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = make(map[string][]pkgdomain.Envelope)
}

// GetEventCount returns the total number of events stored
func (s *InMemoryEventStore) GetEventCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, envelopes := range s.events {
		count += len(envelopes)
	}
	return count
}

// TestEnvelope is a test implementation of the Envelope interface
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
	handlers map[string][]pkgdomain.EventHandler
	mu       sync.RWMutex
}

// NewInMemoryEventDispatcher creates a new in-memory event dispatcher
func NewInMemoryEventDispatcher() *InMemoryEventDispatcher {
	return &InMemoryEventDispatcher{
		handlers: make(map[string][]pkgdomain.EventHandler),
	}
}

// RegisterHandler registers an event handler
func (d *InMemoryEventDispatcher) RegisterHandler(aggregateType, eventType string, handler pkgdomain.EventHandler) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := fmt.Sprintf("%s:%s", aggregateType, eventType)
	d.handlers[key] = append(d.handlers[key], handler)
	return nil
}

// Dispatch dispatches an event to registered handlers
func (d *InMemoryEventDispatcher) Dispatch(ctx context.Context, event pkgdomain.Event) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", event.AggregateType(), event.Type())
	handlers, exists := d.handlers[key]
	if !exists {
		return nil // No handlers registered
	}

	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			return err
		}
	}

	return nil
}

// GetHandlerCount returns the number of registered handlers
func (d *InMemoryEventDispatcher) GetHandlerCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	count := 0
	for _, handlers := range d.handlers {
		count += len(handlers)
	}
	return count
}

// Clear removes all handlers (useful for test cleanup)
func (d *InMemoryEventDispatcher) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers = make(map[string][]pkgdomain.EventHandler)
}

// InMemoryUserRepository provides an in-memory implementation of UserRepository for testing
type InMemoryUserRepository struct {
	users map[string]*examples.User
	mu    sync.RWMutex
}

// NewInMemoryUserRepository creates a new in-memory user repository
func NewInMemoryUserRepository() *InMemoryUserRepository {
	return &InMemoryUserRepository{
		users: make(map[string]*examples.User),
	}
}

// Save saves a user
func (r *InMemoryUserRepository) Save(user *examples.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.users[user.GetID()] = user
	return nil
}

// FindByID finds a user by GetID
func (r *InMemoryUserRepository) FindByID(id string) (*examples.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, exists := r.users[id]
	if !exists {
		return nil, fmt.Errorf("user not found: %s", id)
	}

	return user, nil
}

// FindByEmail finds a user by email
func (r *InMemoryUserRepository) FindByEmail(email string) (*examples.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, user := range r.users {
		if user.Email() == email {
			return user, nil
		}
	}

	return nil, fmt.Errorf("user not found with email: %s", email)
}

// Delete deletes a user
func (r *InMemoryUserRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.users, id)
	return nil
}

// LoadFromSequence loads a user from a specific sequence number
func (r *InMemoryUserRepository) LoadFromSequence(id string, sequenceNo int64) (*examples.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, exists := r.users[id]
	if !exists {
		return nil, fmt.Errorf("user not found: %s", id)
	}

	// For testing purposes, we'll just return the user as-is
	// In a real implementation, you'd reconstruct from events up to sequenceNo
	return user, nil
}

// Clear removes all users (useful for test cleanup)
func (r *InMemoryUserRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users = make(map[string]*examples.User)
}

// GetUserCount returns the number of users stored
func (r *InMemoryUserRepository) GetUserCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.users)
}

// TestLogger provides a test implementation of Logger
type TestLogger struct {
	Logs []LogEntry
	mu   sync.RWMutex
}

// LogEntry represents a log entry
type LogEntry struct {
	Level   string
	Message string
	Fields  map[string]interface{}
	Time    time.Time
}

// NewTestLogger creates a new test logger
func NewTestLogger() *TestLogger {
	return &TestLogger{
		Logs: make([]LogEntry, 0),
	}
}

// Debug logs a debug message
func (l *TestLogger) Debug(message string, fields ...interface{}) {
	l.log("DEBUG", message, fields...)
}

// Info logs an info message
func (l *TestLogger) Info(message string, fields ...interface{}) {
	l.log("INFO", message, fields...)
}

// Warn logs a warning message
func (l *TestLogger) Warn(message string, fields ...interface{}) {
	l.log("WARN", message, fields...)
}

// Error logs an error message
func (l *TestLogger) Error(message string, fields ...interface{}) {
	l.log("ERROR", message, fields...)
}

// Fatal logs a fatal message
func (l *TestLogger) Fatal(message string, fields ...interface{}) {
	l.log("FATAL", message, fields...)
}

func (l *TestLogger) log(level, message string, fields ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := LogEntry{
		Level:   level,
		Message: message,
		Fields:  make(map[string]interface{}),
		Time:    time.Now(),
	}

	// Convert fields to map
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			if key, ok := fields[i].(string); ok {
				entry.Fields[key] = fields[i+1]
			}
		}
	}

	l.Logs = append(l.Logs, entry)
}

// GetLogs returns all log entries
func (l *TestLogger) GetLogs() []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]LogEntry{}, l.Logs...)
}

// GetLogsByLevel returns log entries by level
func (l *TestLogger) GetLogsByLevel(level string) []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var filtered []LogEntry
	for _, entry := range l.Logs {
		if entry.Level == level {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// Clear removes all log entries
func (l *TestLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Logs = make([]LogEntry, 0)
}

// HasLogEntry checks if a log entry exists
func (l *TestLogger) HasLogEntry(level, message string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, entry := range l.Logs {
		if entry.Level == level && entry.Message == message {
			return true
		}
	}
	return false
}
