package domain

import (
	"errors"
	"testing"
)

// Example value objects for testing

// Email represents an email address value object
type Email struct {
	BaseValueObject
	value string
}

// NewEmail creates a new Email value object
func NewEmail(value string) (*Email, error) {
	email := &Email{value: value}
	if err := email.Validate(); err != nil {
		return nil, err
	}
	return email, nil
}

// Value returns the email string value
func (e *Email) Value() string {
	return e.value
}

// Equals compares this email with another value object
func (e *Email) Equals(other ValueObject) bool {
	if otherEmail, ok := other.(*Email); ok {
		return e.value == otherEmail.value
	}
	return false
}

// Validate ensures the email is in a valid format
func (e *Email) Validate() error {
	if e.value == "" {
		return errors.New("email cannot be empty")
	}

	// Simple email validation for testing
	if len(e.value) < 3 || !contains(e.value, "@") {
		return errors.New("invalid email format")
	}

	return nil
}

// Name represents a person's name value object
type Name struct {
	BaseValueObject
	firstName string
	lastName  string
}

// NewName creates a new Name value object
func NewName(firstName, lastName string) (*Name, error) {
	name := &Name{firstName: firstName, lastName: lastName}
	if err := name.Validate(); err != nil {
		return nil, err
	}
	return name, nil
}

// FirstName returns the first name
func (n *Name) FirstName() string {
	return n.firstName
}

// LastName returns the last name
func (n *Name) LastName() string {
	return n.lastName
}

// FullName returns the full name
func (n *Name) FullName() string {
	return n.firstName + " " + n.lastName
}

// Equals compares this name with another value object
func (n *Name) Equals(other ValueObject) bool {
	if otherName, ok := other.(*Name); ok {
		return n.firstName == otherName.firstName && n.lastName == otherName.lastName
	}
	return false
}

// Validate ensures the name is valid
func (n *Name) Validate() error {
	if n.firstName == "" {
		return errors.New("first name cannot be empty")
	}
	if n.lastName == "" {
		return errors.New("last name cannot be empty")
	}
	if len(n.firstName) > 50 {
		return errors.New("first name cannot exceed 50 characters")
	}
	if len(n.lastName) > 50 {
		return errors.New("last name cannot exceed 50 characters")
	}
	return nil
}

// Helper function for simple string contains check
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Tests

func TestBaseValueObject_Validate(t *testing.T) {
	bvo := BaseValueObject{}
	err := bvo.Validate()
	if err != nil {
		t.Errorf("BaseValueObject.Validate() should return nil, got %v", err)
	}
}

func TestEmail_NewEmail(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid email",
			value:       "john@example.com",
			expectError: false,
		},
		{
			name:        "empty email should fail",
			value:       "",
			expectError: true,
			errorMsg:    "email cannot be empty",
		},
		{
			name:        "invalid email format should fail",
			value:       "invalid",
			expectError: true,
			errorMsg:    "invalid email format",
		},
		{
			name:        "email without @ should fail",
			value:       "john.example.com",
			expectError: true,
			errorMsg:    "invalid email format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, err := NewEmail(tt.value)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error message '%s', got '%s'", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if email.Value() != tt.value {
				t.Errorf("expected value '%s', got '%s'", tt.value, email.Value())
			}
		})
	}
}

func TestEmail_Equals(t *testing.T) {
	email1, _ := NewEmail("john@example.com")
	email2, _ := NewEmail("john@example.com")
	email3, _ := NewEmail("jane@example.com")

	t.Run("same emails should be equal", func(t *testing.T) {
		if !email1.Equals(email2) {
			t.Errorf("emails with same value should be equal")
		}
	})

	t.Run("different emails should not be equal", func(t *testing.T) {
		if email1.Equals(email3) {
			t.Errorf("emails with different values should not be equal")
		}
	})

	t.Run("email should not equal different type", func(t *testing.T) {
		name, _ := NewName("John", "Doe")
		if email1.Equals(name) {
			t.Errorf("email should not equal name value object")
		}
	})
}

