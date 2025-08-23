package domain

import "context"

// UserRepository defines the interface for User aggregate persistence
type UserRepository interface {
	Repository[*User]

	// FindByEmail finds a user by email address
	FindByEmail(ctx context.Context, email string) (*User, error)

	// Exists checks if a user with the given ID exists
	Exists(ctx context.Context, id string) (bool, error)

	// ExistsByEmail checks if a user with the given email exists
	ExistsByEmail(ctx context.Context, email string) (bool, error)
}
