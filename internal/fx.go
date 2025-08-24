package internal

import (
	"context"
	"fmt"

	"github.com/example/pericarp/internal/application"
	"github.com/example/pericarp/internal/domain"
	"github.com/example/pericarp/internal/infrastructure"
	pkgapp "github.com/example/pericarp/pkg/application"
	pkgdomain "github.com/example/pericarp/pkg/domain"
	pkginfra "github.com/example/pericarp/pkg/infrastructure"
	"go.uber.org/fx"
)

// InternalModule provides all internal application dependencies
var InternalModule = fx.Options(
	fx.Provide(
		// Repositories
		UserReadModelRepositoryProvider,
		UserRepositoryCompositeProvider,

		// User projector
		UserProjectorProvider,

		// Command and query handlers (raw handlers)
		CreateUserHandlerProvider,
		UpdateUserEmailHandlerProvider,
		GetUserHandlerProvider,
		GetUserByEmailHandlerProvider,
		ListUsersHandlerProvider,

		// Tagged handlers for registration
		fx.Annotate(CreateUserTaggedHandlerProvider, fx.ResultTags(`group:"public_command_handlers"`)),
		fx.Annotate(UpdateUserEmailTaggedHandlerProvider, fx.ResultTags(`group:"public_command_handlers"`)),
		fx.Annotate(GetUserTaggedHandlerProvider, fx.ResultTags(`group:"public_query_handlers"`)),
		fx.Annotate(GetUserByEmailTaggedHandlerProvider, fx.ResultTags(`group:"public_query_handlers"`)),
		fx.Annotate(ListUsersTaggedHandlerProvider, fx.ResultTags(`group:"public_query_handlers"`)),
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
func UserReadModelRepositoryProvider(db *pkginfra.Database) application.UserReadModelRepository {
	repo := infrastructure.NewUserReadModelGORMRepository(db.DB)

	// Auto-migrate the read model table
	if err := repo.Migrate(); err != nil {
		panic(err) // In production, handle this more gracefully
	}

	return repo
}

// UserRepositoryCompositeProvider creates a composite user repository
func UserRepositoryCompositeProvider(
	eventStore pkgdomain.EventStore,
	logger pkgdomain.Logger,
	readModelRepo application.UserReadModelRepository,
) domain.UserRepository {
	eventSourcingRepo := infrastructure.NewUserEventSourcingRepository(eventStore, logger)
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

// GetUserByEmailHandlerProvider creates a GetUserByEmailHandler
func GetUserByEmailHandlerProvider(
	readModelRepo application.UserReadModelRepository,
) *application.GetUserByEmailHandler {
	return application.NewGetUserByEmailHandler(readModelRepo)
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

// Tagged handler providers for registration with buses

// CreateUserTaggedHandlerProvider creates a tagged CreateUser command handler
func CreateUserTaggedHandlerProvider(handler *application.CreateUserHandler) pkgapp.TaggedCommandHandler {
	return pkgapp.TaggedCommandHandler{
		CommandType: "CreateUser",
		Handler: func(ctx context.Context, log pkgdomain.Logger, p pkgapp.Payload[pkgapp.Command]) (pkgapp.Response[struct{}], error) {
			cmd, ok := p.Data.(application.CreateUserCommand)
			if !ok {
				return pkgapp.Response[struct{}]{}, fmt.Errorf("invalid command type")
			}
			
			err := handler.Handle(ctx, log, cmd)
			return pkgapp.Response[struct{}]{Data: struct{}{}}, err
		},
	}
}

// UpdateUserEmailTaggedHandlerProvider creates a tagged UpdateUserEmail command handler
func UpdateUserEmailTaggedHandlerProvider(handler *application.UpdateUserEmailHandler) pkgapp.TaggedCommandHandler {
	return pkgapp.TaggedCommandHandler{
		CommandType: "UpdateUserEmail",
		Handler: func(ctx context.Context, log pkgdomain.Logger, p pkgapp.Payload[pkgapp.Command]) (pkgapp.Response[struct{}], error) {
			cmd, ok := p.Data.(application.UpdateUserEmailCommand)
			if !ok {
				return pkgapp.Response[struct{}]{}, fmt.Errorf("invalid command type")
			}
			
			err := handler.Handle(ctx, log, cmd)
			return pkgapp.Response[struct{}]{Data: struct{}{}}, err
		},
	}
}

// GetUserTaggedHandlerProvider creates a tagged GetUser query handler
func GetUserTaggedHandlerProvider(handler *application.GetUserHandler) pkgapp.TaggedQueryHandler {
	return pkgapp.TaggedQueryHandler{
		QueryType: "GetUser",
		Handler: func(ctx context.Context, log pkgdomain.Logger, p pkgapp.Payload[pkgapp.Query]) (pkgapp.Response[any], error) {
			query, ok := p.Data.(application.GetUserQuery)
			if !ok {
				return pkgapp.Response[any]{}, fmt.Errorf("invalid query type")
			}
			
			result, err := handler.Handle(ctx, log, query)
			return pkgapp.Response[any]{Data: result}, err
		},
	}
}

// GetUserByEmailTaggedHandlerProvider creates a tagged GetUserByEmail query handler
func GetUserByEmailTaggedHandlerProvider(handler *application.GetUserByEmailHandler) pkgapp.TaggedQueryHandler {
	return pkgapp.TaggedQueryHandler{
		QueryType: "GetUserByEmail",
		Handler: func(ctx context.Context, log pkgdomain.Logger, p pkgapp.Payload[pkgapp.Query]) (pkgapp.Response[any], error) {
			query, ok := p.Data.(application.GetUserByEmailQuery)
			if !ok {
				return pkgapp.Response[any]{}, fmt.Errorf("invalid query type")
			}
			
			result, err := handler.Handle(ctx, log, query)
			return pkgapp.Response[any]{Data: result}, err
		},
	}
}

// ListUsersTaggedHandlerProvider creates a tagged ListUsers query handler
func ListUsersTaggedHandlerProvider(handler *application.ListUsersHandler) pkgapp.TaggedQueryHandler {
	return pkgapp.TaggedQueryHandler{
		QueryType: "ListUsers",
		Handler: func(ctx context.Context, log pkgdomain.Logger, p pkgapp.Payload[pkgapp.Query]) (pkgapp.Response[any], error) {
			query, ok := p.Data.(application.ListUsersQuery)
			if !ok {
				return pkgapp.Response[any]{}, fmt.Errorf("invalid query type")
			}
			
			result, err := handler.Handle(ctx, log, query)
			return pkgapp.Response[any]{Data: result}, err
		},
	}
}
