# Pericarp Documentation

Pericarp is a Go library implementing Event Sourcing, DDD, and CQRS primitives. It provides base types for aggregate roots, event envelopes, event stores, a unit of work, an event dispatcher, and a command dispatcher.

Documentation is organized using the [Diataxis framework](https://diataxis.fr/):

| Document | Purpose |
|----------|---------|
| [Tutorial](tutorial.md) | Learn by building — walks through creating an event-sourced aggregate from scratch |
| [How-To Guides](how-to.md) | Task-oriented recipes — pattern matching, concurrency control, fire-and-forget commands, etc. |
| [Reference](reference.md) | Complete API documentation — every exported type, function, and interface |
| [Explanation](explanation.md) | Design decisions — why generics work the way they do, the Watchable pattern, thread safety model |

## Quick Start

```go
go get github.com/akeemphilbert/pericarp
```

```go
import (
    "github.com/akeemphilbert/pericarp/pkg/ddd"
    "github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
    "github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
    "github.com/akeemphilbert/pericarp/pkg/eventsourcing/application"
    "github.com/akeemphilbert/pericarp/pkg/cqrs"
)
```

Start with the [Tutorial](tutorial.md) if you're new to Pericarp.
