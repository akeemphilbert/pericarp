// Package subscription contains application.SubscriptionService adapters for
// concrete billing providers. Each adapter normalizes provider-specific
// subscription state into auth.SubscriptionClaim so consumer services see a
// uniform shape regardless of the billing backend.
package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth"
)

const (
	revenueCatDefaultBaseURL = "https://api.revenuecat.com/v1"
	revenueCatProvider       = "revenuecat"
)

// RevenueCatOption configures a RevenueCat adapter.
type RevenueCatOption func(*RevenueCat)

// WithRevenueCatBaseURL overrides the API base URL. Primarily used by tests
// pointing at httptest servers; production wiring uses the default.
func WithRevenueCatBaseURL(u string) RevenueCatOption {
	return func(r *RevenueCat) {
		if u != "" {
			r.baseURL = strings.TrimRight(u, "/")
		}
	}
}

// WithRevenueCatHTTPClient overrides the HTTP client. Use this to plumb a
// shared client with timeouts, retries, or instrumentation. The default
// client has a 5-second timeout, which is short enough that a wedged
// billing API still returns control to IssueIdentityToken before the user
// gives up on login.
func WithRevenueCatHTTPClient(c *http.Client) RevenueCatOption {
	return func(r *RevenueCat) {
		if c != nil {
			r.client = c
		}
	}
}

// WithRevenueCatNow overrides the time source used to decide whether an
// entitlement's expiry is in the past. Tests inject a deterministic clock;
// production uses time.Now.
func WithRevenueCatNow(now func() time.Time) RevenueCatOption {
	return func(r *RevenueCat) {
		if now != nil {
			r.now = now
		}
	}
}

// RevenueCat resolves SubscriptionClaim values from the RevenueCat REST API.
// It implements application.SubscriptionService.
type RevenueCat struct {
	apiKey  string
	baseURL string
	client  *http.Client
	now     func() time.Time
}

// NewRevenueCat returns a RevenueCat adapter authenticated with apiKey
// (the project's secret API key). An empty apiKey is permitted at
// construction time but every GetSubscription call will fail until set.
func NewRevenueCat(apiKey string, opts ...RevenueCatOption) *RevenueCat {
	r := &RevenueCat{
		apiKey:  apiKey,
		baseURL: revenueCatDefaultBaseURL,
		client:  &http.Client{Timeout: 5 * time.Second},
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// revenueCatResponse is the trimmed shape of GET /v1/subscribers/{app_user_id}.
// Only the fields the adapter consumes are decoded; the rest of RevenueCat's
// response is preserved opaquely in SubscriptionClaim.Metadata via
// raw_entitlements when the caller wants provider-native data.
type revenueCatResponse struct {
	Subscriber struct {
		Entitlements  map[string]revenueCatEntitlement  `json:"entitlements"`
		Subscriptions map[string]revenueCatSubscription `json:"subscriptions"`
	} `json:"subscriber"`
}

type revenueCatEntitlement struct {
	ExpiresDate       *time.Time `json:"expires_date"`
	ProductIdentifier string     `json:"product_identifier"`
	PurchaseDate      *time.Time `json:"purchase_date"`
}

type revenueCatSubscription struct {
	ExpiresDate             *time.Time `json:"expires_date"`
	BillingIssuesDetectedAt *time.Time `json:"billing_issues_detected_at"`
	UnsubscribeDetectedAt   *time.Time `json:"unsubscribe_detected_at"`
	PeriodType              string     `json:"period_type"`
	Store                   string     `json:"store"`
}

// GetSubscription queries RevenueCat for agentID's current subscription
// state. accountID is unused — RevenueCat keys on a single app_user_id, so
// callers should arrange to use agentID consistently across registration
// and lookup. Returns (nil, nil) when the subscriber has no active or
// recently-active entitlement; returns an error for transport failures and
// non-2xx responses other than 404 (which is also treated as "no record").
func (r *RevenueCat) GetSubscription(ctx context.Context, agentID, accountID string) (*auth.SubscriptionClaim, error) {
	_ = accountID
	if r.apiKey == "" {
		return nil, errors.New("revenuecat: missing API key")
	}
	if agentID == "" {
		return nil, errors.New("revenuecat: agentID must not be empty")
	}

	url := fmt.Sprintf("%s/subscribers/%s", r.baseURL, agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("revenuecat: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("revenuecat: http call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("revenuecat: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded revenueCatResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("revenuecat: decode response: %w", err)
	}

	return r.toClaim(decoded), nil
}

// toClaim picks the best entitlement (latest expiry, ties broken by name)
// and maps it to a SubscriptionClaim. Returns nil when the subscriber has
// no entitlements at all — RevenueCat returns 200 with empty maps for
// users who exist in the project but have never purchased.
func (r *RevenueCat) toClaim(resp revenueCatResponse) *auth.SubscriptionClaim {
	if len(resp.Subscriber.Entitlements) == 0 {
		return nil
	}

	now := r.now()
	var (
		bestName   string
		bestExpiry time.Time
		bestEnt    revenueCatEntitlement
		hasUnbound bool // true when the chosen entitlement has nil expires_date (lifetime)
	)
	for name, ent := range resp.Subscriber.Entitlements {
		if ent.ExpiresDate == nil {
			// Lifetime entitlements always win over any time-bounded one.
			if !hasUnbound || name < bestName {
				bestName = name
				bestEnt = ent
				bestExpiry = time.Time{}
				hasUnbound = true
			}
			continue
		}
		if hasUnbound {
			continue
		}
		if bestName == "" || ent.ExpiresDate.After(bestExpiry) || (ent.ExpiresDate.Equal(bestExpiry) && name < bestName) {
			bestName = name
			bestEnt = ent
			bestExpiry = *ent.ExpiresDate
		}
	}

	claim := &auth.SubscriptionClaim{
		Plan:     bestName,
		Provider: revenueCatProvider,
		Metadata: map[string]any{
			"product_identifier": bestEnt.ProductIdentifier,
		},
	}
	if !hasUnbound {
		claim.ExpiresAt = bestExpiry
	}

	// Status mapping prefers signals from the underlying subscription row
	// (period_type == "trial", billing_issues_detected_at set) over the
	// raw entitlement-expiry check, because RevenueCat surfaces those
	// signals on the subscription, not the entitlement.
	sub, hasSub := resp.Subscriber.Subscriptions[bestEnt.ProductIdentifier]
	switch {
	case !hasUnbound && now.After(bestExpiry):
		claim.Status = auth.SubscriptionStatusInactive
	case hasSub && sub.BillingIssuesDetectedAt != nil:
		claim.Status = auth.SubscriptionStatusPastDue
	case hasSub && sub.PeriodType == "trial":
		claim.Status = auth.SubscriptionStatusTrialing
	default:
		claim.Status = auth.SubscriptionStatusActive
	}

	if hasSub && sub.Store != "" {
		claim.Metadata["store"] = sub.Store
	}
	return claim
}
