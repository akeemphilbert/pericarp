package application

import (
	"context"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

// Shared mock implementations for testing

// MockLogger provides a mock implementation of domain.Logger for testing
type MockLogger struct {
	logs []string
}

func NewMockLogger() *MockLogger {
	return &MockLogger{
		logs: make([]string, 0),
	}
}

// Structured logging methods
func (m *MockLogger) Debug(msg string, keysAndValues ...any) {
	m.logs = append(m.logs, "DEBUG: "+msg)
}

func (m *MockLogger) Info(msg string, keysAndValues ...any) {
	m.logs = append(m.logs, "INFO: "+msg)
}

func (m *MockLogger) Warn(msg string, keysAndValues ...any) {
	m.logs = append(m.logs, "WARN: "+msg)
}

func (m *MockLogger) Error(msg string, keysAndValues ...any) {
	m.logs = append(m.logs, "ERROR: "+msg)
}

func (m *MockLogger) Fatal(msg string, keysAndValues ...any) {
	m.logs = append(m.logs, "FATAL: "+msg)
}

// Formatted logging methods
func (m *MockLogger) Debugf(format string, args ...any) {
	m.logs = append(m.logs, "DEBUG: "+format)
}

func (m *MockLogger) Infof(format string, args ...any) {
	m.logs = append(m.logs, "INFO: "+format)
}

func (m *MockLogger) Warnf(format string, args ...any) {
	m.logs = append(m.logs, "WARN: "+format)
}

func (m *MockLogger) Errorf(format string, args ...any) {
	m.logs = append(m.logs, "ERROR: "+format)
}

func (m *MockLogger) Fatalf(format string, args ...any) {
	m.logs = append(m.logs, "FATAL: "+format)
}

func (m *MockLogger) GetLogs() []string {
	return m.logs
}

// UserReadModel represents a simple user read model for testing
type UserReadModel struct {
	ID       string
	Email    string
	Name     string
	IsActive bool
}

// MockUserReadModelRepository provides a mock implementation of UserReadModelRepository for testing
type MockUserReadModelRepository struct {
	users   map[string]*UserReadModel
	saveErr error
	getErr  error
}

func NewMockUserReadModelRepository() *MockUserReadModelRepository {
	return &MockUserReadModelRepository{
		users: make(map[string]*UserReadModel),
	}
}

func (m *MockUserReadModelRepository) SetSaveError(err error) {
	m.saveErr = err
}

func (m *MockUserReadModelRepository) SetGetError(err error) {
	m.getErr = err
}

func (m *MockUserReadModelRepository) GetByID(ctx context.Context, id string) (*UserReadModel, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	user, exists := m.users[id]
	if !exists {
		return nil, NewApplicationError("USER_NOT_FOUND", "User not found", nil)
	}
	return user, nil
}

func (m *MockUserReadModelRepository) GetByEmail(ctx context.Context, email string) (*UserReadModel, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, user := range m.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, nil
}

func (m *MockUserReadModelRepository) List(ctx context.Context, page, pageSize int) ([]UserReadModel, int, error) {
	users := make([]UserReadModel, 0, len(m.users))
	for _, user := range m.users {
		users = append(users, *user)
	}

	// Simple pagination logic for testing
	start := (page - 1) * pageSize
	end := start + pageSize

	if start >= len(users) {
		return []UserReadModel{}, len(users), nil
	}

	if end > len(users) {
		end = len(users)
	}

	return users[start:end], len(users), nil
}

func (m *MockUserReadModelRepository) Save(ctx context.Context, user *UserReadModel) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.users[user.ID] = user
	return nil
}

func (m *MockUserReadModelRepository) Delete(ctx context.Context, id string) error {
	delete(m.users, id)
	return nil
}

func (m *MockUserReadModelRepository) Count(ctx context.Context) (int, error) {
	return len(m.users), nil
}

// MockEventDispatcher provides a mock implementation of domain.EventDispatcher for testing
type MockEventDispatcher struct {
	handlers map[string]domain.EventHandler
}

func NewMockEventDispatcher() *MockEventDispatcher {
	return &MockEventDispatcher{
		handlers: make(map[string]domain.EventHandler),
	}
}

func (m *MockEventDispatcher) Dispatch(ctx context.Context, envelopes []domain.Envelope) error {
	for _, envelope := range envelopes {
		if handler, exists := m.handlers[envelope.Event().EventType()]; exists {
			if err := handler.Handle(ctx, envelope); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *MockEventDispatcher) Subscribe(eventType string, handler domain.EventHandler) error {
	m.handlers[eventType] = handler
	return nil
}

// MockEnvelope provides a mock implementation of domain.Envelope for testing
type MockEnvelope struct {
	event     domain.Event
	eventID   string
	timestamp time.Time
}

func NewMockEnvelope(event domain.Event) *MockEnvelope {
	return &MockEnvelope{
		event:     event,
		eventID:   "mock-event-id",
		timestamp: time.Now(),
	}
}

func (m *MockEnvelope) Event() domain.Event {
	return m.event
}

func (m *MockEnvelope) Metadata() map[string]any {
	return make(map[string]any)
}

func (m *MockEnvelope) EventID() string {
	return m.eventID
}

func (m *MockEnvelope) Timestamp() time.Time {
	return m.timestamp
}

// MockEventHandler provides a mock implementation of domain.EventHandler for testing
type MockEventHandler struct {
	handleFunc func(context.Context, domain.Envelope) error
	eventTypes []string
}

func NewMockEventHandler(eventTypes []string, handleFunc func(context.Context, domain.Envelope) error) *MockEventHandler {
	return &MockEventHandler{
		handleFunc: handleFunc,
		eventTypes: eventTypes,
	}
}

func (m *MockEventHandler) Handle(ctx context.Context, envelope domain.Envelope) error {
	if m.handleFunc != nil {
		return m.handleFunc(ctx, envelope)
	}
	return nil
}

func (m *MockEventHandler) EventTypes() []string {
	return m.eventTypes
}
