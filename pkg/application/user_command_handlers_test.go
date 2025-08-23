package application

import (
	"context"
	"testing"

	"github.com/example/pericarp/pkg/domain"
)

// Mock implementations for testing
type mockUserRepository struct {
	users       map[string]*domain.User
	emailToUser map[string]*domain.User
}

func newMockUserRepository() *mockUserRepository {
	return &mockUserRepository{
		users:       make(map[string]*domain.User),
		emailToUser: make(map[string]*domain.User),
	}
}

func (m *mockUserRepository) Save(ctx context.Context, user *domain.User) error {
	m.users[user.ID()] = user
	m.emailToUser[user.Email()] = user
	return nil
}

func (m *mockUserRepository) Load(ctx context.Context, id string) (*domain.User, error) {
	user, exists := m.users[id]
	if !exists {
		return nil, NewApplicationError("USER_NOT_FOUND", "User not found", nil)
	}
	return user, nil
}

func (m *mockUserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	user, exists := m.emailToUser[email]
	if !exists {
		return nil, NewApplicationError("USER_NOT_FOUND", "User not found", nil)
	}
	return user, nil
}

func (m *mockUserRepository) Exists(ctx context.Context, id string) (bool, error) {
	_, exists := m.users[id]
	return exists, nil
}

func (m *mockUserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	_, exists := m.emailToUser[email]
	return exists, nil
}

type mockUnitOfWork struct {
	events []domain.Event
}

func newMockUnitOfWork() *mockUnitOfWork {
	return &mockUnitOfWork{
		events: make([]domain.Event, 0),
	}
}

func (m *mockUnitOfWork) RegisterEvents(events []domain.Event) {
	m.events = append(m.events, events...)
}

func (m *mockUnitOfWork) Commit(ctx context.Context) ([]domain.Envelope, error) {
	// For testing, just return empty envelopes
	envelopes := make([]domain.Envelope, len(m.events))
	m.events = make([]domain.Event, 0) // Clear events after commit
	return envelopes, nil
}

func (m *mockUnitOfWork) Rollback() error {
	m.events = make([]domain.Event, 0)
	return nil
}

type mockLogger struct{}

func (m *mockLogger) Debug(msg string, keysAndValues ...interface{}) {}
func (m *mockLogger) Debugf(format string, args ...interface{})      {}
func (m *mockLogger) Info(msg string, keysAndValues ...interface{})  {}
func (m *mockLogger) Infof(format string, args ...interface{})       {}
func (m *mockLogger) Warn(msg string, keysAndValues ...interface{})  {}
func (m *mockLogger) Warnf(format string, args ...interface{})       {}
func (m *mockLogger) Error(msg string, keysAndValues ...interface{}) {}
func (m *mockLogger) Errorf(format string, args ...interface{})      {}
func (m *mockLogger) Fatal(msg string, keysAndValues ...interface{}) {}
func (m *mockLogger) Fatalf(format string, args ...interface{})      {}

func TestCreateUserHandler_Handle_ValidCommand_CreatesUser(t *testing.T) {
	// Arrange
	userRepo := newMockUserRepository()
	unitOfWork := newMockUnitOfWork()
	handler := NewCreateUserHandler(userRepo, unitOfWork)
	logger := &mockLogger{}

	cmd := CreateUserCommand{
		ID:    "user-123",
		Email: "john@example.com",
		Name:  "John Doe",
	}

	// Act
	err := handler.Handle(context.Background(), logger, cmd)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify user was saved
	exists, _ := userRepo.Exists(context.Background(), cmd.ID)
	if !exists {
		t.Error("Expected user to exist after creation")
	}

	// Verify user can be loaded
	user, err := userRepo.Load(context.Background(), cmd.ID)
	if err != nil {
		t.Fatalf("Expected to load user, got error: %v", err)
	}

	if user.Email() != cmd.Email {
		t.Errorf("Expected email %s, got %s", cmd.Email, user.Email())
	}

	if user.Name() != cmd.Name {
		t.Errorf("Expected name %s, got %s", cmd.Name, user.Name())
	}
}

func TestCreateUserHandler_Handle_UserAlreadyExists_ReturnsError(t *testing.T) {
	// Arrange
	userRepo := newMockUserRepository()
	unitOfWork := newMockUnitOfWork()
	handler := NewCreateUserHandler(userRepo, unitOfWork)
	logger := &mockLogger{}

	// Create existing user
	existingUser, _ := domain.NewUser("user-123", "existing@example.com", "Existing User")
	userRepo.Save(context.Background(), existingUser)

	cmd := CreateUserCommand{
		ID:    "user-123",
		Email: "john@example.com",
		Name:  "John Doe",
	}

	// Act
	err := handler.Handle(context.Background(), logger, cmd)

	// Assert
	if err == nil {
		t.Fatal("Expected error for existing user, got nil")
	}

	appErr, ok := err.(ApplicationError)
	if !ok {
		t.Errorf("Expected ApplicationError, got %T", err)
	}

	if appErr.Code != "USER_ALREADY_EXISTS" {
		t.Errorf("Expected error code 'USER_ALREADY_EXISTS', got %s", appErr.Code)
	}
}

