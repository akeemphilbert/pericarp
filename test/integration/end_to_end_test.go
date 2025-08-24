//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	internalapp "github.com/example/pericarp/internal/application"
	internaldomain "github.com/example/pericarp/internal/domain"
	internalinfra "github.com/example/pericarp/internal/infrastructure"
	pkgapp "github.com/example/pericarp/pkg/application"
	pkgdomain "github.com/example/pericarp/pkg/domain"
	pkginfra "github.com/example/pericarp/pkg/infrastructure"
)

// TestEndToEndFlow tests complete command and query flows
func TestEndToEndFlow(t *testing.T) {
	tests := []struct {
		name     string
		dbDriver string
		dsn      string
	}{
		{
			name:     "SQLite",
			dbDriver: "sqlite",
			dsn:      ":memory:",
		},
	}

	// Add PostgreSQL test if available
	if postgresURL := os.Getenv("POSTGRES_TEST_DSN"); postgresURL != "" {
		tests = append(tests, struct {
			name     string
			dbDriver string
			dsn      string
		}{
			name:     "PostgreSQL",
			dbDriver: "postgres",
			dsn:      postgresURL,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testEndToEndWithDatabase(t, tt.dbDriver, tt.dsn)
		})
	}
}

func testEndToEndWithDatabase(t *testing.T, driver, dsn string) {
	// Setup system
	system, cleanup := setupSystem(t, driver, dsn)
	defer cleanup()

	t.Run("UserLifecycle", func(t *testing.T) {
		testUserLifecycle(t, system)
	})

	t.Run("ConcurrentOperations", func(t *testing.T) {
		testConcurrentOperations(t, system)
	})

	t.Run("EventSourcingConsistency", func(t *testing.T) {
		testEventSourcingConsistency(t, system)
	})

	t.Run("ReadModelConsistency", func(t *testing.T) {
		testReadModelConsistency(t, system)
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		testErrorHandling(t, system)
	})
}

// TestSystem encapsulates all system components
type TestSystem struct {
	DB                    *gorm.DB
	Logger                pkgdomain.Logger
	EventStore            pkgdomain.EventStore
	EventDispatcher       pkgdomain.EventDispatcher
	UnitOfWork            pkgdomain.UnitOfWork
	UserRepo              internaldomain.UserRepository
	UserReadModelRepo     internalapp.UserReadModelRepository
	CreateUserHandler     *internalapp.CreateUserHandler
	UpdateUserEmailHandler *internalapp.UpdateUserEmailHandler
	UpdateUserNameHandler  *internalapp.UpdateUserNameHandler
	DeactivateUserHandler  *internalapp.DeactivateUserHandler
	ActivateUserHandler    *internalapp.ActivateUserHandler
	GetUserHandler         *internalapp.GetUserHandler
	GetUserByEmailHandler  *internalapp.GetUserByEmailHandler
	ListUsersHandler       *internalapp.ListUsersHandler
	UserProjector          *internalapp.UserProjector
}

func setupSystem(t *testing.T, driver, dsn string) (*TestSystem, func()) {
	// Setup database
	var db *gorm.DB
	var err error

	switch driver {
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	case "postgres":
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	default:
		t.Fatalf("unsupported database driver: %s", driver)
	}

	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	// Auto-migrate tables
	if err := db.AutoMigrate(&pkginfra.EventRecord{}, &internalinfra.UserReadModelGORM{}); err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}

	// Initialize components
	logger := pkginfra.NewLogger()
	eventStore := pkginfra.NewEventStore(db)
	eventDispatcher := pkginfra.NewEventDispatcher()
	unitOfWork := pkginfra.NewUnitOfWork(eventStore, eventDispatcher)

	// Initialize repositories
	userRepo := internalinfra.NewUserRepositoryEventSourcing(eventStore)
	userReadModelRepo := internalinfra.NewUserReadModelGORM(db)

	// Initialize command handlers
	createUserHandler := internalapp.NewCreateUserHandler(userRepo, unitOfWork)
	updateUserEmailHandler := internalapp.NewUpdateUserEmailHandler(userRepo, unitOfWork)
	updateUserNameHandler := internalapp.NewUpdateUserNameHandler(userRepo, unitOfWork)
	deactivateUserHandler := internalapp.NewDeactivateUserHandler(userRepo, unitOfWork)
	activateUserHandler := internalapp.NewActivateUserHandler(userRepo, unitOfWork)

	// Initialize query handlers
	getUserHandler := internalapp.NewGetUserHandler(userReadModelRepo)
	getUserByEmailHandler := internalapp.NewGetUserByEmailHandler(userReadModelRepo)
	listUsersHandler := internalapp.NewListUsersHandler(userReadModelRepo)

	// Initialize projector
	userProjector := internalapp.NewUserProjector(userReadModelRepo)

	// Subscribe projector to events
	eventDispatcher.Subscribe("UserCreated", userProjector)
	eventDispatcher.Subscribe("UserEmailUpdated", userProjector)
	eventDispatcher.Subscribe("UserNameUpdated", userProjector)
	eventDispatcher.Subscribe("UserDeactivated", userProjector)
	eventDispatcher.Subscribe("UserActivated", userProjector)

	system := &TestSystem{
		DB:                     db,
		Logger:                 logger,
		EventStore:             eventStore,
		EventDispatcher:        eventDispatcher,
		UnitOfWork:             unitOfWork,
		UserRepo:               userRepo,
		UserReadModelRepo:      userReadModelRepo,
		CreateUserHandler:      createUserHandler,
		UpdateUserEmailHandler: updateUserEmailHandler,
		UpdateUserNameHandler:  updateUserNameHandler,
		DeactivateUserHandler:  deactivateUserHandler,
		ActivateUserHandler:    activateUserHandler,
		GetUserHandler:         getUserHandler,
		GetUserByEmailHandler:  getUserByEmailHandler,
		ListUsersHandler:       listUsersHandler,
		UserProjector:          userProjector,
	}

	cleanup := func() {
		if driver == "sqlite" && dsn != ":memory:" {
			if err := os.Remove(dsn); err != nil {
				t.Logf("Warning: Failed to cleanup test database file %s: %v", dsn, err)
			}
		}
	}

	return system, cleanup
}

