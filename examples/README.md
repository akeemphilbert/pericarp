# Runnable Examples

Self-contained examples demonstrating Pericarp's authentication and authorization systems.

## Authentication (`authn/`)

Walks through the full authentication lifecycle:

1. RSA key generation and JWT service setup
2. OAuth flow initiation with PKCE
3. Code exchange and state validation
4. Agent + Credential + Account creation via `FindOrCreateAgent`
5. Session creation, validation, and revocation
6. JWT issuance and validation
7. Identity context propagation (`ContextWithAgent` / `AgentFromCtx`)
8. Resource ownership and tenant isolation (`ResourceOwnershipFromCtx` / `VerifyAccountAccess`)

```bash
go run ./examples/authn/
go test -v -race ./examples/authn/
```

## Authorization (`authz/`)

Demonstrates authorization decisions using both the `PolicyDecisionPoint` (pure Go) and `CasbinAuthorizationChecker`:

- Role-based access control (viewer / editor / admin)
- ODRL prohibition overrides (deny beats allow)
- Account-scoped roles
- Default deny for agents with no permissions
- Behavioral parity between PDP and Casbin implementations
- Identity context and resource ownership integration

```bash
go run ./examples/authz/
go test -v -race ./examples/authz/
```