func TestCreateUserHandler_Handle_EmailAlreadyExists_ReturnsError(t *testing.T) {
	// Arrange
	userRepo := newMockUserRepository()
	unitOfWork := newMockUnitOfWork()
	handler := NewCreateUserHandler(userRepo, unitOfWork)
	logger := &mockLogger{}

	// Create user with existing email
	existingUser, _ := domain.NewUser("existing-user", "john@example.com", "Existing User")
	userRepo.Save(context.Background(), existingUser)

	cmd := CreateUserCommand{
		ID:    "user-123",
		Email: "john@example.com",
		Name:  "John Doe",
	}

	// Act
	err := handler.Handle(context.Background(), logger, cmd)

	// Assert
	if err == nil {
		t.Fatal("Expected error for existing email, got nil")
	}

	appErr, ok := err.(ApplicationError)
	if !ok {
		t.Errorf("Expected ApplicationError, got %T", err)
	}

	if appErr.Code != "EMAIL_ALREADY_EXISTS" {
		t.Errorf("Expected error code 'EMAIL_ALREADY_EXISTS', got %s", appErr.Code)
	}
}

func TestUpdateUserEmailHandler_Handle_ValidCommand_UpdatesEmail(t *testing.T) {
	// Arrange
	userRepo := newMockUserRepository()
	unitOfWork := newMockUnitOfWork()
	handler := NewUpdateUserEmailHandler(userRepo, unitOfWork)
	logger := &mockLogger{}

	// Create existing user
	user, _ := domain.NewUser("user-123", "john@example.com", "John Doe")
	userRepo.Save(context.Background(), user)

	cmd := UpdateUserEmailCommand{
		ID:       "user-123",
		NewEmail: "john.doe@example.com",
	}

	// Act
	err := handler.Handle(context.Background(), logger, cmd)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify email was updated
	updatedUser, err := userRepo.Load(context.Background(), cmd.ID)
	if err != nil {
		t.Fatalf("Expected to load user, got error: %v", err)
	}

	if updatedUser.Email() != cmd.NewEmail {
		t.Errorf("Expected email %s, got %s", cmd.NewEmail, updatedUser.Email())
	}
}

func TestUpdateUserEmailHandler_Handle_UserNotFound_ReturnsError(t *testing.T) {
	// Arrange
	userRepo := newMockUserRepository()
	unitOfWork := newMockUnitOfWork()
	handler := NewUpdateUserEmailHandler(userRepo, unitOfWork)
	logger := &mockLogger{}

	cmd := UpdateUserEmailCommand{
		ID:       "nonexistent-user",
		NewEmail: "john.doe@example.com",
	}

	// Act
	err := handler.Handle(context.Background(), logger, cmd)

	// Assert
	if err == nil {
		t.Fatal("Expected error for nonexistent user, got nil")
	}

	appErr, ok := err.(ApplicationError)
	if !ok {
		t.Errorf("Expected ApplicationError, got %T", err)
	}

	if appErr.Code != "USER_LOAD_FAILED" {
		t.Errorf("Expected error code 'USER_LOAD_FAILED', got %s", appErr.Code)
	}
}

func TestUpdateUserEmailHandler_Handle_EmailAlreadyInUse_ReturnsError(t *testing.T) {
	// Arrange
	userRepo := newMockUserRepository()
	unitOfWork := newMockUnitOfWork()
	handler := NewUpdateUserEmailHandler(userRepo, unitOfWork)
	logger := &mockLogger{}

	// Create two users
	user1, _ := domain.NewUser("user-1", "john@example.com", "John Doe")
	user2, _ := domain.NewUser("user-2", "jane@example.com", "Jane Doe")
	userRepo.Save(context.Background(), user1)
	userRepo.Save(context.Background(), user2)

	// Try to update user-1's email to user-2's email
	cmd := UpdateUserEmailCommand{
		ID:       "user-1",
		NewEmail: "jane@example.com",
	}

	// Act
	err := handler.Handle(context.Background(), logger, cmd)

	// Assert
	if err == nil {
		t.Fatal("Expected error for email already in use, got nil")
	}

	appErr, ok := err.(ApplicationError)
	if !ok {
		t.Errorf("Expected ApplicationError, got %T", err)
	}

	if appErr.Code != "EMAIL_ALREADY_EXISTS" {
		t.Errorf("Expected error code 'EMAIL_ALREADY_EXISTS', got %s", appErr.Code)
	}
}
