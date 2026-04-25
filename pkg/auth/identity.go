package auth

import "context"

// contextKey is an unexported type used for context keys in this package,
// preventing collisions with keys defined in other packages.
type contextKey struct{ name string }

var identityKey = &contextKey{"pericarp-identity"}

// Identity represents an authenticated agent's identity, independent of the
// underlying authentication mechanism (JWT, session, etc.).
//
// Subscription is populated by JWT-validating middleware when the validated
// token carries a subscription claim. Session-based authentication leaves
// it nil — sessions don't snapshot subscription state.
type Identity struct {
	AgentID         string
	AccountIDs      []string
	ActiveAccountID string
	Subscription    *SubscriptionClaim
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
