package domain

import (
	"go.uber.org/fx"
)

// DomainModule provides domain layer dependencies
// Note: Domain layer should remain pure with minimal dependencies.
// Most domain components (aggregates, value objects, events) should be
// created directly by application layer without dependency injection.
//
// If domain services are needed, they can be provided through the
// application or infrastructure layers.
var DomainModule = fx.Options(
// Domain layer typically doesn't need dependency injection
// as it should remain pure and free of infrastructure concerns.
// Domain services, if needed, should be provided by higher layers.
)
