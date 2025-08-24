package pkg

import (
	"github.com/example/pericarp/internal"
	"github.com/example/pericarp/pkg/application"
	"github.com/example/pericarp/pkg/domain"
	"github.com/example/pericarp/pkg/infrastructure"
	"go.uber.org/fx"
)

// Module is an alias for PericarpModule for convenience
var Module = PericarpModule

// PericarpModule combines all layer modules into a single module
var PericarpModule = fx.Options(
	domain.DomainModule,
	application.ApplicationModule,
	infrastructure.InfrastructureModule,
	internal.InternalModule,
)

// NewApp creates a new Fx application with all Pericarp modules
func NewApp(additionalOptions ...fx.Option) *fx.App {
	options := []fx.Option{PericarpModule}
	options = append(options, additionalOptions...)
	
	return fx.New(options...)
}

// RunApp creates and runs a new Fx application with graceful shutdown
func RunApp(additionalOptions ...fx.Option) {
	app := NewApp(additionalOptions...)
	app.Run()
}