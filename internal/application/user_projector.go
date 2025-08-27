package application

import (
	"context"
	"fmt"
	"time"

	internaldomain "github.com/akeemphilbert/pericarp/internal/domain"
	"github.com/akeemphilbert/pericarp/pkg/domain"
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

// HandleUserCreated processes UserCreatedEvent to create a new user read model
func (p *UserProjector) HandleUserCreated(ctx context.Context, event internaldomain.UserCreatedEvent) error {
	p.logger.Info("Processing UserCreatedEvent for user", "user_id", event.UserID)

	// Check if user already exists (idempotency)
	existingUser, err := p.readModelRepo.GetByID(ctx, event.UserID)
	if err == nil && existingUser != nil {
		p.logger.Debug("User read model already exists, skipping", "user_id", event.UserID)
		return nil
	}

	// Create new user read model
	now := time.Now()
	userReadModel := &UserReadModel{
		ID:        event.UserID,
		Email:     event.Email,
		Name:      event.Name,
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Save to read model repository
	if err := p.readModelRepo.Save(ctx, userReadModel); err != nil {
		p.logger.Error("Failed to save user read model", "user_id", event.UserID, "error", err)
		return fmt.Errorf("failed to save user read model: %w", err)
	}

	p.logger.Info("Successfully created user read model", "user_id", event.UserID)
	return nil
}

// HandleUserEmailUpdated processes UserEmailUpdatedEvent to update user read model
func (p *UserProjector) HandleUserEmailUpdated(ctx context.Context, event internaldomain.UserEmailUpdatedEvent) error {
	p.logger.Info("Processing UserEmailUpdatedEvent for user", "user_id", event.UserID)

	// Get existing user read model
	userReadModel, err := p.readModelRepo.GetByID(ctx, event.UserID)
	if err != nil {
		p.logger.Error("Failed to get user read model", "user_id", event.UserID, "error", err)
		return fmt.Errorf("failed to get user read model: %w", err)
	}

	if userReadModel == nil {
		p.logger.Warn("User read model not found, cannot update email", "user_id", event.UserID)
		return fmt.Errorf("user read model not found for ID: %s", event.UserID)
	}

	// Update email and timestamp
	userReadModel.Email = event.NewEmail
	userReadModel.UpdatedAt = time.Now()

	// Save updated read model
	if err := p.readModelRepo.Save(ctx, userReadModel); err != nil {
		p.logger.Error("Failed to update user read model", "user_id", event.UserID, "error", err)
		return fmt.Errorf("failed to update user read model: %w", err)
	}

	p.logger.Info("Successfully updated email for user", "user_id", event.UserID, "old_email", event.OldEmail, "new_email", event.NewEmail)
	return nil
}

// HandleUserNameUpdated processes UserNameUpdatedEvent to update user read model
func (p *UserProjector) HandleUserNameUpdated(ctx context.Context, event internaldomain.UserNameUpdatedEvent) error {
	p.logger.Info("Processing UserNameUpdatedEvent for user", "user_id", event.UserID)

	// Get existing user read model
	userReadModel, err := p.readModelRepo.GetByID(ctx, event.UserID)
	if err != nil {
		p.logger.Error("Failed to get user read model", "user_id", event.UserID, "error", err)
		return fmt.Errorf("failed to get user read model: %w", err)
	}

	if userReadModel == nil {
		p.logger.Warn("User read model not found, cannot update name", "user_id", event.UserID)
		return fmt.Errorf("user read model not found for ID: %s", event.UserID)
	}

	// Update name and timestamp
	userReadModel.Name = event.NewName
	userReadModel.UpdatedAt = time.Now()

	// Save updated read model
	if err := p.readModelRepo.Save(ctx, userReadModel); err != nil {
		p.logger.Error("Failed to update user read model", "user_id", event.UserID, "error", err)
		return fmt.Errorf("failed to update user read model: %w", err)
	}

	p.logger.Info("Successfully updated name for user", "user_id", event.UserID, "old_name", event.OldName, "new_name", event.NewName)
	return nil
}

// HandleUserDeactivated processes UserDeactivatedEvent to update user read model
func (p *UserProjector) HandleUserDeactivated(ctx context.Context, event internaldomain.UserDeactivatedEvent) error {
	p.logger.Info("Processing UserDeactivatedEvent for user", "user_id", event.UserID)

	// Get existing user read model
	userReadModel, err := p.readModelRepo.GetByID(ctx, event.UserID)
	if err != nil {
		p.logger.Error("Failed to get user read model", "user_id", event.UserID, "error", err)
		return fmt.Errorf("failed to get user read model: %w", err)
	}

	if userReadModel == nil {
		p.logger.Warn("User read model not found, cannot deactivate", "user_id", event.UserID)
		return fmt.Errorf("user read model not found for ID: %s", event.UserID)
	}

	// Update active status and timestamp
	userReadModel.IsActive = false
	userReadModel.UpdatedAt = time.Now()

	// Save updated read model
	if err := p.readModelRepo.Save(ctx, userReadModel); err != nil {
		p.logger.Error("Failed to update user read model", "user_id", event.UserID, "error", err)
		return fmt.Errorf("failed to update user read model: %w", err)
	}

	p.logger.Info("Successfully deactivated user", "user_id", event.UserID)
	return nil
}

// HandleUserActivated processes UserActivatedEvent to update user read model
func (p *UserProjector) HandleUserActivated(ctx context.Context, event internaldomain.UserActivatedEvent) error {
	p.logger.Info("Processing UserActivatedEvent for user", "user_id", event.UserID)

	// Get existing user read model
	userReadModel, err := p.readModelRepo.GetByID(ctx, event.UserID)
	if err != nil {
		p.logger.Error("Failed to get user read model", "user_id", event.UserID, "error", err)
		return fmt.Errorf("failed to get user read model: %w", err)
	}

	if userReadModel == nil {
		p.logger.Warn("User read model not found, cannot activate", "user_id", event.UserID)
		return fmt.Errorf("user read model not found for ID: %s", event.UserID)
	}

	// Update active status and timestamp
	userReadModel.IsActive = true
	userReadModel.UpdatedAt = time.Now()

	// Save updated read model
	if err := p.readModelRepo.Save(ctx, userReadModel); err != nil {
		p.logger.Error("Failed to update user read model", "user_id", event.UserID, "error", err)
		return fmt.Errorf("failed to update user read model: %w", err)
	}

	p.logger.Info("Successfully activated user", "user_id", event.UserID)
	return nil
}

// Handle implements domain.EventHandler interface for the user projector
func (p *UserProjector) Handle(ctx context.Context, envelope domain.Envelope) error {
	p.logger.Debug("UserProjector handling event", "event_type", envelope.Event().EventType(), "event_id", envelope.EventID())

	switch event := envelope.Event().(type) {
	case internaldomain.UserCreatedEvent:
		return p.HandleUserCreated(ctx, event)
	case internaldomain.UserEmailUpdatedEvent:
		return p.HandleUserEmailUpdated(ctx, event)
	case internaldomain.UserNameUpdatedEvent:
		return p.HandleUserNameUpdated(ctx, event)
	case internaldomain.UserDeactivatedEvent:
		return p.HandleUserDeactivated(ctx, event)
	case internaldomain.UserActivatedEvent:
		return p.HandleUserActivated(ctx, event)
	default:
		p.logger.Warn("UserProjector received unsupported event type", "event_type", fmt.Sprintf("%T", event))
		return fmt.Errorf("unsupported event type: %T", event)
	}
}

// EventTypes returns the list of event types this projector can handle
func (p *UserProjector) EventTypes() []string {
	return []string{"UserCreated", "UserEmailUpdated", "UserNameUpdated", "UserDeactivated", "UserActivated"}
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
