package application

import "context"

// UserReadModel represents a user in the read model optimized for queries
type UserReadModel struct {
	ID      string
	Email   string
	Name    string
	Version int
}

// UserReadModelRepository defines the interface for querying user read models
type UserReadModelRepository interface {
	// GetByID retrieves a user read model by ID
	GetByID(ctx context.Context, id string) (*UserReadModel, error)

	// GetByEmail retrieves a user read model by email
	GetByEmail(ctx context.Context, email string) (*UserReadModel, error)

	// List retrieves a paginated list of user read models
	List(ctx context.Context, page, pageSize int) ([]UserReadModel, int, error)

	// Save saves or updates a user read model
	Save(ctx context.Context, user *UserReadModel) error

	// Delete removes a user read model
	Delete(ctx context.Context, id string) error

	// Count returns the total number of users
	Count(ctx context.Context) (int, error)
}

// ToDTO converts a UserReadModel to a UserDTO
func (u *UserReadModel) ToDTO() UserDTO {
	return UserDTO{
		ID:      u.ID,
		Email:   u.Email,
		Name:    u.Name,
		Version: u.Version,
	}
}
