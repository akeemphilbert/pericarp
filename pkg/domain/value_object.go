package domain

// ValueObject defines the interface for domain value objects
type ValueObject interface {
	// Equals compares this value object with another for equality
	Equals(other ValueObject) bool

	// Validate ensures the value object is in a valid state
	Validate() error
}

// BaseValueObject provides common functionality for value objects
type BaseValueObject struct{}

// Validate provides a default validation implementation
func (bvo BaseValueObject) Validate() error {
	return nil
}
