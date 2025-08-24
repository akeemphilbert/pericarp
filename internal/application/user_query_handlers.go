package application

import (
	"context"
	"math"

	pkgapp "github.com/example/pericarp/pkg/application"
	"github.com/example/pericarp/pkg/domain"
)

// GetUserHandler handles GetUserQuery
type GetUserHandler struct {
	readModelRepo UserReadModelRepository
}

// NewGetUserHandler creates a new GetUserHandler
func NewGetUserHandler(readModelRepo UserReadModelRepository) *GetUserHandler {
	return &GetUserHandler{
		readModelRepo: readModelRepo,
	}
}

// Handle processes the GetUserQuery
func (h *GetUserHandler) Handle(ctx context.Context, logger domain.Logger, query GetUserQuery) (UserDTO, error) {
	logger.Debug("Processing GetUserQuery", "id", query.ID)

	// Get user from read model
	user, err := h.readModelRepo.GetByID(ctx, query.ID)
	if err != nil {
		logger.Error("Failed to get user from read model", "id", query.ID, "error", err)
		return UserDTO{}, pkgapp.NewApplicationError("USER_NOT_FOUND", "User not found", err)
	}

	logger.Debug("User retrieved successfully", "id", query.ID, "email", user.Email)
	return user.ToDTO(), nil
}

// GetUserByEmailHandler handles GetUserByEmailQuery
type GetUserByEmailHandler struct {
	readModelRepo UserReadModelRepository
}

// NewGetUserByEmailHandler creates a new GetUserByEmailHandler
func NewGetUserByEmailHandler(readModelRepo UserReadModelRepository) *GetUserByEmailHandler {
	return &GetUserByEmailHandler{
		readModelRepo: readModelRepo,
	}
}

// Handle processes the GetUserByEmailQuery
func (h *GetUserByEmailHandler) Handle(ctx context.Context, logger domain.Logger, query GetUserByEmailQuery) (UserDTO, error) {
	logger.Debug("Processing GetUserByEmailQuery", "email", query.Email)

	// Get user from read model
	user, err := h.readModelRepo.GetByEmail(ctx, query.Email)
	if err != nil {
		logger.Error("Failed to get user from read model", "email", query.Email, "error", err)
		return UserDTO{}, pkgapp.NewApplicationError("USER_NOT_FOUND", "User not found", err)
	}

	logger.Debug("User retrieved successfully", "id", user.ID, "email", user.Email)
	return user.ToDTO(), nil
}

// ListUsersHandler handles ListUsersQuery
type ListUsersHandler struct {
	readModelRepo UserReadModelRepository
}

// NewListUsersHandler creates a new ListUsersHandler
func NewListUsersHandler(readModelRepo UserReadModelRepository) *ListUsersHandler {
	return &ListUsersHandler{
		readModelRepo: readModelRepo,
	}
}

// Handle processes the ListUsersQuery
func (h *ListUsersHandler) Handle(ctx context.Context, logger domain.Logger, query ListUsersQuery) (ListUsersResult, error) {
	logger.Debug("Processing ListUsersQuery", "page", query.Page, "page_size", query.PageSize, "active", query.Active)

	// Get users from read model
	users, totalCount, err := h.readModelRepo.List(ctx, query.Page, query.PageSize, query.Active)
	if err != nil {
		logger.Error("Failed to list users from read model", "page", query.Page, "page_size", query.PageSize, "error", err)
		return ListUsersResult{}, pkgapp.NewApplicationError("USER_LIST_FAILED", "Failed to list users", err)
	}

	// Convert to DTOs
	userDTOs := make([]UserDTO, len(users))
	for i, user := range users {
		userDTOs[i] = user.ToDTO()
	}

	// Calculate total pages
	totalPages := int(math.Ceil(float64(totalCount) / float64(query.PageSize)))

	result := ListUsersResult{
		Users:      userDTOs,
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalCount: totalCount,
		TotalPages: totalPages,
	}

	logger.Debug("Users listed successfully", "count", len(userDTOs), "total_count", totalCount, "total_pages", totalPages)
	return result, nil
}
