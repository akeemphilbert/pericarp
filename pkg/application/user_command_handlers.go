package application

import (
	"context"

	"github.com/example/pericarp/pkg/domain"
)

// CreateUserHandler handles CreateUserCommand
type CreateUserHandler struct {
	userRepo   domain.UserRepository
	unitOfWork domain.UnitOfWork
}

// NewCreateUserHandler creates a new CreateUserHandler
func NewCreateUserHandler(userRepo domain.UserRepository, unitOfWork domain.UnitOfWork) *CreateUserHandler {
	return &CreateUserHandler{
		userRepo:   userRepo,
		unitOfWork: unitOfWork,
	}
}

// Handle processes the CreateUserCommand
func (h *CreateUserHandler) Handle(ctx context.Context, logger domain.Logger, cmd CreateUserCommand) error {
	logger.Debug("Processing CreateUserCommand", "id", cmd.ID, "email", cmd.Email, "name", cmd.Name)

	// Check if user already exists
	exists, err := h.userRepo.Exists(ctx, cmd.ID)
	if err != nil {
		logger.Error("Failed to check if user exists", "id", cmd.ID, "error", err)
		return NewApplicationError("USER_EXISTENCE_CHECK_FAILED", "Failed to check if user exists", err)
	}

	if exists {
		logger.Warn("User already exists", "id", cmd.ID)
		return NewApplicationError("USER_ALREADY_EXISTS", "User with this ID already exists", nil)
	}

	// Check if email is already in use
	emailExists, err := h.userRepo.ExistsByEmail(ctx, cmd.Email)
	if err != nil {
		logger.Error("Failed to check if email exists", "email", cmd.Email, "error", err)
		return NewApplicationError("EMAIL_EXISTENCE_CHECK_FAILED", "Failed to check if email exists", err)
	}

	if emailExists {
		logger.Warn("Email already in use", "email", cmd.Email)
		return NewApplicationError("EMAIL_ALREADY_EXISTS", "Email is already in use", nil)
	}

	// Create new user aggregate
	user, err := domain.NewUser(cmd.ID, cmd.Email, cmd.Name)
	if err != nil {
		logger.Error("Failed to create user aggregate", "id", cmd.ID, "error", err)
		return NewApplicationError("USER_CREATION_FAILED", "Failed to create user", err)
	}

	// Register events with unit of work
	h.unitOfWork.RegisterEvents(user.UncommittedEvents())

	// Save user through repository
	if err := h.userRepo.Save(ctx, user); err != nil {
		logger.Error("Failed to save user", "id", cmd.ID, "error", err)
		return NewApplicationError("USER_SAVE_FAILED", "Failed to save user", err)
	}

	// Commit unit of work (persist and dispatch events)
	envelopes, err := h.unitOfWork.Commit(ctx)
	if err != nil {
		logger.Error("Failed to commit unit of work", "id", cmd.ID, "error", err)
		return NewApplicationError("UNIT_OF_WORK_COMMIT_FAILED", "Failed to commit transaction", err)
	}

	logger.Info("User created successfully", "id", cmd.ID, "email", cmd.Email, "events_dispatched", len(envelopes))
	return nil
}

// UpdateUserEmailHandler handles UpdateUserEmailCommand
type UpdateUserEmailHandler struct {
	userRepo   domain.UserRepository
	unitOfWork domain.UnitOfWork
}

// NewUpdateUserEmailHandler creates a new UpdateUserEmailHandler
func NewUpdateUserEmailHandler(userRepo domain.UserRepository, unitOfWork domain.UnitOfWork) *UpdateUserEmailHandler {
	return &UpdateUserEmailHandler{
		userRepo:   userRepo,
		unitOfWork: unitOfWork,
	}
}

// Handle processes the UpdateUserEmailCommand
func (h *UpdateUserEmailHandler) Handle(ctx context.Context, logger domain.Logger, cmd UpdateUserEmailCommand) error {
	logger.Debug("Processing UpdateUserEmailCommand", "id", cmd.ID, "new_email", cmd.NewEmail)

	// Load user aggregate
	user, err := h.userRepo.Load(ctx, cmd.ID)
	if err != nil {
		logger.Error("Failed to load user", "id", cmd.ID, "error", err)
		return NewApplicationError("USER_LOAD_FAILED", "Failed to load user", err)
	}

	// Check if new email is already in use by another user
	emailExists, err := h.userRepo.ExistsByEmail(ctx, cmd.NewEmail)
	if err != nil {
		logger.Error("Failed to check if email exists", "email", cmd.NewEmail, "error", err)
		return NewApplicationError("EMAIL_EXISTENCE_CHECK_FAILED", "Failed to check if email exists", err)
	}

	if emailExists {
		// Check if it's the same user (in case they're trying to update to their current email)
		existingUser, err := h.userRepo.FindByEmail(ctx, cmd.NewEmail)
		if err != nil {
			logger.Error("Failed to find user by email", "email", cmd.NewEmail, "error", err)
			return NewApplicationError("USER_LOOKUP_FAILED", "Failed to lookup user by email", err)
		}

		if existingUser.ID() != cmd.ID {
			logger.Warn("Email already in use by another user", "email", cmd.NewEmail, "existing_user_id", existingUser.ID())
			return NewApplicationError("EMAIL_ALREADY_EXISTS", "Email is already in use by another user", nil)
		}
	}

	// Update user email
	if err := user.UpdateUserEmail(cmd.NewEmail); err != nil {
		logger.Error("Failed to update user email", "id", cmd.ID, "new_email", cmd.NewEmail, "error", err)
		return NewApplicationError("EMAIL_UPDATE_FAILED", "Failed to update user email", err)
	}

	// Register events with unit of work
	h.unitOfWork.RegisterEvents(user.UncommittedEvents())

	// Save user through repository
	if err := h.userRepo.Save(ctx, user); err != nil {
		logger.Error("Failed to save user", "id", cmd.ID, "error", err)
		return NewApplicationError("USER_SAVE_FAILED", "Failed to save user", err)
	}

	// Commit unit of work (persist and dispatch events)
	envelopes, err := h.unitOfWork.Commit(ctx)
	if err != nil {
		logger.Error("Failed to commit unit of work", "id", cmd.ID, "error", err)
		return NewApplicationError("UNIT_OF_WORK_COMMIT_FAILED", "Failed to commit transaction", err)
	}

	logger.Info("User email updated successfully", "id", cmd.ID, "new_email", cmd.NewEmail, "events_dispatched", len(envelopes))
	return nil
}
