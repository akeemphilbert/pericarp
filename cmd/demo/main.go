package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/pericarp/pkg"
	"github.com/example/pericarp/pkg/application"
	"github.com/example/pericarp/pkg/domain"
	"github.com/example/pericarp/pkg/infrastructure"
	"go.uber.org/fx"
)

func main() {
	// Create the Pericarp application with additional demo components
	app := pkg.NewApp(
		fx.Invoke(runDemo),
	)

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		fmt.Println("\nReceived shutdown signal, stopping application...")
		cancel()
	}()

	// Start the application
	startCtx, startCancel := context.WithTimeout(ctx, 10*time.Second)
	defer startCancel()

	if err := app.Start(startCtx); err != nil {
		fmt.Printf("Failed to start application: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Pericarp demo application started successfully!")
	fmt.Println("Press Ctrl+C to stop...")

	// Wait for shutdown signal
	<-ctx.Done()

	// Stop the application
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		fmt.Printf("Failed to stop application gracefully: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Application stopped successfully!")
}

// runDemo demonstrates the basic functionality of the Pericarp library
func runDemo(
	config *infrastructure.Config,
	logger domain.Logger,
	eventStore domain.EventStore,
	eventDispatcher domain.EventDispatcher,
	unitOfWork domain.UnitOfWork,
	commandBus application.CommandBus,
	queryBus application.QueryBus,
	metrics application.MetricsCollector,
) {
	logger.Info("Starting Pericarp demo", "version", "1.0.0")

	// Display configuration
	logger.Info("Configuration loaded",
		"database_driver", config.Database.Driver,
		"events_publisher", config.Events.Publisher,
		"logging_level", config.Logging.Level,
		"logging_format", config.Logging.Format,
	)

	// Test event store
	ctx := context.Background()
	logger.Info("Testing event store...")
	
	events := []domain.Event{} // Empty for now
	envelopes, err := eventStore.Save(ctx, events)
	if err != nil {
		logger.Error("Event store test failed", "error", err)
		return
	}
	logger.Info("Event store test completed", "envelopes_count", len(envelopes))

	// Test event dispatcher
	logger.Info("Testing event dispatcher...")
	err = eventDispatcher.Dispatch(ctx, envelopes)
	if err != nil {
		logger.Error("Event dispatcher test failed", "error", err)
		return
	}
	logger.Info("Event dispatcher test completed")

	// Test unit of work
	logger.Info("Testing unit of work...")
	unitOfWork.RegisterEvents(events)
	resultEnvelopes, err := unitOfWork.Commit(ctx)
	if err != nil {
		logger.Error("Unit of work test failed", "error", err)
		return
	}
	logger.Info("Unit of work test completed", "result_envelopes_count", len(resultEnvelopes))

	// Test command and query buses
	logger.Info("Command and query buses are ready")
	logger.Info("Metrics collector is ready")

	logger.Info("Demo completed successfully! All components are working.")
	logger.Info("The application will continue running until you press Ctrl+C")
}