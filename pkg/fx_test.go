package pkg

import (
	"context"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/application"
	"github.com/akeemphilbert/pericarp/pkg/domain"
	"github.com/akeemphilbert/pericarp/pkg/infrastructure"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestPericarpModule(t *testing.T) {
	app := fxtest.New(t,
		PericarpModule,
		fx.StartTimeout(10*time.Second),
		fx.StopTimeout(5*time.Second),
		fx.Invoke(func(
			config *infrastructure.Config,
			logger domain.Logger,
			eventStore domain.EventStore,
			eventDispatcher domain.EventDispatcher,
			unitOfWork domain.UnitOfWork,
			commandBus application.CommandBus,
			queryBus application.QueryBus,
			metrics application.MetricsCollector,
		) {
			// Test that all major components are properly injected
			if config == nil {
				t.Error("Config should not be nil")
			}
			if logger == nil {
				t.Error("Logger should not be nil")
			}
			if eventStore == nil {
				t.Error("EventStore should not be nil")
			}
			if eventDispatcher == nil {
				t.Error("EventDispatcher should not be nil")
			}
			if unitOfWork == nil {
				t.Error("UnitOfWork should not be nil")
			}
			if commandBus == nil {
				t.Error("CommandBus should not be nil")
			}
			if queryBus == nil {
				t.Error("QueryBus should not be nil")
			}
			if metrics == nil {
				t.Error("MetricsCollector should not be nil")
			}

			// Test basic functionality
			logger.Info("Pericarp module test", "status", "success")

			// Test event store with empty events
			ctx := context.Background()
			events := []domain.Event{}
			envelopes, err := eventStore.Save(ctx, events)
			if err != nil {
				t.Errorf("EventStore.Save failed: %v", err)
			}
			if len(envelopes) != 0 {
				t.Errorf("Expected 0 envelopes, got %d", len(envelopes))
			}

			// Test event dispatcher with empty envelopes
			err = eventDispatcher.Dispatch(ctx, envelopes)
			if err != nil {
				t.Errorf("EventDispatcher.Dispatch failed: %v", err)
			}

			// Test unit of work
			unitOfWork.RegisterEvents(events)
			resultEnvelopes, err := unitOfWork.Commit(ctx)
			if err != nil {
				t.Errorf("UnitOfWork.Commit failed: %v", err)
			}
			if len(resultEnvelopes) != 0 {
				t.Errorf("Expected 0 result envelopes, got %d", len(resultEnvelopes))
			}
		}),
	)

	defer app.RequireStart().RequireStop()
}

func TestNewApp(t *testing.T) {
	app := NewApp()
	if app == nil {
		t.Error("NewApp should not return nil")
	}

	// Test that the app can be created without errors
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	startCtx, startCancel := context.WithTimeout(ctx, 2*time.Second)
	defer startCancel()

	if err := app.Start(startCtx); err != nil {
		t.Fatalf("App failed to start: %v", err)
	}

	stopCtx, stopCancel := context.WithTimeout(ctx, 2*time.Second)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("App failed to stop: %v", err)
	}
}

func TestNewAppWithAdditionalOptions(t *testing.T) {
	additionalOption := fx.Invoke(func() {
		// This is just a test invoke function
	})

	app := NewApp(additionalOption)
	if app == nil {
		t.Error("NewApp with additional options should not return nil")
	}

	// Test that the app can be created with additional options
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	startCtx, startCancel := context.WithTimeout(ctx, 2*time.Second)
	defer startCancel()

	if err := app.Start(startCtx); err != nil {
		t.Fatalf("App with additional options failed to start: %v", err)
	}

	stopCtx, stopCancel := context.WithTimeout(ctx, 2*time.Second)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("App with additional options failed to stop: %v", err)
	}
}
