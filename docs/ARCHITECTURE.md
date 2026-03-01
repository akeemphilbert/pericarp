---
layout: default
title: Architecture
nav_order: 6
---

# Pericarp Architecture

This document describes the architecture and design decisions of the Pericarp library.

## Overview

Pericarp is a Go library implementing Domain-Driven Design (DDD), Command Query Responsibility Segregation (CQRS), and Event Sourcing patterns with clean architecture principles.

## Project Structure

```
pericarp/
├── pkg/
│   ├── ddd/                            # BaseEntity — embed in aggregate roots
│   │   └── entity.go
│   ├── eventsourcing/
│   │   ├── domain/                     # Core interfaces and types
│   │   │   ├── event.go                # EventEnvelope[T], BasicTripleEvent
│   │   │   ├── eventstore.go           # EventStore interface, sentinel errors
│   │   │   ├── event_dispatcher.go     # EventDispatcher (subscribe + dispatch)
│   │   │   ├── event_type.go           # Event type constants and helpers
│   │   │   └── entity.go              # Entity interface (for UnitOfWork)
│   │   ├── infrastructure/             # EventStore implementations
│   │   │   ├── memory_eventstore.go    # In-memory (testing)
│   │   │   └── file_eventstore.go      # File-based JSON (development)
│   │   └── application/                # Application services
│   │       └── unit_of_work.go         # SimpleUnitOfWork
│   ├── cqrs/                           # Command dispatching
│   │   └── command_dispatcher.go       # CommandDispatcher, Watchable
│   └── auth/                           # Authentication and authorization
│       ├── domain/
│       │   ├── entities/               # Aggregate roots
│       │   │   ├── agent.go            # Agent (FOAF-typed actors)
│       │   │   ├── account.go          # Account (multi-tenant workspace)
│       │   │   ├── role.go             # Role (W3C ORG)
│       │   │   ├── policy.go           # Policy (ODRL access control)
│       │   │   ├── credential.go       # Credential (OAuth provider link)
│       │   │   ├── auth_session.go     # AuthSession (authenticated session)
│       │   │   ├── ontology.go         # FOAF, ORG, ODRL, Schema.org constants
│       │   │   └── *_events.go         # Domain event definitions
│       │   └── repositories/           # Repository interfaces
│       │       └── repositories.go     # All repository contracts
│       ├── application/                # Application services
│       │   ├── authentication_service.go  # OAuth flow orchestration
│       │   ├── authorization_service.go   # PolicyDecisionPoint (ODRL)
│       │   └── pkce.go                    # PKCE helpers (RFC 7636)
│       └── infrastructure/
│           └── session/                # HTTP session management
│               ├── session_manager.go  # SessionManager interface
│               └── gorilla_session_manager.go  # gorilla/sessions implementation
├── internal/               # Private implementation packages
├── cmd/pericarp/           # CLI tools/demos
├── examples/               # Runnable examples for users
├── test/                   # Integration tests
├── docs/                   # Documentation (Diataxis framework)
├── scripts/                # Build/automation scripts
└── configs/                # Configuration templates
```

## Architecture Layers

### Domain Layer (`pkg/domain/`)

The domain layer contains:
- Aggregate roots and entities
- Domain events
- Value objects
- Domain services
- Repository interfaces

### Application Layer (`pkg/application/`)

The application layer contains:
- Command and query handlers
- Application services
- DTOs and data transfer objects
- Use case orchestration
- CQRS bus implementation

### Infrastructure Layer (`pkg/infrastructure/`)

The infrastructure layer contains:
- Event store implementations
- Repository implementations
- Database access
- External service integrations
- Configuration management

## Design Patterns

### Event Sourcing

All state changes are captured as events. The event store persists events, and aggregates are reconstructed by replaying events.

### CQRS

Commands and queries are separated:
- Commands modify state and return success/failure
- Queries read state and return data
- Separate handlers for commands and queries

### Clean Architecture

Dependencies point inward:
- Domain has no dependencies
- Application depends only on domain
- Infrastructure depends on application and domain

## Key Components

### Event Store

The event store is responsible for:
- Persisting domain events
- Retrieving events for aggregate reconstruction
- Optimistic concurrency control via `expectedVersion`
- Supporting multiple backends (MemoryStore for tests, FileStore for dev, database for prod)

### Event Dispatcher

The event dispatcher:
- Publishes events to subscribers via pattern matching (`user.created`, `user.*`, `*.created`, `*.*`)
- Executes handlers in parallel via `errgroup`
- Supports wildcard catch-all handlers

### Aggregate Root

The base aggregate root (`ddd.BaseEntity`) provides:
- Event recording and sequence number management
- Uncommitted event tracking for UnitOfWork integration
- Event replay for state reconstruction
- Thread-safe access via `sync.RWMutex`

### Authentication (OAuth 2.0 / OIDC)

The auth package implements the Backend-for-Frontend (BFF) pattern:
- **AuthenticationService** — orchestrates the OAuth Authorization Code Flow with PKCE
- **OAuthProvider** — provider-agnostic interface (implement once per IdP)
- **Credential** aggregate — links external identities to agents via Schema.org vocabulary
- **AuthSession** aggregate — tracks authenticated sessions with expiration and account scoping
- **SessionManager** — HTTP cookie management (gorilla/sessions) with secure defaults
- **TokenStore** — encrypted server-side token storage interface

### Authorization (ODRL)

The auth package implements ODRL-based access control:
- **PolicyDecisionPoint** — evaluates permissions and prohibitions following ODRL semantics (prohibitions override permissions, default deny)
- **Policy** aggregate — defines permissions, prohibitions, and duties using ODRL vocabulary
- **Role** aggregate — named roles assigned to agents globally or per-account
- **Account** aggregate — multi-tenant workspace with member management

### Ontology

The auth domain uses three established ontologies:
- **FOAF** — agent typing (Person, Organization, Group, Software Agent)
- **W3C ORG** — organizational relationships (roles, membership, temporal tracking)
- **ODRL** — access control (permissions, prohibitions, duties, policy types)
- **Schema.org** — authentication concepts (credentials, sessions, providers)

Cross-aggregate relationships are modeled as enriched triple events (`BasicTripleEvent`) using these standard predicates.

## Testing Strategy

- **Unit Tests**: Fast, isolated tests for individual components
- **Integration Tests**: End-to-end testing with real databases
- **Performance Tests**: Benchmarking and profiling

## Database Support

- **SQLite**: Development and testing
- **PostgreSQL**: Production deployments

## Future Enhancements

- [ ] Add more database backends
- [ ] Improve performance optimizations
- [ ] Add more middleware options
- [ ] Enhance documentation

## References

- [Domain-Driven Design](https://martinfowler.com/bliki/DomainDrivenDesign.html)
- [CQRS Pattern](https://martinfowler.com/bliki/CQRS.html)
- [Event Sourcing](https://martinfowler.com/eaaDev/EventSourcing.html)
- [Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
