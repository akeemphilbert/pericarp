package auth

import "time"

// SubscriptionStatus enumerates the lifecycle states a subscription can be
// in. Provider-specific states are normalized into one of these values so
// consumers can gate access uniformly regardless of billing backend.
type SubscriptionStatus string

const (
	SubscriptionStatusActive   SubscriptionStatus = "active"
	SubscriptionStatusTrialing SubscriptionStatus = "trialing"
	// PastDue is split out from Cancelled because billing providers commonly
	// flag a failed payment without ending the subscription. Treated as
	// inactive by IsActive; consumers may layer a grace-period policy on top.
	SubscriptionStatusPastDue   SubscriptionStatus = "past_due"
	SubscriptionStatusCancelled SubscriptionStatus = "cancelled"
	SubscriptionStatusInactive  SubscriptionStatus = "inactive"
)

// Valid reports whether s is one of the defined SubscriptionStatus
// constants. Useful in adapter-side tests to catch provider-string drift
// (e.g. "ACTIVE" vs "active") before a malformed status reaches token
// issuance and silently downgrades a paying customer.
func (s SubscriptionStatus) Valid() bool {
	switch s {
	case SubscriptionStatusActive,
		SubscriptionStatusTrialing,
		SubscriptionStatusPastDue,
		SubscriptionStatusCancelled,
		SubscriptionStatusInactive:
		return true
	}
	return false
}

// SubscriptionClaim is the normalized subscription snapshot embedded in an
// identity token at issuance time. Consumers read it off the validated
// token instead of looking up billing state per request. Metadata is the
// open extension space for provider-specific fields (Stripe metadata,
// RevenueCat entitlements); the four normalized fields above it are the
// minimal common shape every consumer can rely on.
type SubscriptionClaim struct {
	Status   SubscriptionStatus `json:"status"`
	Plan     string             `json:"plan,omitempty"`
	Provider string             `json:"provider,omitempty"`
	// ExpiresAt uses omitzero (Go 1.24+) because time.Time's zero value is
	// not the JSON zero value — omitempty would emit "0001-01-01T00:00:00Z"
	// for unset times instead of dropping the field.
	ExpiresAt time.Time      `json:"expires_at,omitzero"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// IsActive reports whether the subscription should grant paid-tier access.
// Returns false for a nil receiver, for any non-active/trialing status, and
// for an explicit ExpiresAt in the past — that last guard means a stale
// snapshot held across an account-switch (which preserves the claim
// verbatim) cannot grant access beyond the provider-attested expiry. A
// zero ExpiresAt is treated as "no expiry expressed" and ignored.
func (s *SubscriptionClaim) IsActive() bool {
	if s == nil {
		return false
	}
	if !s.ExpiresAt.IsZero() && time.Now().After(s.ExpiresAt) {
		return false
	}
	switch s.Status {
	case SubscriptionStatusActive, SubscriptionStatusTrialing:
		return true
	}
	return false
}
