package application

import (
	"testing"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

// mockMetricsCollector is a simple mock implementation for testing
type mockMetricsCollector struct{}

func (m *mockMetricsCollector) RecordRequestDuration(requestType string, duration time.Duration) {}
func (m *mockMetricsCollector) IncrementRequestErrors(requestType string)                        {}

// mockCacheProvider is a simple mock implementation for testing
type mockCacheProvider struct{}

func (m *mockCacheProvider) Get(key string) (any, bool) { return nil, false }
func (m *mockCacheProvider) Set(key string, value any)  {}
func (m *mockCacheProvider) Delete(key string)          {}

func TestApplicationModule(t *testing.T) {
	app := fxtest.New(t,
		ApplicationModule,
		fx.Provide(
			func() MetricsCollector { return &mockMetricsCollector{} },
			func() CacheProvider { return &mockCacheProvider{} },
		),
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
	if loggingCmdMiddleware.Middleware == nil {
		t.Error("LoggingCommandMiddleware should not be nil")
	}

	loggingQueryMiddleware := LoggingQueryMiddlewareProvider()
	if loggingQueryMiddleware.Middleware == nil {
		t.Error("LoggingQueryMiddleware should not be nil")
	}

	// Test validation middleware provider
	validationMiddleware := ValidationCommandMiddlewareProvider()
	if validationMiddleware.Middleware == nil {
		t.Error("ValidationCommandMiddleware should not be nil")
	}
}
