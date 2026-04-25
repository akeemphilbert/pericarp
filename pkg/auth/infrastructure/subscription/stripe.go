package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth"
)

const (
	stripeDefaultBaseURL     = "https://api.stripe.com/v1"
	stripeDefaultMetadataKey = "agent_id"
	stripeProvider           = "stripe"
)

// StripeOption configures a Stripe adapter.
type StripeOption func(*Stripe)

// WithStripeBaseURL overrides the API base URL. The test seam.
func WithStripeBaseURL(u string) StripeOption {
	return func(s *Stripe) {
		if u != "" {
			s.baseURL = strings.TrimRight(u, "/")
		}
	}
}

// WithStripeHTTPClient overrides the HTTP client. Default is &http.Client
// {Timeout: 5 * time.Second}; production deployments can plumb a shared
// client with retries or instrumentation.
func WithStripeHTTPClient(c *http.Client) StripeOption {
	return func(s *Stripe) {
		if c != nil {
			s.client = c
		}
	}
}

// WithStripeAgentMetadataKey overrides the customer-metadata key the
// adapter searches on. The default is "agent_id"; existing Stripe
// installations may already store the link under a different key.
func WithStripeAgentMetadataKey(k string) StripeOption {
	return func(s *Stripe) {
		if k != "" {
			s.agentMetadataKey = k
		}
	}
}

// Stripe resolves SubscriptionClaim values from the Stripe API by searching
// customers on a configurable metadata field. It implements
// application.SubscriptionService.
type Stripe struct {
	apiKey           string
	baseURL          string
	client           *http.Client
	agentMetadataKey string
}

