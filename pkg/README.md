# Public API Packages

This directory contains the public API packages that users of the Pericarp library should import.

## Structure

The public API is organized into logical packages:

- **domain**: Domain entities, events, value objects, and repository interfaces
- **application**: Command/query handlers, application services, and CQRS bus
- **infrastructure**: Event store implementations, database access, and external integrations

## Usage

```go
import (
    "github.com/wepala/pericarp/pkg/domain"
    "github.com/wepala/pericarp/pkg/application"
    "github.com/wepala/pericarp/pkg/infrastructure"
)
```

## Note

This directory structure follows golang-standards/project-layout. The actual implementation will be added in subsequent tasks.

