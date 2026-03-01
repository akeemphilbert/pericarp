# Pericarp Documentation

Pericarp is a Go library implementing Event Sourcing, DDD, and CQRS primitives with built-in authentication and authorization. It provides base types for aggregate roots, event envelopes, event stores, a unit of work, an event dispatcher, a command dispatcher, and a complete auth package with OAuth 2.0/OIDC support and ODRL-based policy evaluation.

Documentation is organized using the [Diataxis framework](https://diataxis.fr/):

| Document | Purpose |
|----------|---------|
| [Tutorial](tutorial.md) | Learn by building — event-sourced aggregates, OAuth authentication, and Casbin authorization from scratch |
| [How-To Guides](how-to.md) | Task-oriented recipes — pattern matching, concurrency control, OAuth flows, Casbin and PDP authorization |
| [Reference](reference.md) | Complete API documentation — every exported type, function, and interface, including Casbin |
| [Explanation](explanation.md) | Design decisions — generics strategy, Watchable pattern, ontology choices, BFF security, ODRL-to-Casbin mapping |

## Quick Start

```go
go get github.com/akeemphilbert/pericarp
```

```go
import (
    // Event sourcing and DDD primitives
    "github.com/akeemphilbert/pericarp/pkg/ddd"
    "github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
    "github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
    "github.com/akeemphilbert/pericarp/pkg/eventsourcing/application"
    "github.com/akeemphilbert/pericarp/pkg/cqrs"

    // Authentication and authorization
    "github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
    "github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
    authapp "github.com/akeemphilbert/pericarp/pkg/auth/application"
    "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/session"
)
```

Start with the [Tutorial](tutorial.md) if you're new to Pericarp.