// NewStripe returns a Stripe adapter authenticated with apiKey (a Stripe
// secret key, "sk_..."). Construction with an empty key is permitted but
// every call returns an error until the key is set.
func NewStripe(apiKey string, opts ...StripeOption) *Stripe {
	s := &Stripe{
		apiKey:           apiKey,
		baseURL:          stripeDefaultBaseURL,
		client:           &http.Client{Timeout: 5 * time.Second},
		agentMetadataKey: stripeDefaultMetadataKey,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// stripeCustomerSearch is the trimmed shape of GET /v1/customers/search
// with `expand[]=data.subscriptions` so each customer carries its
// subscription list inline. Only fields the adapter consumes are decoded.
type stripeCustomerSearch struct {
	Data []stripeCustomer `json:"data"`
}

type stripeCustomer struct {
	ID            string `json:"id"`
	Subscriptions struct {
		Data []stripeSubscription `json:"data"`
	} `json:"subscriptions"`
}

type stripeSubscription struct {
	ID                string                  `json:"id"`
	Status            string                  `json:"status"`
	CurrentPeriodEnd  int64                   `json:"current_period_end"`
	CancelAtPeriodEnd bool                    `json:"cancel_at_period_end"`
	Items             stripeSubscriptionItems `json:"items"`
}

type stripeSubscriptionItems struct {
	Data []stripeSubscriptionItem `json:"data"`
}

type stripeSubscriptionItem struct {
	Price stripePrice `json:"price"`
}

type stripePrice struct {
	LookupKey string `json:"lookup_key"`
	Nickname  string `json:"nickname"`
	Product   string `json:"product"`
}

// stripeStatusToClaim normalizes Stripe's subscription.status values into
// auth.SubscriptionStatus. Statuses that map to no paid access (incomplete,
// incomplete_expired, unpaid, paused) collapse to Inactive.
func stripeStatusToClaim(status string) auth.SubscriptionStatus {
	switch status {
	case "active":
		return auth.SubscriptionStatusActive
	case "trialing":
		return auth.SubscriptionStatusTrialing
	case "past_due":
		return auth.SubscriptionStatusPastDue
	case "canceled":
		return auth.SubscriptionStatusCancelled
	default:
		return auth.SubscriptionStatusInactive
	}
}

// GetSubscription queries Stripe for the subscription tied to agentID via
// the configured customer-metadata key. accountID is unused — Stripe is
// keyed on a single customer per agent in this adapter; consumers needing
// per-account billing should compose a different adapter or fork this one.
// Returns (nil, nil) when no customer matches or when the customer has no
// subscriptions; returns an error for transport failures, non-2xx responses
// other than 404, and decode failures.
func (s *Stripe) GetSubscription(ctx context.Context, agentID, accountID string) (*auth.SubscriptionClaim, error) {
	_ = accountID
	if s.apiKey == "" {
		return nil, errors.New("stripe: missing API key")
	}
	if agentID == "" {
		return nil, errors.New("stripe: agentID must not be empty")
	}

	// Stripe search syntax: metadata['key']:'value'. Single quotes inside
	// agentID are escaped per Stripe's docs by doubling — same convention
	// SQL uses. Limits the blast radius of a malformed agent ID.
	query := fmt.Sprintf("metadata['%s']:'%s'", s.agentMetadataKey, strings.ReplaceAll(agentID, "'", `\'`))
	endpoint := fmt.Sprintf("%s/customers/search?query=%s&expand[]=data.subscriptions", s.baseURL, url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("stripe: build request: %w", err)
	}
	req.SetBasicAuth(s.apiKey, "")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe: http call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("stripe: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded stripeCustomerSearch
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("stripe: decode response: %w", err)
	}

	return s.toClaim(decoded), nil
}

// toClaim picks the best subscription across all matched customers and
// maps it to a SubscriptionClaim. Selection order: prefer active/trialing
// over other statuses; among ties, latest current_period_end wins;
// further ties broken by subscription ID for stable output.
func (s *Stripe) toClaim(search stripeCustomerSearch) *auth.SubscriptionClaim {
	var best *stripeSubscription
	for i := range search.Data {
		for j := range search.Data[i].Subscriptions.Data {
			cand := &search.Data[i].Subscriptions.Data[j]
			if betterSubscription(best, cand) {
				best = cand
			}
		}
	}
	if best == nil {
		return nil
	}

	claim := &auth.SubscriptionClaim{
		Status:   stripeStatusToClaim(best.Status),
		Provider: stripeProvider,
		Plan:     subscriptionPlan(best),
	}
	if best.CurrentPeriodEnd > 0 {
		claim.ExpiresAt = time.Unix(best.CurrentPeriodEnd, 0).UTC()
	}
	if best.CancelAtPeriodEnd {
		// Surface the cancel-at-period-end flag for consumers that want
		// to render renewal-status UI; the headline Status remains
		// active/trialing until the period actually lapses.
		if claim.Metadata == nil {
			claim.Metadata = map[string]any{}
		}
		claim.Metadata["cancel_at_period_end"] = true
	}
	if best.ID != "" {
		if claim.Metadata == nil {
			claim.Metadata = map[string]any{}
		}
		claim.Metadata["subscription_id"] = best.ID
	}
	return claim
}

// betterSubscription returns true when cand should replace best. Active
// and trialing rank above any other status; within a rank, latest period
// end wins; within that, subscription ID breaks ties for stable output
// regardless of map iteration order in the response.
func betterSubscription(best, cand *stripeSubscription) bool {
	if best == nil {
		return true
	}
	bestRank := stripeRank(best.Status)
	candRank := stripeRank(cand.Status)
	if candRank != bestRank {
		return candRank > bestRank
	}
	if cand.CurrentPeriodEnd != best.CurrentPeriodEnd {
		return cand.CurrentPeriodEnd > best.CurrentPeriodEnd
	}
	return cand.ID < best.ID
}

func stripeRank(status string) int {
	switch status {
	case "active", "trialing":
		return 3
	case "past_due":
		return 2
	case "canceled":
		return 1
	default:
		return 0
	}
}

func subscriptionPlan(sub *stripeSubscription) string {
	if sub == nil || len(sub.Items.Data) == 0 {
		return ""
	}
	p := sub.Items.Data[0].Price
	switch {
	case p.LookupKey != "":
		return p.LookupKey
	case p.Nickname != "":
		return p.Nickname
	default:
		return p.Product
	}
}