func testUserLifecycle(t *testing.T, system *TestSystem) {
	ctx := context.Background()

	// 1. Create user
	userID := uuid.New()
	createCmd := internalapp.CreateUserCommand{
		ID:    userID,
		Email: "lifecycle@example.com",
		Name:  "Lifecycle User",
	}

	err := system.CreateUserHandler.Handle(ctx, system.Logger, createCmd)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Wait for projection
	time.Sleep(100 * time.Millisecond)

	// 2. Query user by ID
	getUserQuery := internalapp.GetUserQuery{ID: userID}
	userDTO, err := system.GetUserHandler.Handle(ctx, system.Logger, getUserQuery)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}

	if userDTO.ID != userID {
		t.Errorf("expected user ID %s, got %s", userID, userDTO.ID)
	}
	if userDTO.Email != "lifecycle@example.com" {
		t.Errorf("expected email lifecycle@example.com, got %s", userDTO.Email)
	}
	if userDTO.Name != "Lifecycle User" {
		t.Errorf("expected name 'Lifecycle User', got %s", userDTO.Name)
	}
	if !userDTO.IsActive {
		t.Error("expected user to be active")
	}

	// 3. Query user by email
	getUserByEmailQuery := internalapp.GetUserByEmailQuery{Email: "lifecycle@example.com"}
	userDTO2, err := system.GetUserByEmailHandler.Handle(ctx, system.Logger, getUserByEmailQuery)
	if err != nil {
		t.Fatalf("failed to get user by email: %v", err)
	}

	if userDTO2.ID != userID {
		t.Errorf("expected user ID %s, got %s", userID, userDTO2.ID)
	}

	// 4. Update user email
	updateEmailCmd := internalapp.UpdateUserEmailCommand{
		ID:       userID,
		NewEmail: "updated@example.com",
	}

	err = system.UpdateUserEmailHandler.Handle(ctx, system.Logger, updateEmailCmd)
	if err != nil {
		t.Fatalf("failed to update user email: %v", err)
	}

	// Wait for projection
	time.Sleep(100 * time.Millisecond)

	// 5. Verify email update
	userDTO3, err := system.GetUserHandler.Handle(ctx, system.Logger, getUserQuery)
	if err != nil {
		t.Fatalf("failed to get user after email update: %v", err)
	}

	if userDTO3.Email != "updated@example.com" {
		t.Errorf("expected updated email 'updated@example.com', got %s", userDTO3.Email)
	}

	// 6. Update user name
	updateNameCmd := internalapp.UpdateUserNameCommand{
		ID:      userID,
		NewName: "Updated User",
	}

	err = system.UpdateUserNameHandler.Handle(ctx, system.Logger, updateNameCmd)
	if err != nil {
		t.Fatalf("failed to update user name: %v", err)
	}

	// Wait for projection
	time.Sleep(100 * time.Millisecond)

	// 7. Verify name update
	userDTO4, err := system.GetUserHandler.Handle(ctx, system.Logger, getUserQuery)
	if err != nil {
		t.Fatalf("failed to get user after name update: %v", err)
	}

	if userDTO4.Name != "Updated User" {
		t.Errorf("expected updated name 'Updated User', got %s", userDTO4.Name)
	}

	// 8. Deactivate user
	deactivateCmd := internalapp.DeactivateUserCommand{ID: userID}
	err = system.DeactivateUserHandler.Handle(ctx, system.Logger, deactivateCmd)
	if err != nil {
		t.Fatalf("failed to deactivate user: %v", err)
	}

	// Wait for projection
	time.Sleep(100 * time.Millisecond)

	// 9. Verify deactivation
	userDTO5, err := system.GetUserHandler.Handle(ctx, system.Logger, getUserQuery)
	if err != nil {
		t.Fatalf("failed to get user after deactivation: %v", err)
	}

	if userDTO5.IsActive {
		t.Error("expected user to be inactive")
	}

	// 10. Activate user
	activateCmd := internalapp.ActivateUserCommand{ID: userID}
	err = system.ActivateUserHandler.Handle(ctx, system.Logger, activateCmd)
	if err != nil {
		t.Fatalf("failed to activate user: %v", err)
	}

	// Wait for projection
	time.Sleep(100 * time.Millisecond)

	// 11. Verify activation
	userDTO6, err := system.GetUserHandler.Handle(ctx, system.Logger, getUserQuery)
	if err != nil {
		t.Fatalf("failed to get user after activation: %v", err)
	}

	if !userDTO6.IsActive {
		t.Error("expected user to be active")
	}

	// 12. Verify event sourcing - reconstruct from events
	envelopes, err := system.EventStore.Load(ctx, userID.String())
	if err != nil {
		t.Fatalf("failed to load events: %v", err)
	}

	expectedEventCount := 5 // Created, EmailUpdated, NameUpdated, Deactivated, Activated
	if len(envelopes) != expectedEventCount {
		t.Errorf("expected %d events, got %d", expectedEventCount, len(envelopes))
	}

	// Reconstruct user from events
	events := make([]pkgdomain.Event, len(envelopes))
	for i, envelope := range envelopes {
		events[i] = envelope.Event()
	}

	reconstructedUser := &internaldomain.User{}
	reconstructedUser.LoadFromHistory(events)

	if reconstructedUser.Email() != "updated@example.com" {
		t.Errorf("reconstructed user email: expected 'updated@example.com', got %s", reconstructedUser.Email())
	}
	if reconstructedUser.Name() != "Updated User" {
		t.Errorf("reconstructed user name: expected 'Updated User', got %s", reconstructedUser.Name())
	}
	if !reconstructedUser.IsActive() {
		t.Error("reconstructed user should be active")
	}
	if reconstructedUser.Version() != 5 {
		t.Errorf("reconstructed user version: expected 5, got %d", reconstructedUser.Version())
	}
}

