package application

import (
	"strings"
)

// GetUserQuery represents a query to get a single user by ID
type GetUserQuery struct {
	ID string `json:"id"`
}

// QueryType returns the query type identifier
func (q GetUserQuery) QueryType() string {
	return "GetUser"
}

// Validate validates the get user query
func (q GetUserQuery) Validate() error {
	if q.ID == "" {
		return NewValidationError("id", "ID cannot be empty")
	}

	if strings.TrimSpace(q.ID) == "" {
		return NewValidationError("id", "ID cannot be whitespace only")
	}

	return nil
}

// ListUsersQuery represents a query to list users with pagination
type ListUsersQuery struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// QueryType returns the query type identifier
func (q ListUsersQuery) QueryType() string {
	return "ListUsers"
}

// Validate validates the list users query
func (q ListUsersQuery) Validate() error {
	if q.Page < 1 {
		return NewValidationError("page", "page must be greater than 0")
	}

	if q.PageSize < 1 {
		return NewValidationError("page_size", "page_size must be greater than 0")
	}

	if q.PageSize > 100 {
		return NewValidationError("page_size", "page_size cannot exceed 100")
	}

	return nil
}

// UserDTO represents a user data transfer object for queries
type UserDTO struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// ListUsersResult represents the result of a list users query
type ListUsersResult struct {
	Users      []UserDTO `json:"users"`
	Page       int       `json:"page"`
	PageSize   int       `json:"page_size"`
	TotalCount int       `json:"total_count"`
	TotalPages int       `json:"total_pages"`
}
