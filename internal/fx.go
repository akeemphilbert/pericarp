package internal

import (
	"github.com/example/pericarp/internal/application"
	"github.com/example/pericarp/internal/domain"
	"github.com/example/pericarp/internal/infrastructure"
	pkgdomain "github.com/example/pericarp/pkg/domain"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

// InternalModule provides all internal components for examples and testing
var InternalModule = fx.Module("internal",
	// Domain layer
	fx.Provide(
	// User repository interface is provided by the concrete implementation
	),

	// Application layer
	fx.Provide(
		// Command handlers
		application.NewCreateUserHandler,
		application.NewUpdateUserEmailHandler,
		application.NewUpdateUserNameHandler,
		application.NewDeactivateUserHandler,
		application.NewActivateUserHandler,

		// Query handlers
		application.NewGetUserHandler,
		application.NewGetUserByEmailHandler,
		application.NewListUsersHandler,

		// Projector
		application.NewUserProjector,

		// Read model repository interface is provided by the concrete implementation
	),

	// Infrastructure layer
	fx.Provide(
		// GORM repository implementations
		fx.Annotate(
			infrastructure.NewUserReadModelGORMRepository,
			fx.As(new(application.UserReadModelRepository)),
		),
	),
)

// UserExampleModule provides a complete example setup for user management
var UserExampleModule = fx.Module("user-example",
	InternalModule,

	// Register event handlers
	fx.Invoke(func(
		projector *application.UserProjector,
		dispatcher pkgdomain.EventDispatcher,
	) error {
		return projector.RegisterEventHandlers(dispatcher)
	}),

	// Auto-migrate database tables
	fx.Invoke(func(db *gorm.DB) error {
		repo := infrastructure.NewUserReadModelGORMRepository(db)
		return repo.Migrate()
	}),
)
