package application

import (
	"context"
	"fmt"
	"time"

	internaldomain "github.com/akeemphilbert/pericarp/internal/domain"
	"github.com/akeemphilbert/pericarp/pkg/domain"
	"github.com/segmentio/ksuid"
)

// UserProjector handles domain events to maintain user read models
type UserProjector struct {
	readModelRepo UserReadModelRepository
	logger        domain.Logger
}

// NewUserProjector creates a new UserProjector
func NewUserProjector(readModelRepo UserReadModelRepository, logger domain.Logger) *UserProjector {
	return &UserProjector{
		readModelRepo: readModelRepo,
		logger:        logger,
	}
}

// HandleUserCreated processes User.created event to create a new user read model
func (p *UserProjector) HandleUserCreated(ctx context.Context, event *domain.EntityEvent) error {
	userIDStr := event.AggregateID()
	p.logger.Info("Processing User.created event for user", "user_id", userIDStr)

	// Convert string ID to ksuid.KSUID
	userID, err := ksuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("invalid user ID format: %w", err)
	}

	// Extract user data from event payload
	userData, ok := event.Data.(*internaldomain.User)
	if !ok {
		return fmt.Errorf("invalid user data in event payload")
	}

	// Check if user already exists (idempotency)
	existingUser, err := p.readModelRepo.GetByID(ctx, userID)
	if err == nil && existingUser != nil {
		p.logger.Debug("User read model already exists, skipping", "user_id", userIDStr)
		return nil
	}

	// Create new user read model
	now := time.Now()
	userReadModel := &UserReadModel{
		ID:        userID,
		Email:     userData.Email,
		Name:      userData.Name,
		IsActive:  userData.Active,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Save to read model repository
	if err := p.readModelRepo.Save(ctx, userReadModel); err != nil {
		p.logger.Error("Failed to save user read model", "user_id", userID, "error", err)
		return fmt.Errorf("failed to save user read model: %w", err)
	}

	p.logger.Info("Successfully created user read model", "user_id", userID)
	return nil
}

// HandleUserUpdated processes User.updated event to update user read model
func (p *UserProjector) HandleUserUpdated(ctx context.Context, event *domain.EntityEvent) error {
	userIDStr := event.AggregateID()
	p.logger.Info("Processing User.updated event for user", "user_id", userIDStr)

	// Convert string ID to ksuid.KSUID
	userID, err := ksuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("invalid user ID format: %w", err)
	}

	// Extract user data from event payload
	userData, ok := event.Data.(*internaldomain.User)
	if !ok {
		return fmt.Errorf("invalid user data in event payload")
	}

	// Get existing user read model
	userReadModel, err := p.readModelRepo.GetByID(ctx, userID)
	if err != nil {
		p.logger.Error("Failed to get user read model", "user_id", userIDStr, "error", err)
		return fmt.Errorf("failed to get user read model: %w", err)
	}

	if userReadModel == nil {
		p.logger.Warn("User read model not found, cannot update", "user_id", userIDStr)
		return fmt.Errorf("user read model not found for ID: %s", userIDStr)
	}

	// Update user data and timestamp
	userReadModel.Email = userData.Email
	userReadModel.Name = userData.Name
	userReadModel.IsActive = userData.Active
	userReadModel.UpdatedAt = time.Now()

	// Save updated read model
	if err := p.readModelRepo.Save(ctx, userReadModel); err != nil {
		p.logger.Error("Failed to update user read model", "user_id", userID, "error", err)
		return fmt.Errorf("failed to update user read model: %w", err)
	}

	p.logger.Info("Successfully updated user read model", "user_id", userID, "email", userData.Email, "name", userData.Name, "active", userData.Active)
	return nil
}

// Handle implements domain.EventHandler interface for the user projector
func (p *UserProjector) Handle(ctx context.Context, envelope domain.Envelope) error {
	p.logger.Debug("UserProjector handling event", "event_type", envelope.Event().EventType(), "event_id", envelope.EventID())

	event, ok := envelope.Event().(*domain.EntityEvent)
	if !ok {
		p.logger.Warn("UserProjector received non-EntityEvent", "event_type", fmt.Sprintf("%T", envelope.Event()))
		return fmt.Errorf("unsupported event type: %T", envelope.Event())
	}

	switch event.EventType() {
	case "User.created":
		return p.HandleUserCreated(ctx, event)
	case "User.updated":
		return p.HandleUserUpdated(ctx, event)
	default:
		p.logger.Warn("UserProjector received unsupported event type", "event_type", event.EventType())
		return fmt.Errorf("unsupported event type: %s", event.EventType())
	}
}

// EventTypes returns the list of event types this projector can handle
func (p *UserProjector) EventTypes() []string {
	return []string{"User.created", "User.updated"}
}

// RegisterEventHandlers registers this projector's event handlers with the event dispatcher
func (p *UserProjector) RegisterEventHandlers(dispatcher domain.EventDispatcher) error {
	eventTypes := p.EventTypes()
	for _, eventType := range eventTypes {
		if err := dispatcher.Subscribe(eventType, p); err != nil {
			return fmt.Errorf("failed to register %s handler: %w", eventType, err)
		}
	}

	p.logger.Info("Successfully registered UserProjector event handlers", "event_types", eventTypes)
	return nil
}
