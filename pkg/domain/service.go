package domain

// DomainService defines the interface for domain services that contain
// business logic that doesn't naturally belong to any single aggregate
type DomainService interface {
	// Domain services should define their own specific methods
	// This is a marker interface to identify domain services
}
