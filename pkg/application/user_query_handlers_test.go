package application

import (
	"context"
	"testing"
)

// Mock implementation for UserReadModelRepository
type mockUserReadModelRepository struct {
	users map[string]*UserReadModel
}

func newMockUserReadModelRepository() *mockUserReadModelRepository {
	return &mockUserReadModelRepository{
		users: make(map[string]*UserReadModel),
	}
}

func (m *mockUserReadModelRepository) GetByID(ctx context.Context, id string) (*UserReadModel, error) {
	user, exists := m.users[id]
	if !exists {
		return nil, NewApplicationError("USER_NOT_FOUND", "User not found", nil)
	}
	return user, nil
}

func (m *mockUserReadModelRepository) GetByEmail(ctx context.Context, email string) (*UserReadModel, error) {
	for _, user := range m.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, NewApplicationError("USER_NOT_FOUND", "User not found", nil)
}

func (m *mockUserReadModelRepository) List(ctx context.Context, page, pageSize int) ([]UserReadModel, int, error) {
	users := make([]UserReadModel, 0, len(m.users))
	for _, user := range m.users {
		users = append(users, *user)
	}

	totalCount := len(users)

	// Simple pagination
	start := (page - 1) * pageSize
	end := start + pageSize

	if start >= len(users) {
		return []UserReadModel{}, totalCount, nil
	}

	if end > len(users) {
		end = len(users)
	}

	return users[start:end], totalCount, nil
}

func (m *mockUserReadModelRepository) Save(ctx context.Context, user *UserReadModel) error {
	m.users[user.ID] = user
	return nil
}

func (m *mockUserReadModelRepository) Delete(ctx context.Context, id string) error {
	delete(m.users, id)
	return nil
}

func (m *mockUserReadModelRepository) Count(ctx context.Context) (int, error) {
	return len(m.users), nil
}

func TestGetUserHandler_Handle_ValidQuery_ReturnsUser(t *testing.T) {
	// Arrange
	readModelRepo := newMockUserReadModelRepository()
	handler := NewGetUserHandler(readModelRepo)
	logger := &mockLogger{}

	// Add test user to repository
	testUser := &UserReadModel{
		ID:      "user-123",
		Email:   "john@example.com",
		Name:    "John Doe",
		Version: 1,
	}
	readModelRepo.Save(context.Background(), testUser)

	query := GetUserQuery{ID: "user-123"}

	// Act
	result, err := handler.Handle(context.Background(), logger, query)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.ID != testUser.ID {
		t.Errorf("Expected ID %s, got %s", testUser.ID, result.ID)
	}

	if result.Email != testUser.Email {
		t.Errorf("Expected email %s, got %s", testUser.Email, result.Email)
	}

	if result.Name != testUser.Name {
		t.Errorf("Expected name %s, got %s", testUser.Name, result.Name)
	}

	if result.Version != testUser.Version {
		t.Errorf("Expected version %d, got %d", testUser.Version, result.Version)
	}
}

func TestGetUserHandler_Handle_UserNotFound_ReturnsError(t *testing.T) {
	// Arrange
	readModelRepo := newMockUserReadModelRepository()
	handler := NewGetUserHandler(readModelRepo)
	logger := &mockLogger{}

	query := GetUserQuery{ID: "nonexistent-user"}

	// Act
	result, err := handler.Handle(context.Background(), logger, query)

	// Assert
	if err == nil {
		t.Fatal("Expected error for nonexistent user, got nil")
	}

	appErr, ok := err.(ApplicationError)
	if !ok {
		t.Errorf("Expected ApplicationError, got %T", err)
	}

	if appErr.Code != "USER_NOT_FOUND" {
		t.Errorf("Expected error code 'USER_NOT_FOUND', got %s", appErr.Code)
	}

	// Result should be empty
	if result.ID != "" {
		t.Errorf("Expected empty result, got %+v", result)
	}
}

func TestListUsersHandler_Handle_ValidQuery_ReturnsUsers(t *testing.T) {
	// Arrange
	readModelRepo := newMockUserReadModelRepository()
	handler := NewListUsersHandler(readModelRepo)
	logger := &mockLogger{}

	// Add test users to repository
	testUsers := []*UserReadModel{
		{ID: "user-1", Email: "user1@example.com", Name: "User One", Version: 1},
		{ID: "user-2", Email: "user2@example.com", Name: "User Two", Version: 1},
		{ID: "user-3", Email: "user3@example.com", Name: "User Three", Version: 1},
	}

	for _, user := range testUsers {
		readModelRepo.Save(context.Background(), user)
	}

	query := ListUsersQuery{Page: 1, PageSize: 2}

	// Act
	result, err := handler.Handle(context.Background(), logger, query)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.Page != query.Page {
		t.Errorf("Expected page %d, got %d", query.Page, result.Page)
	}

	if result.PageSize != query.PageSize {
		t.Errorf("Expected page size %d, got %d", query.PageSize, result.PageSize)
	}

	if result.TotalCount != len(testUsers) {
		t.Errorf("Expected total count %d, got %d", len(testUsers), result.TotalCount)
	}

	if len(result.Users) != query.PageSize {
		t.Errorf("Expected %d users in result, got %d", query.PageSize, len(result.Users))
	}

	expectedTotalPages := 2 // 3 users with page size 2 = 2 pages
	if result.TotalPages != expectedTotalPages {
		t.Errorf("Expected total pages %d, got %d", expectedTotalPages, result.TotalPages)
	}
}

func TestListUsersHandler_Handle_EmptyRepository_ReturnsEmptyList(t *testing.T) {
	// Arrange
	readModelRepo := newMockUserReadModelRepository()
	handler := NewListUsersHandler(readModelRepo)
	logger := &mockLogger{}

	query := ListUsersQuery{Page: 1, PageSize: 10}

	// Act
	result, err := handler.Handle(context.Background(), logger, query)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.TotalCount != 0 {
		t.Errorf("Expected total count 0, got %d", result.TotalCount)
	}

	if len(result.Users) != 0 {
		t.Errorf("Expected 0 users in result, got %d", len(result.Users))
	}

	if result.TotalPages != 0 {
		t.Errorf("Expected total pages 0, got %d", result.TotalPages)
	}
}

func TestUserReadModel_ToDTO_ConvertsCorrectly(t *testing.T) {
	// Arrange
	user := &UserReadModel{
		ID:      "user-123",
		Email:   "john@example.com",
		Name:    "John Doe",
		Version: 1,
	}

	// Act
	dto := user.ToDTO()

	// Assert
	if dto.ID != user.ID {
		t.Errorf("Expected ID %s, got %s", user.ID, dto.ID)
	}

	if dto.Email != user.Email {
		t.Errorf("Expected email %s, got %s", user.Email, dto.Email)
	}

	if dto.Name != user.Name {
		t.Errorf("Expected name %s, got %s", user.Name, dto.Name)
	}

	if dto.Version != user.Version {
		t.Errorf("Expected version %d, got %d", user.Version, dto.Version)
	}
}
