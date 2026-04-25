package application

import (
	"context"

	"github.com/akeemphilbert/pericarp/pkg/auth"
)

// SubscriptionService resolves the current subscription state for an agent
// at token-issuance time. The resulting SubscriptionClaim is embedded in the
// JWT so consumer services can gate paid-tier access without per-request
// billing API calls.
//
// Implementations should return a non-nil *SubscriptionClaim with
// SubscriptionStatusInactive when the agent has no record at the provider,
// so the absence of a paid plan is captured explicitly rather than as a
// nil claim. A nil claim with a nil error means "no claim to embed" and is
// also valid (the JWT will omit the field).
type SubscriptionService interface {
	GetSubscription(ctx context.Context, agentID, accountID string) (*auth.SubscriptionClaim, error)
}
