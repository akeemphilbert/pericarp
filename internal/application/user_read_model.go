package application

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// UserReadModel represents a user in the read model optimized for queries
type UserReadModel struct {
	ID        uuid.UUID
	Email     string
	Name      string
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserReadModelRepository defines the interface for querying user read models
type UserReadModelRepository interface {
	// GetByID retrieves a user read model by ID
	GetByID(ctx context.Context, id uuid.UUID) (*UserReadModel, error)

	// GetByEmail retrieves a user read model by email
	GetByEmail(ctx context.Context, email string) (*UserReadModel, error)

	// List retrieves a paginated list of user read models with optional active filter
	List(ctx context.Context, page, pageSize int, active *bool) ([]UserReadModel, int, error)

	// Save saves or updates a user read model
	Save(ctx context.Context, user *UserReadModel) error

	// Delete removes a user read model
	Delete(ctx context.Context, id uuid.UUID) error

	// Count returns the total number of users with optional active filter
	Count(ctx context.Context, active *bool) (int, error)
}

// ToDTO converts a UserReadModel to a UserDTO
func (u *UserReadModel) ToDTO() UserDTO {
	return UserDTO{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		IsActive:  u.IsActive,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
