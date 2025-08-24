package internal

import (
	"github.com/example/pericarp/internal/application"
	"github.com/example/pericarp/internal/domain"
	"github.com/example/pericarp/internal/infrastructure"
	pkgdomain "github.com/example/pericarp/pkg/domain"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

// InternalModule provides all internal application dependencies
var InternalModule = fx.Options(
	fx.Provide(
		// Repositories
		UserEventSourcingRepositoryProvider,
		UserReadModelRepositoryProvider,
		UserRepositoryCompositeProvider,

		// User projector
		UserProjectorProvider,

		// Command and query handlers
		CreateUserHandlerProvider,
		UpdateUserEmailHandlerProvider,
		GetUserHandlerProvider,
		ListUsersHandlerProvider,
	),
	fx.Invoke(
		// Register event handlers
		RegisterUserProjectorEventHandlers,
	),
)

// UserProjectorProvider creates a UserProjector
func UserProjectorProvider(
	readModelRepo application.UserReadModelRepository,
	logger pkgdomain.Logger,
) *application.UserProjector {
	return application.NewUserProjector(readModelRepo, logger)
}

// UserEventSourcingRepositoryProvider creates an event sourcing repository for users
func UserEventSourcingRepositoryProvider(
	eventStore pkgdomain.EventStore,
	logger pkgdomain.Logger,
) domain.UserRepository {
	return infrastructure.NewUserEventSourcingRepository(eventStore, logger)
}

// UserReadModelRepositoryProvider creates a UserReadModelRepository
func UserReadModelRepositoryProvider(db *gorm.DB) application.UserReadModelRepository {
	repo := infrastructure.NewUserReadModelGORMRepository(db)

	// Auto-migrate the read model table
	if err := repo.Migrate(); err != nil {
		panic(err) // In production, handle this more gracefully
	}

	return repo
}

// UserRepositoryCompositeProvider creates a composite user repository
func UserRepositoryCompositeProvider(
	eventSourcingRepo domain.UserRepository,
	readModelRepo application.UserReadModelRepository,
) domain.UserRepository {
	return infrastructure.NewUserRepositoryComposite(eventSourcingRepo, readModelRepo)
}

// CreateUserHandlerProvider creates a CreateUserHandler
func CreateUserHandlerProvider(
	userRepo domain.UserRepository,
	unitOfWork pkgdomain.UnitOfWork,
) *application.CreateUserHandler {
	return application.NewCreateUserHandler(userRepo, unitOfWork)
}

// UpdateUserEmailHandlerProvider creates an UpdateUserEmailHandler
func UpdateUserEmailHandlerProvider(
	userRepo domain.UserRepository,
	unitOfWork pkgdomain.UnitOfWork,
) *application.UpdateUserEmailHandler {
	return application.NewUpdateUserEmailHandler(userRepo, unitOfWork)
}

// GetUserHandlerProvider creates a GetUserHandler
func GetUserHandlerProvider(
	readModelRepo application.UserReadModelRepository,
) *application.GetUserHandler {
	return application.NewGetUserHandler(readModelRepo)
}

// ListUsersHandlerProvider creates a ListUsersHandler
func ListUsersHandlerProvider(
	readModelRepo application.UserReadModelRepository,
) *application.ListUsersHandler {
	return application.NewListUsersHandler(readModelRepo)
}

// RegisterUserProjectorEventHandlers registers the user projector with the event dispatcher
func RegisterUserProjectorEventHandlers(
	projector *application.UserProjector,
	dispatcher pkgdomain.EventDispatcher,
) error {
	return projector.RegisterEventHandlers(dispatcher)
}
