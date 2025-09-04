package application

import (
	"context"

	internaldomain "github.com/akeemphilbert/pericarp/internal/domain"
	pkgapp "github.com/akeemphilbert/pericarp/pkg/application"
	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	pkginfra "github.com/akeemphilbert/pericarp/pkg/infrastructure"
)

// CreateUserHandler handles CreateUserCommand
type CreateUserHandler struct {
	userRepo internaldomain.UserRepository
}

// Handle processes the CreateUserCommand
func (h *CreateUserHandler) Handle(ctx context.Context, logger pkgdomain.Logger, eventStore pkgdomain.EventStore, eventDispatcher pkgdomain.EventDispatcher, payload pkgapp.Payload[pkgapp.Command]) (pkgapp.Response[any], error) {
	cmd, ok := payload.Data.(CreateUserCommand)
	if !ok {
		return pkgapp.Response[any]{
			Error: pkgapp.NewApplicationError("INVALID_COMMAND", "Expected CreateUserCommand", nil),
		}, nil
	}
	logger.Debug("Processing CreateUserCommand", "id", cmd.ID, "email", cmd.Email, "name", cmd.Name)

	// Check if user already exists by email
	existingUser, err := h.userRepo.FindByEmail(cmd.Email)
	if err == nil && existingUser != nil {
		logger.Warn("User with email already exists", "email", cmd.Email)
		return pkgapp.Response[any]{
			Error: pkgapp.NewApplicationError("EMAIL_ALREADY_EXISTS", "Email is already in use", nil),
		}, nil
	}

	unitOfWork := pkginfra.NewUnitOfWork(eventStore, eventDispatcher)

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
			return pkgapp.Response[any]{
				Error: err,
			}, nil
		}
		eventDispatcher.Dispatch(ctx, envelopes)
	} else {
		// Handle validation errors
		errors := user.Errors()
		if len(errors) > 0 {
			logger.Error("User validation failed", "id", cmd.ID, "errors", errors)
			return pkgapp.Response[any]{
				Error: pkgapp.NewApplicationError("USER_VALIDATION_FAILED", "User validation failed", errors[0]),
			}, nil
		}
	}

	return pkgapp.Response[any]{
		Data: pkgapp.CommandResponse{
			Code:    200,
			Message: "User created successfully",
			Payload: map[string]string{"user_id": user.ID()},
		},
	}, nil
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
func (h *UpdateUserEmailHandler) Handle(ctx context.Context, logger pkgdomain.Logger, eventStore pkgdomain.EventStore, eventDispatcher pkgdomain.EventDispatcher, payload pkgapp.Payload[pkgapp.Command]) (pkgapp.Response[any], error) {
	cmd, ok := payload.Data.(UpdateUserEmailCommand)
	if !ok {
		return pkgapp.Response[any]{
			Error: pkgapp.NewApplicationError("INVALID_COMMAND", "Expected UpdateUserEmailCommand", nil),
		}, nil
	}
	logger.Debug("Processing UpdateUserEmailCommand", "id", cmd.ID, "new_email", cmd.NewEmail)

	// Load user aggregate
	user, err := h.userRepo.FindByID(cmd.ID)
	if err != nil {
		return pkgapp.Response[any]{
			Error: pkgapp.NewApplicationError("USER_NOT_FOUND", "User not found", err),
		}, nil
	}

	// Check if new email is already in use by another user
	existingUser, err := h.userRepo.FindByEmail(cmd.NewEmail)
	if err == nil && existingUser != nil && existingUser.ID() != cmd.ID {
		logger.Warn("Email already in use by another user", "email", cmd.NewEmail, "existing_user_id", existingUser.ID())
		return pkgapp.Response[any]{
			Error: pkgapp.NewApplicationError("EMAIL_ALREADY_EXISTS", "Email is already in use by another user", nil),
		}, nil
	}

	user.UpdateEmail(cmd.NewEmail)

	if user.IsValid() {
		// Register events with unit of work
		unitOfWork := pkginfra.NewUnitOfWork(eventStore, eventDispatcher)
		unitOfWork.RegisterEvents(user.UncommittedEvents())
		// Commit unit of work (persist and dispatch events)
		envelopes, err := unitOfWork.Commit(ctx)
		if err != nil {
			logger.Error("Failed to commit unit of work", "id", cmd.ID, "error", err)
			unitOfWork.Rollback()
			return pkgapp.Response[any]{
				Error: err,
			}, nil
		}
		eventDispatcher.Dispatch(ctx, envelopes)
		logger.Info("User email updated successfully", "id", cmd.ID, "new_email", cmd.NewEmail, "events_dispatched", len(envelopes))
	} else {
		// Handle validation errors
		errors := user.Errors()
		if len(errors) > 0 {
			logger.Error("User email update validation failed", "id", cmd.ID, "errors", errors)
			return pkgapp.Response[any]{
				Error: pkgapp.NewApplicationError("USER_VALIDATION_FAILED", "User email update validation failed", errors[0]),
			}, nil
		}
	}

	return pkgapp.Response[any]{
		Data: pkgapp.CommandResponse{
			Code:    200,
			Message: "User email updated successfully",
			Payload: map[string]string{"user_id": cmd.ID, "new_email": cmd.NewEmail},
		},
	}, nil
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
func (h *DeactivateUserHandler) Handle(ctx context.Context, logger pkgdomain.Logger, eventStore pkgdomain.EventStore, eventDispatcher pkgdomain.EventDispatcher, payload pkgapp.Payload[pkgapp.Command]) (pkgapp.Response[any], error) {
	cmd, ok := payload.Data.(DeactivateUserCommand)
	if !ok {
		return pkgapp.Response[any]{
			Error: pkgapp.NewApplicationError("INVALID_COMMAND", "Expected DeactivateUserCommand", nil),
		}, nil
	}
	logger.Debug("Processing DeactivateUserCommand", "id", cmd.ID)

	// Load user aggregate
	user, err := h.userRepo.FindByID(cmd.ID)
	if err != nil {
		logger.Error("Failed to load user", "id", cmd.ID, "error", err)
		return pkgapp.Response[any]{
			Error: pkgapp.NewApplicationError("USER_NOT_FOUND", "User not found", err),
		}, nil
	}

	// Deactivate user
	user.Deactivate()

	// Check if deactivation was successful
	if !user.IsValid() {
		errors := user.Errors()
		if len(errors) > 0 {
			logger.Error("Failed to deactivate user", "id", cmd.ID, "error", errors[0])
			return pkgapp.Response[any]{
				Error: pkgapp.NewApplicationError("USER_DEACTIVATION_FAILED", "Failed to deactivate user", errors[0]),
			}, nil
		}
	}

	// Register events with unit of work
	h.unitOfWork.RegisterEvents(user.UncommittedEvents())

	// Save user through repository
	if err := h.userRepo.Save(user); err != nil {
		logger.Error("Failed to save user", "id", cmd.ID, "error", err)
		return pkgapp.Response[any]{
			Error: pkgapp.NewApplicationError("USER_SAVE_FAILED", "Failed to save user", err),
		}, nil
	}

	// Commit unit of work (persist and dispatch events)
	envelopes, err := h.unitOfWork.Commit(ctx)
	if err != nil {
		logger.Error("Failed to commit unit of work", "id", cmd.ID, "error", err)
		return pkgapp.Response[any]{
			Error: pkgapp.NewApplicationError("UNIT_OF_WORK_COMMIT_FAILED", "Failed to commit transaction", err),
		}, nil
	}

	logger.Info("User deactivated successfully", "id", cmd.ID, "events_dispatched", len(envelopes))
	return pkgapp.Response[any]{
		Data: pkgapp.CommandResponse{
			Code:    200,
			Message: "User deactivated successfully",
			Payload: map[string]string{"user_id": cmd.ID},
		},
	}, nil
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
func (h *ActivateUserHandler) Handle(ctx context.Context, logger pkgdomain.Logger, eventStore pkgdomain.EventStore, eventDispatcher pkgdomain.EventDispatcher, payload pkgapp.Payload[pkgapp.Command]) (pkgapp.Response[any], error) {
	cmd, ok := payload.Data.(ActivateUserCommand)
	if !ok {
		return pkgapp.Response[any]{
			Error: pkgapp.NewApplicationError("INVALID_COMMAND", "Expected ActivateUserCommand", nil),
		}, nil
	}
	logger.Debug("Processing ActivateUserCommand", "id", cmd.ID)

	// Load user aggregate
	user, err := h.userRepo.FindByID(cmd.ID.String())
	if err != nil {
		logger.Error("Failed to load user", "id", cmd.ID, "error", err)
		return pkgapp.Response[any]{
			Error: pkgapp.NewApplicationError("USER_NOT_FOUND", "User not found", err),
		}, nil
	}

	// Activate user
	user.Activate()

	// Check if activation was successful
	if !user.IsValid() {
		errors := user.Errors()
		if len(errors) > 0 {
			logger.Error("Failed to activate user", "id", cmd.ID, "error", errors[0])
			return pkgapp.Response[any]{
				Error: pkgapp.NewApplicationError("USER_ACTIVATION_FAILED", "Failed to activate user", errors[0]),
			}, nil
		}
	}

	// Register events with unit of work
	h.unitOfWork.RegisterEvents(user.UncommittedEvents())

	// Save user through repository
	if err := h.userRepo.Save(user); err != nil {
		logger.Error("Failed to save user", "id", cmd.ID, "error", err)
		return pkgapp.Response[any]{
			Error: pkgapp.NewApplicationError("USER_SAVE_FAILED", "Failed to save user", err),
		}, nil
	}

	// Commit unit of work (persist and dispatch events)
	envelopes, err := h.unitOfWork.Commit(ctx)
	if err != nil {
		logger.Error("Failed to commit unit of work", "id", cmd.ID, "error", err)
		return pkgapp.Response[any]{
			Error: pkgapp.NewApplicationError("UNIT_OF_WORK_COMMIT_FAILED", "Failed to commit transaction", err),
		}, nil
	}

	logger.Info("User activated successfully", "id", cmd.ID, "events_dispatched", len(envelopes))
	return pkgapp.Response[any]{
		Data: pkgapp.CommandResponse{
			Code:    200,
			Message: "User activated successfully",
			Payload: map[string]string{"user_id": cmd.ID.String()},
		},
	}, nil
}
