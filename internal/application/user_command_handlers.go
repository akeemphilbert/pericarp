package application

import (
	"context"

	internaldomain "github.com/akeemphilbert/pericarp/internal/domain"
	pkgapp "github.com/akeemphilbert/pericarp/pkg/application"
	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
)

// CreateUserHandler handles CreateUserCommand
type CreateUserHandler struct {
	userRepo internaldomain.UserRepository
}

// Handle processes the CreateUserCommand
func (h *CreateUserHandler) Handle(ctx context.Context, logger pkgdomain.Logger, eventStore pkgdomain.EventStore, eventDispatcher pkgdomain.EventDispatcher, payload pkgapp.Payload[CreateUserCommand]) (resposne pkgapp.Response[pkgapp.CommandResponse], err error) {
	cmd := payload.Data
	logger.Debug("Processing CreateUserCommand", "id", cmd.ID, "email", cmd.Email, "name", cmd.Name)

	// Check if user already exists by email
	existingUser, err := h.userRepo.FindByEmail(cmd.Email)
	if err == nil && existingUser != nil {
		logger.Warn("User with email already exists", "email", cmd.Email)
		err = pkgapp.NewApplicationError("EMAIL_ALREADY_EXISTS", "Email is already in use", nil)
		return
	}

	unitOfWork := eventStore.NewUnitOfWork()

	// Create new user aggregate
	user := new(internaldomain.User).WithEmail(cmd.Email, cmd.Name)
	if user.IsValid() {
		// Register events with unit of work
		unitOfWork.RegisterEvents(user.UncommittedEvents())
		// Commit unit of work (persist and dispatch events)
		var envelopes []pkgdomain.Envelope
		envelopes, err = unitOfWork.Commit(ctx)
		if err != nil {
			logger.Error("Failed to commit unit of work", "id", cmd.ID, "error", err)
			unitOfWork.Rollback()
			return
		}
		eventDispatcher.Dispatch(ctx, envelopes)
	}

	return

}

// UpdateUserEmailHandler handles UpdateUserEmailCommand
type UpdateUserEmailHandler struct {
	userRepo internaldomain.UserRepository
}

// NewUpdateUserEmailHandler creates a new UpdateUserEmailHandler
func NewUpdateUserEmailHandler(userRepo internaldomain.UserRepository, unitOfWork pkgdomain.UnitOfWork) *UpdateUserEmailHandler {
	return &UpdateUserEmailHandler{
		userRepo: userRepo,
	}
}

// Handle processes the UpdateUserEmailCommand
func (h *UpdateUserEmailHandler) Handle(ctx context.Context, logger pkgdomain.Logger, eventStore pkgdomain.EventStore, eventDispatcher pkgdomain.EventDispatcher, payload pkgapp.Payload[UpdateUserEmailCommand]) (resposne pkgapp.Response[pkgapp.CommandResponse], err error) {
	cmd := payload.Data
	logger.Debug("Processing UpdateUserEmailCommand", "id", cmd.ID, "new_email", cmd.NewEmail)

	// Load user aggregate
	var user *internaldomain.User
	user, err = h.userRepo.FindByID(cmd.ID)
	if err != nil {
		return
	}
	// Check if new email is already in use by another user
	existingUser, err := h.userRepo.FindByEmail(cmd.NewEmail)
	if err == nil && existingUser != nil && existingUser.ID() != cmd.ID {
		logger.Warn("Email already in use by another user", "email", cmd.NewEmail, "existing_user_id", existingUser.ID())
		return
	}
	user.UpdateEmail(cmd.NewEmail)
	if user.IsValid() {
		// Register events with unit of work
		unitOfWork := eventStore.NewUnitOfWork()
		// Commit unit of work (persist and dispatch events)
		envelopes, err := unitOfWork.Commit(ctx)
		if err != nil {
			logger.Error("Failed to commit unit of work", "id", cmd.ID, "error", err)
			unitOfWork.Rollback()
		}
		eventDispatcher.Dispatch(ctx, envelopes)
	}

	logger.Info("User email updated successfully", "id", cmd.ID, "new_email", cmd.NewEmail, "events_dispatched", len(envelopes))
	return nil
}

// UpdateUserNameHandler handles UpdateUserNameCommand
type UpdateUserNameHandler struct {
	userRepo   internaldomain.UserRepository
	unitOfWork pkgdomain.UnitOfWork
}

// NewUpdateUserNameHandler creates a new UpdateUserNameHandler
func NewUpdateUserNameHandler(userRepo internaldomain.UserRepository, unitOfWork pkgdomain.UnitOfWork) *UpdateUserNameHandler {
	return &UpdateUserNameHandler{
		userRepo:   userRepo,
		unitOfWork: unitOfWork,
	}
}

