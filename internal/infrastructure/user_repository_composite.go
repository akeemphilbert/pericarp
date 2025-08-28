package infrastructure

import (
	"context"

	"github.com/akeemphilbert/pericarp/internal/application"
	"github.com/akeemphilbert/pericarp/internal/domain"
	"github.com/segmentio/ksuid"
)

// UserRepositoryComposite combines event sourcing repository with read model queries
// This implements domain.UserRepository by delegating to appropriate repositories
type UserRepositoryComposite struct {
	eventSourcingRepo domain.UserRepository
	readModelRepo     application.UserReadModelRepository
}

// NewUserRepositoryComposite creates a composite repository that combines event sourcing with read models
func NewUserRepositoryComposite(
	eventSourcingRepo domain.UserRepository,
	readModelRepo application.UserReadModelRepository,
) *UserRepositoryComposite {
	return &UserRepositoryComposite{
		eventSourcingRepo: eventSourcingRepo,
		readModelRepo:     readModelRepo,
	}
}

// Save persists the user aggregate using event sourcing
func (r *UserRepositoryComposite) Save(user *domain.User) error {
	return r.eventSourcingRepo.Save(user)
}

// FindByID loads a user aggregate by reconstructing it from events
func (r *UserRepositoryComposite) FindByID(id ksuid.KSUID) (*domain.User, error) {
	return r.eventSourcingRepo.FindByID(id)
}

// LoadFromVersion loads a user aggregate from a specific version
func (r *UserRepositoryComposite) LoadFromVersion(id ksuid.KSUID, version int) (*domain.User, error) {
	return r.eventSourcingRepo.LoadFromVersion(id, version)
}

// FindByEmail finds a user by email using the read model for efficiency
func (r *UserRepositoryComposite) FindByEmail(email string) (*domain.User, error) {
	// First try to find in read model
	readModel, err := r.readModelRepo.GetByEmail(context.Background(), email)
	if err != nil {
		return nil, err
	}

	if readModel == nil {
		return nil, nil
	}

	// Load the full aggregate from event store
	return r.eventSourcingRepo.FindByID(readModel.ID)
}

// Delete removes a user
func (r *UserRepositoryComposite) Delete(id ksuid.KSUID) error {
	return r.eventSourcingRepo.Delete(id)
}