func testConcurrentOperations(t *testing.T, system *TestSystem) {
	ctx := context.Background()
	numUsers := 50

	// Create users concurrently
	errChan := make(chan error, numUsers)
	userIDs := make([]uuid.UUID, numUsers)

	for i := 0; i < numUsers; i++ {
		userIDs[i] = uuid.New()
		go func(index int) {
			cmd := internalapp.CreateUserCommand{
				ID:    userIDs[index],
				Email: fmt.Sprintf("concurrent%d@example.com", index),
				Name:  fmt.Sprintf("Concurrent User %d", index),
			}
			errChan <- system.CreateUserHandler.Handle(ctx, system.Logger, cmd)
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < numUsers; i++ {
		if err := <-errChan; err != nil {
			t.Fatalf("concurrent user creation failed: %v", err)
		}
	}

	// Wait for projections
	time.Sleep(500 * time.Millisecond)

	// Verify all users were created
	listQuery := internalapp.ListUsersQuery{
		Page:     1,
		PageSize: numUsers + 10, // Extra buffer
	}

	result, err := system.ListUsersHandler.Handle(ctx, system.Logger, listQuery)
	if err != nil {
		t.Fatalf("failed to list users: %v", err)
	}

	if result.TotalCount < numUsers {
		t.Errorf("expected at least %d users, got %d", numUsers, result.TotalCount)
	}

	// Verify data integrity
	for _, userID := range userIDs {
		query := internalapp.GetUserQuery{ID: userID}
		userDTO, err := system.GetUserHandler.Handle(ctx, system.Logger, query)
		if err != nil {
			t.Errorf("failed to get user %s: %v", userID, err)
			continue
		}

		if userDTO.ID != userID {
			t.Errorf("user ID mismatch: expected %s, got %s", userID, userDTO.ID)
		}
	}
}

func testEventSourcingConsistency(t *testing.T, system *TestSystem) {
	ctx := context.Background()

	// Create user with multiple operations
	userID := uuid.New()
	operations := []func() error{
		func() error {
			cmd := internalapp.CreateUserCommand{
				ID:    userID,
				Email: "consistency@example.com",
				Name:  "Consistency User",
			}
			return system.CreateUserHandler.Handle(ctx, system.Logger, cmd)
		},
		func() error {
			cmd := internalapp.UpdateUserEmailCommand{
				ID:       userID,
				NewEmail: "updated1@example.com",
			}
			return system.UpdateUserEmailHandler.Handle(ctx, system.Logger, cmd)
		},
		func() error {
			cmd := internalapp.UpdateUserEmailCommand{
				ID:       userID,
				NewEmail: "updated2@example.com",
			}
			return system.UpdateUserEmailHandler.Handle(ctx, system.Logger, cmd)
		},
		func() error {
			cmd := internalapp.UpdateUserNameCommand{
				ID:      userID,
				NewName: "Updated Name",
			}
			return system.UpdateUserNameHandler.Handle(ctx, system.Logger, cmd)
		},
		func() error {
			cmd := internalapp.DeactivateUserCommand{ID: userID}
			return system.DeactivateUserHandler.Handle(ctx, system.Logger, cmd)
		},
		func() error {
			cmd := internalapp.ActivateUserCommand{ID: userID}
			return system.ActivateUserHandler.Handle(ctx, system.Logger, cmd)
		},
	}

	// Execute operations sequentially
	for i, op := range operations {
		if err := op(); err != nil {
			t.Fatalf("operation %d failed: %v", i, err)
		}
	}

	// Wait for projections
	time.Sleep(200 * time.Millisecond)

	// Load events and verify consistency
	envelopes, err := system.EventStore.Load(ctx, userID.String())
	if err != nil {
		t.Fatalf("failed to load events: %v", err)
	}

	if len(envelopes) != len(operations) {
		t.Errorf("expected %d events, got %d", len(operations), len(envelopes))
	}

	// Verify event ordering
	for i := 1; i < len(envelopes); i++ {
		prevVersion := envelopes[i-1].Event().Version()
		currVersion := envelopes[i].Event().Version()

		if currVersion != prevVersion+1 {
			t.Errorf("event version gap: event %d has version %d, event %d has version %d",
				i-1, prevVersion, i, currVersion)
		}

		prevTime := envelopes[i-1].Timestamp()
		currTime := envelopes[i].Timestamp()

		if currTime.Before(prevTime) {
			t.Errorf("event timestamp ordering violation: event %d at %v, event %d at %v",
				i-1, prevTime, i, currTime)
		}
	}

	// Verify final state consistency
	query := internalapp.GetUserQuery{ID: userID}
	userDTO, err := system.GetUserHandler.Handle(ctx, system.Logger, query)
	if err != nil {
		t.Fatalf("failed to get final user state: %v", err)
	}

	if userDTO.Email != "updated2@example.com" {
		t.Errorf("final email: expected 'updated2@example.com', got %s", userDTO.Email)
	}
	if userDTO.Name != "Updated Name" {
		t.Errorf("final name: expected 'Updated Name', got %s", userDTO.Name)
	}
	if !userDTO.IsActive {
		t.Error("final state: expected user to be active")
	}

	// Reconstruct and verify consistency
	events := make([]pkgdomain.Event, len(envelopes))
	for i, envelope := range envelopes {
		events[i] = envelope.Event()
	}

	reconstructedUser := &internaldomain.User{}
	reconstructedUser.LoadFromHistory(events)

	if reconstructedUser.Email() != userDTO.Email {
		t.Errorf("reconstruction inconsistency: email %s vs %s", reconstructedUser.Email(), userDTO.Email)
	}
	if reconstructedUser.Name() != userDTO.Name {
		t.Errorf("reconstruction inconsistency: name %s vs %s", reconstructedUser.Name(), userDTO.Name)
	}
	if reconstructedUser.IsActive() != userDTO.IsActive {
		t.Errorf("reconstruction inconsistency: active %v vs %v", reconstructedUser.IsActive(), userDTO.IsActive)
	}
}

func testReadModelConsistency(t *testing.T, system *TestSystem) {
	ctx := context.Background()

	// Create multiple users
	numUsers := 10
	userIDs := make([]uuid.UUID, numUsers)

	for i := 0; i < numUsers; i++ {
		userIDs[i] = uuid.New()
		cmd := internalapp.CreateUserCommand{
			ID:    userIDs[i],
			Email: fmt.Sprintf("readmodel%d@example.com", i),
			Name:  fmt.Sprintf("ReadModel User %d", i),
		}

		err := system.CreateUserHandler.Handle(ctx, system.Logger, cmd)
		if err != nil {
			t.Fatalf("failed to create user %d: %v", i, err)
		}
	}

	// Wait for projections
	time.Sleep(300 * time.Millisecond)

	// Verify read model consistency
	listQuery := internalapp.ListUsersQuery{
		Page:     1,
		PageSize: numUsers,
	}

	result, err := system.ListUsersHandler.Handle(ctx, system.Logger, listQuery)
	if err != nil {
		t.Fatalf("failed to list users: %v", err)
	}

	if len(result.Users) < numUsers {
		t.Errorf("expected at least %d users in read model, got %d", numUsers, len(result.Users))
	}

	// Verify each user exists in both event store and read model
	for _, userID := range userIDs {
		// Check event store
		envelopes, err := system.EventStore.Load(ctx, userID.String())
		if err != nil {
			t.Errorf("failed to load events for user %s: %v", userID, err)
			continue
		}

		if len(envelopes) == 0 {
			t.Errorf("no events found for user %s", userID)
			continue
		}

		// Check read model
		query := internalapp.GetUserQuery{ID: userID}
		userDTO, err := system.GetUserHandler.Handle(ctx, system.Logger, query)
		if err != nil {
			t.Errorf("failed to get user %s from read model: %v", userID, err)
			continue
		}

		// Verify consistency between event store and read model
		events := make([]pkgdomain.Event, len(envelopes))
		for i, envelope := range envelopes {
			events[i] = envelope.Event()
		}

		reconstructedUser := &internaldomain.User{}
		reconstructedUser.LoadFromHistory(events)

		if reconstructedUser.Email() != userDTO.Email {
			t.Errorf("user %s email inconsistency: event store %s, read model %s",
				userID, reconstructedUser.Email(), userDTO.Email)
		}
		if reconstructedUser.Name() != userDTO.Name {
			t.Errorf("user %s name inconsistency: event store %s, read model %s",
				userID, reconstructedUser.Name(), userDTO.Name)
		}
		if reconstructedUser.IsActive() != userDTO.IsActive {
			t.Errorf("user %s active status inconsistency: event store %v, read model %v",
				userID, reconstructedUser.IsActive(), userDTO.IsActive)
		}
	}
}

func testErrorHandling(t *testing.T, system *TestSystem) {
	ctx := context.Background()

	// Test duplicate email creation
	userID1 := uuid.New()
	userID2 := uuid.New()

	cmd1 := internalapp.CreateUserCommand{
		ID:    userID1,
		Email: "duplicate@example.com",
		Name:  "User 1",
	}

	err := system.CreateUserHandler.Handle(ctx, system.Logger, cmd1)
	if err != nil {
		t.Fatalf("failed to create first user: %v", err)
	}

	cmd2 := internalapp.CreateUserCommand{
		ID:    userID2,
		Email: "duplicate@example.com", // Same email
		Name:  "User 2",
	}

	err = system.CreateUserHandler.Handle(ctx, system.Logger, cmd2)
	if err == nil {
		t.Error("expected error when creating user with duplicate email")
	}

	// Test updating non-existent user
	nonExistentID := uuid.New()
	updateCmd := internalapp.UpdateUserEmailCommand{
		ID:       nonExistentID,
		NewEmail: "new@example.com",
	}

	err = system.UpdateUserEmailHandler.Handle(ctx, system.Logger, updateCmd)
	if err == nil {
		t.Error("expected error when updating non-existent user")
	}

	// Test querying non-existent user
	query := internalapp.GetUserQuery{ID: nonExistentID}
	_, err = system.GetUserHandler.Handle(ctx, system.Logger, query)
	if err == nil {
		t.Error("expected error when querying non-existent user")
	}

	// Verify system remains consistent after errors
	listQuery := internalapp.ListUsersQuery{
		Page:     1,
		PageSize: 10,
	}

	result, err := system.ListUsersHandler.Handle(ctx, system.Logger, listQuery)
	if err != nil {
		t.Fatalf("system inconsistent after errors: %v", err)
	}

	// Should have only the first user
	foundUser := false
	for _, user := range result.Users {
		if user.ID == userID1 {
			foundUser = true
			break
		}
		if user.ID == userID2 {
			t.Error("second user should not exist due to duplicate email error")
		}
	}

	if !foundUser {
		t.Error("first user should still exist after error scenarios")
	}
}