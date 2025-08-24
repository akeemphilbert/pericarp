package application

import (
	"regexp"
	"strings"

	"github.com/example/pericarp/pkg/application"
	"github.com/google/uuid"
)

// CreateUserCommand represents a command to create a new user
type CreateUserCommand struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

// CommandType returns the command type identifier
func (c CreateUserCommand) CommandType() string {
	return "CreateUser"
}

// Validate validates the create user command
func (c CreateUserCommand) Validate() error {
	if c.ID == uuid.Nil {
		return application.NewValidationError("id", "ID cannot be empty")
	}

	if err := validateEmail(c.Email); err != nil {
		return err
	}

	if err := validateName(c.Name); err != nil {
		return err
	}

	return nil
}

// UpdateUserEmailCommand represents a command to update a user's email
type UpdateUserEmailCommand struct {
	ID       uuid.UUID `json:"id"`
	NewEmail string    `json:"new_email"`
}

// CommandType returns the command type identifier
func (c UpdateUserEmailCommand) CommandType() string {
	return "UpdateUserEmail"
}

// Validate validates the update user email command
func (c UpdateUserEmailCommand) Validate() error {
	if c.ID == uuid.Nil {
		return application.NewValidationError("id", "ID cannot be empty")
	}

	if err := validateEmail(c.NewEmail); err != nil {
		return application.NewValidationError("new_email", err.Error())
	}

	return nil
}

// UpdateUserNameCommand represents a command to update a user's name
type UpdateUserNameCommand struct {
	ID      uuid.UUID `json:"id"`
	NewName string    `json:"new_name"`
}

// CommandType returns the command type identifier
func (c UpdateUserNameCommand) CommandType() string {
	return "UpdateUserName"
}

// Validate validates the update user name command
func (c UpdateUserNameCommand) Validate() error {
	if c.ID == uuid.Nil {
		return application.NewValidationError("id", "ID cannot be empty")
	}

	if err := validateName(c.NewName); err != nil {
		return application.NewValidationError("new_name", err.Error())
	}

	return nil
}

// DeactivateUserCommand represents a command to deactivate a user
type DeactivateUserCommand struct {
	ID uuid.UUID `json:"id"`
}

// CommandType returns the command type identifier
func (c DeactivateUserCommand) CommandType() string {
	return "DeactivateUser"
}

// Validate validates the deactivate user command
func (c DeactivateUserCommand) Validate() error {
	if c.ID == uuid.Nil {
		return application.NewValidationError("id", "ID cannot be empty")
	}
	return nil
}

// ActivateUserCommand represents a command to activate a user
type ActivateUserCommand struct {
	ID uuid.UUID `json:"id"`
}

// CommandType returns the command type identifier
func (c ActivateUserCommand) CommandType() string {
	return "ActivateUser"
}

// Validate validates the activate user command
func (c ActivateUserCommand) Validate() error {
	if c.ID == uuid.Nil {
		return application.NewValidationError("id", "ID cannot be empty")
	}
	return nil
}

// validateEmail validates email format and business rules
func validateEmail(email string) error {
	if email == "" {
		return application.NewValidationError("email", "email cannot be empty")
	}

	email = strings.TrimSpace(email)
	if len(email) > 254 {
		return application.NewValidationError("email", "email cannot exceed 254 characters")
	}

	// Basic email regex validation
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return application.NewValidationError("email", "invalid email format")
	}

	return nil
}

// validateName validates name format and business rules
func validateName(name string) error {
	if name == "" {
		return application.NewValidationError("name", "name cannot be empty")
	}

	name = strings.TrimSpace(name)
	if len(name) < 2 {
		return application.NewValidationError("name", "name must be at least 2 characters long")
	}

	if len(name) > 100 {
		return application.NewValidationError("name", "name cannot exceed 100 characters")
	}

	return nil
}
