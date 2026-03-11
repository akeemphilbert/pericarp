package auth

import "context"

// contextKey is an unexported type used for context keys in this package,
// preventing collisions with keys defined in other packages.
type contextKey struct{ name string }

var identityKey = &contextKey{"pericarp-identity"}

// Identity represents an authenticated agent's identity, independent of the
// underlying authentication mechanism (JWT, session, etc.).
type Identity struct {
	AgentID         string
	AccountIDs      []string
	ActiveAccountID string
}

// AgentFromCtx extracts the Identity from ctx. Returns nil if no identity is present.
func AgentFromCtx(ctx context.Context) *Identity {
	id, _ := ctx.Value(identityKey).(*Identity)
	return id
}

// ContextWithAgent returns a new context with the given Identity attached.
func ContextWithAgent(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}
