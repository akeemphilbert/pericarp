package application

import (
	"time"

	"github.com/akeemphilbert/pericarp/pkg/application"
	"github.com/segmentio/ksuid"
)

// GetUserQuery represents a query to get a single user by ID
type GetUserQuery struct {
	ID ksuid.KSUID `json:"id"`
}

// QueryType returns the query type identifier
func (q GetUserQuery) QueryType() string {
	return "GetUser"
}

// Validate validates the get user query
func (q GetUserQuery) Validate() error {
	if q.ID == ksuid.Nil {
		return application.NewValidationError("id", "ID cannot be empty")
	}
	return nil
}

// GetUserByEmailQuery represents a query to get a single user by email
type GetUserByEmailQuery struct {
	Email string `json:"email"`
}

// QueryType returns the query type identifier
func (q GetUserByEmailQuery) QueryType() string {
	return "GetUserByEmail"
}

// Validate validates the get user by email query
func (q GetUserByEmailQuery) Validate() error {
	if q.Email == "" {
		return application.NewValidationError("email", "email cannot be empty")
	}
	return nil
}

// ListUsersQuery represents a query to list users with pagination
type ListUsersQuery struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Active   *bool `json:"active,omitempty"` // Filter by active status
}

// QueryType returns the query type identifier
func (q ListUsersQuery) QueryType() string {
	return "ListUsers"
}

// Validate validates the list users query
func (q ListUsersQuery) Validate() error {
	if q.Page < 1 {
		return application.NewValidationError("page", "page must be greater than 0")
	}

	if q.PageSize < 1 {
		return application.NewValidationError("page_size", "page_size must be greater than 0")
	}

	if q.PageSize > 100 {
		return application.NewValidationError("page_size", "page_size cannot exceed 100")
	}

	return nil
}

// UserDTO represents a user data transfer object for queries
type UserDTO struct {
	ID        ksuid.KSUID `json:"id"`
	Email     string      `json:"email"`
	Name      string      `json:"name"`
	IsActive  bool        `json:"is_active"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// ListUsersResult represents the result of a list users query
type ListUsersResult struct {
	Users      []UserDTO `json:"users"`
	Page       int       `json:"page"`
	PageSize   int       `json:"page_size"`
	TotalCount int       `json:"total_count"`
	TotalPages int       `json:"total_pages"`
}