// Handle processes the UpdateUserNameCommand
func (h *UpdateUserNameHandler) Handle(ctx context.Context, logger pkgdomain.Logger, cmd UpdateUserNameCommand) error {
	logger.Debug("Processing UpdateUserNameCommand", "id", cmd.ID, "new_name", cmd.NewName)

	// Load user aggregate
	user, err := h.userRepo.FindByID(cmd.ID)
	if err != nil {
		logger.Error("Failed to load user", "id", cmd.ID, "error", err)
		return pkgapp.NewApplicationError("USER_NOT_FOUND", "User not found", err)
	}

	// Update user name
	if err := user.UpdateName(cmd.NewName); err != nil {
		logger.Error("Failed to update user name", "id", cmd.ID, "new_name", cmd.NewName, "error", err)
		return pkgapp.NewApplicationError("NAME_UPDATE_FAILED", "Failed to update user name", err)
	}

	// Register events with unit of work
	h.unitOfWork.RegisterEvents(user.UncommittedEvents())

	// Save user through repository
	if err := h.userRepo.Save(user); err != nil {
		logger.Error("Failed to save user", "id", cmd.ID, "error", err)
		return pkgapp.NewApplicationError("USER_SAVE_FAILED", "Failed to save user", err)
	}

	// Commit unit of work (persist and dispatch events)
	envelopes, err := h.unitOfWork.Commit(ctx)
	if err != nil {
		logger.Error("Failed to commit unit of work", "id", cmd.ID, "error", err)
		return pkgapp.NewApplicationError("UNIT_OF_WORK_COMMIT_FAILED", "Failed to commit transaction", err)
	}

	logger.Info("User name updated successfully", "id", cmd.ID, "new_name", cmd.NewName, "events_dispatched", len(envelopes))
	return nil
}

// DeactivateUserHandler handles DeactivateUserCommand
type DeactivateUserHandler struct {
	userRepo   internaldomain.UserRepository
	unitOfWork pkgdomain.UnitOfWork
}

// NewDeactivateUserHandler creates a new DeactivateUserHandler
func NewDeactivateUserHandler(userRepo internaldomain.UserRepository, unitOfWork pkgdomain.UnitOfWork) *DeactivateUserHandler {
	return &DeactivateUserHandler{
		userRepo:   userRepo,
		unitOfWork: unitOfWork,
	}
}

// Handle processes the DeactivateUserCommand
func (h *DeactivateUserHandler) Handle(ctx context.Context, logger pkgdomain.Logger, cmd DeactivateUserCommand) error {
	logger.Debug("Processing DeactivateUserCommand", "id", cmd.ID)

	// Load user aggregate
	user, err := h.userRepo.FindByID(cmd.ID)
	if err != nil {
		logger.Error("Failed to load user", "id", cmd.ID, "error", err)
		return pkgapp.NewApplicationError("USER_NOT_FOUND", "User not found", err)
	}

	// Deactivate user
	if err := user.Deactivate(); err != nil {
		logger.Error("Failed to deactivate user", "id", cmd.ID, "error", err)
		return pkgapp.NewApplicationError("USER_DEACTIVATION_FAILED", "Failed to deactivate user", err)
	}

	// Register events with unit of work
	h.unitOfWork.RegisterEvents(user.UncommittedEvents())

	// Save user through repository
	if err := h.userRepo.Save(user); err != nil {
		logger.Error("Failed to save user", "id", cmd.ID, "error", err)
		return pkgapp.NewApplicationError("USER_SAVE_FAILED", "Failed to save user", err)
	}

	// Commit unit of work (persist and dispatch events)
	envelopes, err := h.unitOfWork.Commit(ctx)
	if err != nil {
		logger.Error("Failed to commit unit of work", "id", cmd.ID, "error", err)
		return pkgapp.NewApplicationError("UNIT_OF_WORK_COMMIT_FAILED", "Failed to commit transaction", err)
	}

	logger.Info("User deactivated successfully", "id", cmd.ID, "events_dispatched", len(envelopes))
	return nil
}

// ActivateUserHandler handles ActivateUserCommand
type ActivateUserHandler struct {
	userRepo   internaldomain.UserRepository
	unitOfWork pkgdomain.UnitOfWork
}

// NewActivateUserHandler creates a new ActivateUserHandler
func NewActivateUserHandler(userRepo internaldomain.UserRepository, unitOfWork pkgdomain.UnitOfWork) *ActivateUserHandler {
	return &ActivateUserHandler{
		userRepo:   userRepo,
		unitOfWork: unitOfWork,
	}
}

// Handle processes the ActivateUserCommand
func (h *ActivateUserHandler) Handle(ctx context.Context, logger pkgdomain.Logger, cmd ActivateUserCommand) error {
	logger.Debug("Processing ActivateUserCommand", "id", cmd.ID)

	// Load user aggregate
	user, err := h.userRepo.FindByID(cmd.ID)
	if err != nil {
		logger.Error("Failed to load user", "id", cmd.ID, "error", err)
		return pkgapp.NewApplicationError("USER_NOT_FOUND", "User not found", err)
	}

	// Activate user
	if err := user.Activate(); err != nil {
		logger.Error("Failed to activate user", "id", cmd.ID, "error", err)
		return pkgapp.NewApplicationError("USER_ACTIVATION_FAILED", "Failed to activate user", err)
	}

	// Register events with unit of work
	h.unitOfWork.RegisterEvents(user.UncommittedEvents())

	// Save user through repository
	if err := h.userRepo.Save(user); err != nil {
		logger.Error("Failed to save user", "id", cmd.ID, "error", err)
		return pkgapp.NewApplicationError("USER_SAVE_FAILED", "Failed to save user", err)
	}

	// Commit unit of work (persist and dispatch events)
	envelopes, err := h.unitOfWork.Commit(ctx)
	if err != nil {
		logger.Error("Failed to commit unit of work", "id", cmd.ID, "error", err)
		return pkgapp.NewApplicationError("UNIT_OF_WORK_COMMIT_FAILED", "Failed to commit transaction", err)
	}

	logger.Info("User activated successfully", "id", cmd.ID, "events_dispatched", len(envelopes))
	return nil
}
