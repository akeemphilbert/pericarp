package application

import (
	"testing"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

// mockMetricsCollector is a simple mock implementation for testing
type mockMetricsCollector struct{}

func (m *mockMetricsCollector) RecordCommandDuration(commandType string, duration time.Duration) {}
func (m *mockMetricsCollector) RecordQueryDuration(queryType string, duration time.Duration)     {}
func (m *mockMetricsCollector) IncrementCommandErrors(commandType string)                        {}
func (m *mockMetricsCollector) IncrementQueryErrors(queryType string)                            {}

func TestApplicationModule(t *testing.T) {
	app := fxtest.New(t,
		ApplicationModule,
		// Provide mock dependencies needed by the application module
		fx.Provide(func() MetricsCollector {
			return &mockMetricsCollector{}
		}),
		fx.Invoke(func(
			commandBus CommandBus,
			queryBus QueryBus,
		) {
			// Test that all dependencies are properly injected
			if commandBus == nil {
				t.Error("CommandBus should not be nil")
			}
			if queryBus == nil {
				t.Error("QueryBus should not be nil")
			}
		}),
	)

	defer app.RequireStart().RequireStop()
}

func TestCommandBusProvider(t *testing.T) {
	bus := CommandBusProvider()
	if bus == nil {
		t.Error("CommandBus should not be nil")
	}
}

func TestQueryBusProvider(t *testing.T) {
	bus := QueryBusProvider()
	if bus == nil {
		t.Error("QueryBus should not be nil")
	}
}

func TestMiddlewareProviders(t *testing.T) {
	// Test logging middleware providers
	loggingCmdMiddleware := LoggingCommandMiddlewareProvider()
	if loggingCmdMiddleware == nil {
		t.Error("LoggingCommandMiddleware should not be nil")
	}

	loggingQueryMiddleware := LoggingQueryMiddlewareProvider()
	if loggingQueryMiddleware == nil {
		t.Error("LoggingQueryMiddleware should not be nil")
	}

	// Test validation middleware provider
	validationMiddleware := ValidationCommandMiddlewareProvider()
	if validationMiddleware == nil {
		t.Error("ValidationCommandMiddleware should not be nil")
	}
}
