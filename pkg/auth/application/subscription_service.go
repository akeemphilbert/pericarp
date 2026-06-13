package application

import (
	"context"

	"github.com/akeemphilbert/pericarp/pkg/auth"
)

// SubscriptionService resolves the current subscription state for an agent
// at token-issuance time. The resulting SubscriptionClaim is embedded in
// the JWT so consumer services can gate paid-tier access without per-
// request billing API calls. Returning (nil, nil) is the canonical "no
// record" answer — the claim is omitted from the token and consumers see
// inactive via SubscriptionClaim.IsActive on the nil pointer. Errors are
// logged by the caller and treated the same as no record so a billing-
// provider outage cannot block login.
type SubscriptionService interface {
	GetSubscription(ctx context.Context, agentID, accountID string) (*auth.SubscriptionClaim, error)
}