func TestName_NewName(t *testing.T) {
	tests := []struct {
		name        string
		firstName   string
		lastName    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid name",
			firstName:   "John",
			lastName:    "Doe",
			expectError: false,
		},
		{
			name:        "empty first name should fail",
			firstName:   "",
			lastName:    "Doe",
			expectError: true,
			errorMsg:    "first name cannot be empty",
		},
		{
			name:        "empty last name should fail",
			firstName:   "John",
			lastName:    "",
			expectError: true,
			errorMsg:    "last name cannot be empty",
		},
		{
			name:        "first name too long should fail",
			firstName:   "ThisIsAVeryLongFirstNameThatExceedsFiftyCharactersLimit",
			lastName:    "Doe",
			expectError: true,
			errorMsg:    "first name cannot exceed 50 characters",
		},
		{
			name:        "last name too long should fail",
			firstName:   "John",
			lastName:    "ThisIsAVeryLongLastNameThatExceedsFiftyCharactersLimit",
			expectError: true,
			errorMsg:    "last name cannot exceed 50 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, err := NewName(tt.firstName, tt.lastName)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error message '%s', got '%s'", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if name.FirstName() != tt.firstName {
				t.Errorf("expected first name '%s', got '%s'", tt.firstName, name.FirstName())
			}

			if name.LastName() != tt.lastName {
				t.Errorf("expected last name '%s', got '%s'", tt.lastName, name.LastName())
			}

			expectedFullName := tt.firstName + " " + tt.lastName
			if name.FullName() != expectedFullName {
				t.Errorf("expected full name '%s', got '%s'", expectedFullName, name.FullName())
			}
		})
	}
}

func TestName_Equals(t *testing.T) {
	name1, _ := NewName("John", "Doe")
	name2, _ := NewName("John", "Doe")
	name3, _ := NewName("Jane", "Doe")
	name4, _ := NewName("John", "Smith")

	t.Run("same names should be equal", func(t *testing.T) {
		if !name1.Equals(name2) {
			t.Errorf("names with same values should be equal")
		}
	})

	t.Run("different first names should not be equal", func(t *testing.T) {
		if name1.Equals(name3) {
			t.Errorf("names with different first names should not be equal")
		}
	})

	t.Run("different last names should not be equal", func(t *testing.T) {
		if name1.Equals(name4) {
			t.Errorf("names with different last names should not be equal")
		}
	})

	t.Run("name should not equal different type", func(t *testing.T) {
		email, _ := NewEmail("john@example.com")
		if name1.Equals(email) {
			t.Errorf("name should not equal email value object")
		}
	})
}

func TestValueObject_Immutability(t *testing.T) {
	t.Run("email should be immutable", func(t *testing.T) {
		email, _ := NewEmail("john@example.com")
		originalValue := email.Value()

		// Email should not have any methods that modify its state
		// This test verifies that the value remains the same
		if email.Value() != originalValue {
			t.Errorf("email value should remain immutable")
		}
	})

	t.Run("name should be immutable", func(t *testing.T) {
		name, _ := NewName("John", "Doe")
		originalFirstName := name.FirstName()
		originalLastName := name.LastName()
		originalFullName := name.FullName()

		// Name should not have any methods that modify its state
		// This test verifies that the values remain the same
		if name.FirstName() != originalFirstName {
			t.Errorf("first name should remain immutable")
		}

		if name.LastName() != originalLastName {
			t.Errorf("last name should remain immutable")
		}

		if name.FullName() != originalFullName {
			t.Errorf("full name should remain immutable")
		}
	})
}

func TestValueObject_Validation(t *testing.T) {
	t.Run("email validation should be called during creation", func(t *testing.T) {
		_, err := NewEmail("")
		if err == nil {
			t.Errorf("expected validation error for empty email")
		}
	})

	t.Run("name validation should be called during creation", func(t *testing.T) {
		_, err := NewName("", "Doe")
		if err == nil {
			t.Errorf("expected validation error for empty first name")
		}

		_, err = NewName("John", "")
		if err == nil {
			t.Errorf("expected validation error for empty last name")
		}
	})

	t.Run("valid value objects should pass validation", func(t *testing.T) {
		email, err := NewEmail("john@example.com")
		if err != nil {
			t.Errorf("valid email should not fail validation: %v", err)
		}

		if err := email.Validate(); err != nil {
			t.Errorf("valid email should pass validation: %v", err)
		}

		name, err := NewName("John", "Doe")
		if err != nil {
			t.Errorf("valid name should not fail validation: %v", err)
		}

		if err := name.Validate(); err != nil {
			t.Errorf("valid name should pass validation: %v", err)
		}
	})
}
