package application

import (
	"regexp"
	"strings"
)

// CreateUserCommand represents a command to create a new user
type CreateUserCommand struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// CommandType returns the command type identifier
func (c CreateUserCommand) CommandType() string {
	return "CreateUser"
}

// Validate validates the create user command
func (c CreateUserCommand) Validate() error {
	if c.ID == "" {
		return NewValidationError("id", "ID cannot be empty")
	}

	if strings.TrimSpace(c.ID) == "" {
		return NewValidationError("id", "ID cannot be whitespace only")
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
	ID       string `json:"id"`
	NewEmail string `json:"new_email"`
}

// CommandType returns the command type identifier
func (c UpdateUserEmailCommand) CommandType() string {
	return "UpdateUserEmail"
}

// Validate validates the update user email command
func (c UpdateUserEmailCommand) Validate() error {
	if c.ID == "" {
		return NewValidationError("id", "ID cannot be empty")
	}

	if strings.TrimSpace(c.ID) == "" {
		return NewValidationError("id", "ID cannot be whitespace only")
	}

	if err := validateEmail(c.NewEmail); err != nil {
		return NewValidationError("new_email", err.Error())
	}

	return nil
}

// validateEmail validates email format and business rules
func validateEmail(email string) error {
	if email == "" {
		return NewValidationError("email", "email cannot be empty")
	}

	email = strings.TrimSpace(email)
	if len(email) > 254 {
		return NewValidationError("email", "email cannot exceed 254 characters")
	}

	// Basic email regex validation
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return NewValidationError("email", "invalid email format")
	}

	return nil
}

// validateName validates name format and business rules
func validateName(name string) error {
	if name == "" {
		return NewValidationError("name", "name cannot be empty")
	}

	name = strings.TrimSpace(name)
	if len(name) < 2 {
		return NewValidationError("name", "name must be at least 2 characters long")
	}

	if len(name) > 100 {
		return NewValidationError("name", "name cannot exceed 100 characters")
	}

	return nil
}
