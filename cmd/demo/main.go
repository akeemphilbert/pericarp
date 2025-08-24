package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/example/pericarp/internal/application"
	"github.com/example/pericarp/pkg"
	pkgapp "github.com/example/pericarp/pkg/application"
	"github.com/example/pericarp/pkg/domain"
	"github.com/example/pericarp/pkg/infrastructure"
	"github.com/segmentio/ksuid"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

var (
	configFile string
	verbose    bool
)

// setEnvironmentVariableSecurely sets an environment variable with proper error handling
// and logging. This addresses CWE-703 by ensuring all system operations are error-checked.
func setEnvironmentVariableSecurely(key, value string) error {
	if err := os.Setenv(key, value); err != nil {
		log.Printf("ERROR: Failed to set environment variable %s: %v", key, err)
		return fmt.Errorf("failed to set environment variable %s: %w", key, err)
	}
	log.Printf("DEBUG: Environment variable %s set successfully", key)
	return nil
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "pericarp-demo",
		Short: "Pericarp library demonstration CLI",
		Long: `A demonstration CLI application showcasing the Pericarp library's
Domain-Driven Design, CQRS, and Event Sourcing capabilities.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Set config file if provided
			if configFile != "" {
				// This would be used by viper in LoadConfig
				if err := setEnvironmentVariableSecurely("PERICARP_CONFIG_FILE", configFile); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to set config file environment variable: %v\n", err)
					// Continue execution as this is not critical for basic functionality
				}
			}
			
			// Set verbose logging if requested
			if verbose {
				if err := setEnvironmentVariableSecurely("PERICARP_LOGGING_LEVEL", "debug"); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to set logging level environment variable: %v\n", err)
					// Continue execution as this is not critical for basic functionality
				}
			}
		},
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default is config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Add commands
	rootCmd.AddCommand(createUserCmd())
	rootCmd.AddCommand(updateUserCmd())
	rootCmd.AddCommand(getUserCmd())
	rootCmd.AddCommand(listUsersCmd())
	rootCmd.AddCommand(initDBCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// createUserCmd creates a new user
func createUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-user <email> <name>",
		Short: "Create a new user",
		Long:  "Create a new user with the specified email and name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			email := args[0]
			name := args[1]

			return runWithApp(func(ctx context.Context, logger domain.Logger, commandBus pkgapp.CommandBus) error {
				userID := ksuid.New()
				command := application.CreateUserCommand{
					ID:    userID,
					Email: email,
					Name:  name,
				}

				if err := command.Validate(); err != nil {
					return fmt.Errorf("validation failed: %w", err)
				}

				logger.Info("Creating user", "id", userID, "email", email, "name", name)

				if err := commandBus.Handle(ctx, logger, command); err != nil {
					return fmt.Errorf("failed to create user: %w", err)
				}

				fmt.Printf("✅ User created successfully!\n")
				fmt.Printf("   ID: %s\n", userID)
				fmt.Printf("   Email: %s\n", email)
				fmt.Printf("   Name: %s\n", name)
				return nil
			})
		},
	}
	return cmd
}

// updateUserCmd updates a user's email
func updateUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-user",
		Short: "Update user information",
		Long:  "Update user information (email or name)",
	}

	emailCmd := &cobra.Command{
		Use:   "email <user-id> <new-email>",
		Short: "Update user's email",
		Long:  "Update a user's email address",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			userIDStr := args[0]
			newEmail := args[1]

			userID, err := ksuid.Parse(userIDStr)
			if err != nil {
				return fmt.Errorf("invalid user ID format: %w", err)
			}

			return runWithApp(func(ctx context.Context, logger domain.Logger, commandBus pkgapp.CommandBus) error {
				command := application.UpdateUserEmailCommand{
					ID:       userID,
					NewEmail: newEmail,
				}

				if err := command.Validate(); err != nil {
					return fmt.Errorf("validation failed: %w", err)
				}

				logger.Info("Updating user email", "id", userID, "new_email", newEmail)

				if err := commandBus.Handle(ctx, logger, command); err != nil {
					return fmt.Errorf("failed to update user email: %w", err)
				}

				fmt.Printf("✅ User email updated successfully!\n")
				fmt.Printf("   ID: %s\n", userID)
				fmt.Printf("   New Email: %s\n", newEmail)
				return nil
			})
		},
	}

	nameCmd := &cobra.Command{
		Use:   "name <user-id> <new-name>",
		Short: "Update user's name",
		Long:  "Update a user's name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			userIDStr := args[0]
			newName := args[1]

			userID, err := ksuid.Parse(userIDStr)
			if err != nil {
				return fmt.Errorf("invalid user ID format: %w", err)
			}

			return runWithApp(func(ctx context.Context, logger domain.Logger, commandBus pkgapp.CommandBus) error {
				command := application.UpdateUserNameCommand{
					ID:      userID,
					NewName: newName,
				}

				if err := command.Validate(); err != nil {
					return fmt.Errorf("validation failed: %w", err)
				}

				logger.Info("Updating user name", "id", userID, "new_name", newName)

				if err := commandBus.Handle(ctx, logger, command); err != nil {
					return fmt.Errorf("failed to update user name: %w", err)
				}

				fmt.Printf("✅ User name updated successfully!\n")
				fmt.Printf("   ID: %s\n", userID)
				fmt.Printf("   New Name: %s\n", newName)
				return nil
			})
		},
	}

	cmd.AddCommand(emailCmd)
	cmd.AddCommand(nameCmd)
	return cmd
}

// getUserCmd gets a user by ID or email
func getUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-user",
		Short: "Get user information",
		Long:  "Get user information by ID or email",
	}

	byIDCmd := &cobra.Command{
		Use:   "by-id <user-id>",
		Short: "Get user by ID",
		Long:  "Get user information by user ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			userIDStr := args[0]

			userID, err := ksuid.Parse(userIDStr)
			if err != nil {
				return fmt.Errorf("invalid user ID format: %w", err)
			}

			return runWithApp(func(ctx context.Context, logger domain.Logger, queryBus pkgapp.QueryBus) error {
				query := application.GetUserQuery{
					ID: userID,
				}

				if err := query.Validate(); err != nil {
					return fmt.Errorf("validation failed: %w", err)
				}

				logger.Debug("Getting user by ID", "id", userID)

				result, err := queryBus.Handle(ctx, logger, query)
				if err != nil {
					return fmt.Errorf("failed to get user: %w", err)
				}

				user, ok := result.(application.UserDTO)
				if !ok {
					return fmt.Errorf("unexpected result type")
				}

				fmt.Printf("👤 User Information:\n")
				fmt.Printf("   ID: %s\n", user.ID)
				fmt.Printf("   Email: %s\n", user.Email)
				fmt.Printf("   Name: %s\n", user.Name)
				fmt.Printf("   Active: %t\n", user.IsActive)
				fmt.Printf("   Created: %s\n", user.CreatedAt.Format(time.RFC3339))
				fmt.Printf("   Updated: %s\n", user.UpdatedAt.Format(time.RFC3339))
				return nil
			})
		},
	}

	byEmailCmd := &cobra.Command{
		Use:   "by-email <email>",
		Short: "Get user by email",
		Long:  "Get user information by email address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			email := args[0]

			return runWithApp(func(ctx context.Context, logger domain.Logger, queryBus pkgapp.QueryBus) error {
				query := application.GetUserByEmailQuery{
					Email: email,
				}

				if err := query.Validate(); err != nil {
					return fmt.Errorf("validation failed: %w", err)
				}

				logger.Debug("Getting user by email", "email", email)

				result, err := queryBus.Handle(ctx, logger, query)
				if err != nil {
					return fmt.Errorf("failed to get user: %w", err)
				}

				user, ok := result.(application.UserDTO)
				if !ok {
					return fmt.Errorf("unexpected result type")
				}

				fmt.Printf("👤 User Information:\n")
				fmt.Printf("   ID: %s\n", user.ID)
				fmt.Printf("   Email: %s\n", user.Email)
				fmt.Printf("   Name: %s\n", user.Name)
				fmt.Printf("   Active: %t\n", user.IsActive)
				fmt.Printf("   Created: %s\n", user.CreatedAt.Format(time.RFC3339))
				fmt.Printf("   Updated: %s\n", user.UpdatedAt.Format(time.RFC3339))
				return nil
			})
		},
	}

	cmd.AddCommand(byIDCmd)
	cmd.AddCommand(byEmailCmd)
	return cmd
}

// listUsersCmd lists users with pagination
func listUsersCmd() *cobra.Command {
	var page, pageSize int
	var activeOnly, inactiveOnly bool

	cmd := &cobra.Command{
		Use:   "list-users",
		Short: "List users",
		Long:  "List users with pagination and filtering options",
		RunE: func(cmd *cobra.Command, args []string) error {
			var active *bool
			if activeOnly {
				active = &[]bool{true}[0]
			} else if inactiveOnly {
				active = &[]bool{false}[0]
			}

			return runWithApp(func(ctx context.Context, logger domain.Logger, queryBus pkgapp.QueryBus) error {
				query := application.ListUsersQuery{
					Page:     page,
					PageSize: pageSize,
					Active:   active,
				}

				if err := query.Validate(); err != nil {
					return fmt.Errorf("validation failed: %w", err)
				}

				logger.Debug("Listing users", "page", page, "page_size", pageSize, "active", active)

				result, err := queryBus.Handle(ctx, logger, query)
				if err != nil {
					return fmt.Errorf("failed to list users: %w", err)
				}

				listResult, ok := result.(application.ListUsersResult)
				if !ok {
					return fmt.Errorf("unexpected result type")
				}

				fmt.Printf("📋 Users (Page %d of %d, %d total):\n", listResult.Page, listResult.TotalPages, listResult.TotalCount)
				fmt.Printf("─────────────────────────────────────────────────────────────────────────────\n")

				if len(listResult.Users) == 0 {
					fmt.Printf("   No users found.\n")
					return nil
				}

				for i, user := range listResult.Users {
					status := "✅ Active"
					if !user.IsActive {
						status = "❌ Inactive"
					}

					fmt.Printf("%d. %s\n", i+1, user.Name)
					fmt.Printf("   ID: %s\n", user.ID)
					fmt.Printf("   Email: %s\n", user.Email)
					fmt.Printf("   Status: %s\n", status)
					fmt.Printf("   Created: %s\n", user.CreatedAt.Format(time.RFC3339))
					if i < len(listResult.Users)-1 {
						fmt.Printf("   ─────────────────────────────────────────────────────────────────────────\n")
					}
				}

				return nil
			})
		},
	}

	cmd.Flags().IntVar(&page, "page", 1, "page number (default 1)")
	cmd.Flags().IntVar(&pageSize, "page-size", 10, "page size (default 10)")
	cmd.Flags().BoolVar(&activeOnly, "active-only", false, "show only active users")
	cmd.Flags().BoolVar(&inactiveOnly, "inactive-only", false, "show only inactive users")

	return cmd
}

// initDBCmd initializes the database
func initDBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init-db",
		Short: "Initialize database",
		Long:  "Initialize the database with required tables and migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithApp(func(ctx context.Context, logger domain.Logger, db *infrastructure.Database) error {
				logger.Info("Initializing database...")

				if err := db.Migrate(); err != nil {
					return fmt.Errorf("failed to run database migrations: %w", err)
				}

				fmt.Printf("✅ Database initialized successfully!\n")
				return nil
			})
		},
	}
	return cmd
}

// versionCmd shows version information
func versionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long:  "Show version and build information for the Pericarp demo",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Pericarp Demo CLI v1.0.0\n")
			fmt.Printf("Built with Go %s\n", "1.22+")
			fmt.Printf("Pericarp Library - Domain-Driven Design, CQRS, and Event Sourcing for Go\n")
		},
	}
	return cmd
}

// runWithApp runs a function with the Pericarp application context
func runWithApp(fn interface{}) error {
	var result error
	var done = make(chan struct{})

	// Create the application with the function invocation
	var app *fx.App

	switch f := fn.(type) {
	case func(context.Context, domain.Logger, pkgapp.CommandBus) error:
		app = pkg.NewApp(
			fx.Invoke(func(logger domain.Logger, commandBus pkgapp.CommandBus) {
				defer close(done)
				ctx := context.Background()
				result = f(ctx, logger, commandBus)
			}),
		)
	case func(context.Context, domain.Logger, pkgapp.QueryBus) error:
		app = pkg.NewApp(
			fx.Invoke(func(logger domain.Logger, queryBus pkgapp.QueryBus) {
				defer close(done)
				ctx := context.Background()
				result = f(ctx, logger, queryBus)
			}),
		)
	case func(context.Context, domain.Logger, *infrastructure.Database) error:
		app = pkg.NewApp(
			fx.Invoke(func(logger domain.Logger, db *infrastructure.Database) {
				defer close(done)
				ctx := context.Background()
				result = f(ctx, logger, db)
			}),
		)
	default:
		return fmt.Errorf("unsupported function type")
	}

	// Start the application
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		return fmt.Errorf("failed to start application: %w", err)
	}

	// Wait for the function to complete
	<-done

	// Stop the application
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		return fmt.Errorf("failed to stop application: %w", err)
	}

	return result
}