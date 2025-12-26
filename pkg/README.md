# Public API Packages

This directory contains the public API packages that users of the Pericarp library should import.

## Structure

The public API is organized into logical packages:

- **eventsourcing**: Event sourcing primitives including events, event stores, and event handling
- **application**: Command/query handlers, application services, and CQRS bus (future)
- **infrastructure**: Event store implementations, database access, and external integrations (future)
- **auth**: Authentication and authorization primitives (future)

## Usage

```go
import (
    "github.com/akeemphilbert/pericarp/pkg/eventsourcing"
    "github.com/akeemphilbert/pericarp/pkg/application"
    "github.com/akeemphilbert/pericarp/pkg/infrastructure"
)
```

## Note

This directory structure follows golang-standards/project-layout. Packages are organized by domain concern to allow for clean separation and future expansion.

